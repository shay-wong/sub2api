package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const stickySessionPrefix = "sticky_session:"
const openAIReasoningSourcePrefix = "openai_reasoning_source:"
const liveCallPrefix = "live:call:"
const liveCallRecoveryIndexKey = "live:call:recovery"

type gatewayCache struct {
	rdb *redis.Client
}

func NewGatewayCache(rdb *redis.Client) service.GatewayCache {
	return &gatewayCache{rdb: rdb}
}

// buildSessionKey 构建 session key，包含 groupID 实现分组隔离
// 格式: sticky_session:{groupID}:{sessionHash}
func buildSessionKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", stickySessionPrefix, groupID, sessionHash)
}

func (c *gatewayCache) GetSessionAccountID(ctx context.Context, groupID int64, sessionHash string) (int64, error) {
	key := buildSessionKey(groupID, sessionHash)
	accountID, err := c.rdb.Get(ctx, key).Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, service.ErrStickySessionNotFound
		}
		return 0, err
	}
	return accountID, nil
}

func (c *gatewayCache) SetSessionAccountID(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Set(ctx, key, accountID, ttl).Err()
}

func (c *gatewayCache) RefreshSessionTTL(ctx context.Context, groupID int64, sessionHash string, ttl time.Duration) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Expire(ctx, key, ttl).Err()
}

// DeleteSessionAccountID 删除粘性会话与账号的绑定关系。
// 当检测到绑定的账号不可用（如状态错误、禁用、不可调度等）时调用，
// 以便下次请求能够重新选择可用账号。
//
// DeleteSessionAccountID removes the sticky session binding for the given session.
// Called when the bound account becomes unavailable (e.g., error status, disabled,
// or unschedulable), allowing subsequent requests to select a new available account.
func (c *gatewayCache) DeleteSessionAccountID(ctx context.Context, groupID int64, sessionHash string) error {
	key := buildSessionKey(groupID, sessionHash)
	return c.rdb.Del(ctx, key).Err()
}

func openAIReasoningSourceKey(projectID, groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%d:%s", openAIReasoningSourcePrefix, projectID, groupID, sessionHash)
}

func (c *gatewayCache) GetOpenAIReasoningSourcePassthrough(
	ctx context.Context,
	projectID int64,
	groupID int64,
	sessionHash string,
) (passthrough bool, found bool, err error) {
	value, err := c.rdb.Get(ctx, openAIReasoningSourceKey(projectID, groupID, sessionHash)).Result()
	if errors.Is(err, redis.Nil) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	passthrough, err = strconv.ParseBool(value)
	return passthrough, err == nil, err
}

func (c *gatewayCache) SetOpenAIReasoningSourcePassthrough(
	ctx context.Context,
	projectID int64,
	groupID int64,
	sessionHash string,
	passthrough bool,
	ttl time.Duration,
) error {
	key := openAIReasoningSourceKey(projectID, groupID, sessionHash)
	if passthrough {
		return c.rdb.Set(ctx, key, true, ttl).Err()
	}
	created, err := c.rdb.SetNX(ctx, key, false, ttl).Result()
	if err != nil || created {
		return err
	}
	return c.rdb.Expire(ctx, key, ttl).Err()
}

const (
	grokVideoPendingBillingPrefix = "grok_video_pending:"
	grokVideoBilledPrefix         = "grok_video_billed:"
)

func (c *gatewayCache) SetGrokVideoPendingBilling(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	if c == nil || c.rdb == nil {
		return errors.New("gateway cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" || len(payload) == 0 {
		return errors.New("invalid grok video pending billing payload")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return c.rdb.Set(ctx, grokVideoPendingBillingPrefix+key, payload, ttl).Err()
}

func (c *gatewayCache) GetGrokVideoPendingBilling(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("gateway cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("invalid grok video pending billing key")
	}
	val, err := c.rdb.Get(ctx, grokVideoPendingBillingPrefix+key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	return val, nil
}

func (c *gatewayCache) ClaimGrokVideoBilled(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil {
		return false, errors.New("gateway cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return false, errors.New("invalid grok video billed key")
	}
	if ttl <= 0 {
		ttl = 48 * time.Hour
	}
	return c.rdb.SetNX(ctx, grokVideoBilledPrefix+key, "1", ttl).Result()
}

func (c *gatewayCache) ReleaseGrokVideoBilled(ctx context.Context, key string) error {
	if c == nil || c.rdb == nil {
		return errors.New("gateway cache unavailable")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("invalid grok video billed key")
	}
	return c.rdb.Del(ctx, grokVideoBilledPrefix+key).Err()
}

// Compile-time assertion: gatewayCache must implement CyberSessionBlockStore.
var _ service.CyberSessionBlockStore = (*gatewayCache)(nil)
var _ service.LiveCallStore = (*gatewayCache)(nil)
var _ service.OpenAIReasoningSourceStore = (*gatewayCache)(nil)

const cyberSessionBlockPrefix = "cyber_session_block:"

// SetCyberSessionBlocked 把被 cyber_policy 命中的会话写入屏蔽表（TTL 自动过期）。
// 存储值 "1" 作为存在标记（IsCyberSessionBlocked 只检查 key 是否存在，不读值）。
func (c *gatewayCache) SetCyberSessionBlocked(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Set(ctx, cyberSessionBlockPrefix+key, "1", ttl).Err()
}

// IsCyberSessionBlocked 查询会话是否在屏蔽表中。
func (c *gatewayCache) IsCyberSessionBlocked(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, cyberSessionBlockPrefix+key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

var claimLiveControllerScript = redis.NewScript(`
	local key = KEYS[1]
	local target = ARGV[1]
	local owner = ARGV[2]
	local current = redis.call('HGET', key, 'controller')
	if current == false or current == 'closed' then
		return 0
	end
	if target == 'observer' and current ~= 'pending' then
		return 0
	end
	if target == 'proxy' and current ~= 'pending' and current ~= 'observer' and
		(current ~= 'proxy' or redis.call('HGET', key, 'controller_owner') ~= owner) then
		return 0
	end
	redis.call('HSET', key, 'controller', target, 'controller_owner', owner)
	return 1
`)

var markLiveCallClosedScript = redis.NewScript(`
	local key = KEYS[1]
	if redis.call('EXISTS', key) == 0 then
		redis.call('ZREM', KEYS[2], ARGV[2])
		return 0
	end
	if redis.call('HGET', key, 'controller') == 'closed' then
		redis.call('ZREM', KEYS[2], ARGV[2])
		return 2
	end
	redis.call('HSET', key, 'controller', 'closed', 'controller_owner', '')
	redis.call('EXPIRE', key, ARGV[1])
	redis.call('ZREM', KEYS[2], ARGV[2])
	return 1
`)

var freezeLiveCallFinishedAtScript = redis.NewScript(`
	local key = KEYS[1]
	if redis.call('EXISTS', key) == 0 then
		return 0
	end
	local finished_at = redis.call('HGET', key, 'finished_at')
	if finished_at == false then
		finished_at = ARGV[1]
		redis.call('HSET', key, 'finished_at', finished_at)
	end
	return finished_at
`)

var releaseLiveControllerScript = redis.NewScript(`
	local key = KEYS[1]
	if redis.call('HGET', key, 'controller') ~= 'proxy' or
		redis.call('HGET', key, 'controller_owner') ~= ARGV[1] then
		return 0
	end
	redis.call('HSET', key, 'controller', 'pending', 'controller_owner', '')
	return 1
`)

func liveCallKey(callHash string) string {
	return liveCallPrefix + callHash
}

func HashLiveCallID(callID string) string {
	sum := sha256.Sum256([]byte(callID))
	return hex.EncodeToString(sum[:])
}

func liveCallValues(record *service.LiveCallRecord) map[string]any {
	provisional := 0
	if record.Provisional {
		provisional = 1
	}
	values := map[string]any{
		"call_id":          record.CallID,
		"provisional":      provisional,
		"account_id":       record.AccountID,
		"api_key_id":       record.APIKeyID,
		"project_id":       record.ProjectID,
		"session_id":       record.SessionID,
		"user_id":          record.UserID,
		"group_id":         record.GroupID,
		"subscription_id":  record.SubscriptionID,
		"lease_id":         record.LeaseID,
		"model":            record.Model,
		"created_at":       record.CreatedAt.UnixMilli(),
		"expires_at":       record.ExpiresAt.UnixMilli(),
		"controller":       record.Controller,
		"controller_owner": record.ControllerOwner,
		"user_agent":       record.UserAgent,
		"ip_address":       record.IPAddress,
		"inbound_endpoint": record.InboundEndpoint,
		"attestation":      record.AttestationCiphertext,
	}
	if !record.FinishedAt.IsZero() {
		values["finished_at"] = record.FinishedAt.UnixMilli()
	}
	return values
}

func (c *gatewayCache) SaveLiveCall(ctx context.Context, record *service.LiveCallRecord, ttl time.Duration) error {
	if record == nil || record.CallHash == "" || record.CallID == "" || ttl <= 0 {
		return fmt.Errorf("invalid live call record")
	}
	key := liveCallKey(record.CallHash)
	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, key, liveCallValues(record))
	pipe.Expire(ctx, key, ttl)
	pipe.ZAdd(ctx, liveCallRecoveryIndexKey, redis.Z{
		Score:  float64(record.ExpiresAt.UnixMilli()),
		Member: record.CallHash,
	})
	_, err := pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) PromoteLiveCall(ctx context.Context, intentHash string, record *service.LiveCallRecord, ttl time.Duration) error {
	if intentHash == "" || record == nil || record.Provisional || record.CallHash == "" || record.CallID == "" ||
		intentHash == record.CallHash || ttl <= 0 {
		return fmt.Errorf("invalid live call promotion")
	}
	key := liveCallKey(record.CallHash)
	pipe := c.rdb.TxPipeline()
	pipe.HSet(ctx, key, liveCallValues(record))
	pipe.Expire(ctx, key, ttl)
	pipe.ZAdd(ctx, liveCallRecoveryIndexKey, redis.Z{
		Score:  float64(record.ExpiresAt.UnixMilli()),
		Member: record.CallHash,
	})
	pipe.Del(ctx, liveCallKey(intentHash))
	pipe.ZRem(ctx, liveCallRecoveryIndexKey, intentHash)
	_, err := pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) GetLiveCall(ctx context.Context, callHash string) (*service.LiveCallRecord, error) {
	values, err := c.rdb.HGetAll(ctx, liveCallKey(callHash)).Result()
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, service.ErrLiveCallNotFound
	}
	parseInt := func(field string) int64 {
		value, _ := strconv.ParseInt(values[field], 10, 64)
		return value
	}
	createdAt := time.UnixMilli(parseInt("created_at"))
	expiresAt := time.UnixMilli(parseInt("expires_at"))
	var finishedAt time.Time
	if values["finished_at"] != "" {
		finishedAt = time.UnixMilli(parseInt("finished_at"))
	}
	return &service.LiveCallRecord{
		CallID:                values["call_id"],
		CallHash:              callHash,
		Provisional:           parseInt("provisional") == 1,
		AccountID:             parseInt("account_id"),
		APIKeyID:              parseInt("api_key_id"),
		ProjectID:             parseInt("project_id"),
		SessionID:             values["session_id"],
		UserID:                parseInt("user_id"),
		GroupID:               parseInt("group_id"),
		SubscriptionID:        parseInt("subscription_id"),
		LeaseID:               values["lease_id"],
		Model:                 values["model"],
		CreatedAt:             createdAt,
		FinishedAt:            finishedAt,
		ExpiresAt:             expiresAt,
		Controller:            values["controller"],
		ControllerOwner:       values["controller_owner"],
		UserAgent:             values["user_agent"],
		IPAddress:             values["ip_address"],
		InboundEndpoint:       values["inbound_endpoint"],
		AttestationCiphertext: values["attestation"],
	}, nil
}

func (c *gatewayCache) ClaimLiveController(ctx context.Context, callHash, controller, owner string) (bool, error) {
	result, err := claimLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, controller, owner).Int()
	return result == 1, err
}

func (c *gatewayCache) GetLiveController(ctx context.Context, callHash string) (string, error) {
	value, err := c.rdb.HGet(ctx, liveCallKey(callHash), "controller").Result()
	if err == redis.Nil {
		return "", service.ErrLiveCallNotFound
	}
	return value, err
}

func (c *gatewayCache) ReleaseLiveController(ctx context.Context, callHash, owner string) (bool, error) {
	result, err := releaseLiveControllerScript.Run(ctx, c.rdb, []string{liveCallKey(callHash)}, owner).Int()
	return result == 1, err
}

func (c *gatewayCache) FreezeLiveCallFinishedAt(
	ctx context.Context,
	callHash string,
	finishedAt time.Time,
) (time.Time, error) {
	result, err := freezeLiveCallFinishedAtScript.Run(
		ctx,
		c.rdb,
		[]string{liveCallKey(callHash)},
		finishedAt.UnixMilli(),
	).Int64()
	if err != nil {
		return time.Time{}, err
	}
	if result == 0 {
		return time.Time{}, service.ErrLiveCallNotFound
	}
	return time.UnixMilli(result), nil
}

func (c *gatewayCache) MarkLiveCallClosed(ctx context.Context, callHash string, ttl time.Duration) (service.LiveCallCloseStatus, error) {
	result, err := markLiveCallClosedScript.Run(
		ctx,
		c.rdb,
		[]string{liveCallKey(callHash), liveCallRecoveryIndexKey},
		int64(ttl.Seconds()),
		callHash,
	).Int()
	return service.LiveCallCloseStatus(result), err
}

func (c *gatewayCache) ScheduleLiveCallRecovery(ctx context.Context, record *service.LiveCallRecord, at time.Time, ttl time.Duration) error {
	if record == nil || record.CallHash == "" || record.CallID == "" || ttl <= 0 {
		return fmt.Errorf("invalid live call recovery")
	}
	pipe := c.rdb.TxPipeline()
	key := liveCallKey(record.CallHash)
	pipe.HSet(ctx, key, liveCallValues(record))
	pipe.Expire(ctx, key, ttl)
	pipe.ZAdd(ctx, liveCallRecoveryIndexKey, redis.Z{
		Score:  float64(at.UnixMilli()),
		Member: record.CallHash,
	})
	_, err := pipe.Exec(ctx)
	return err
}

func (c *gatewayCache) ListRecoverableLiveCalls(ctx context.Context, before time.Time, limit int64) ([]*service.LiveCallRecord, error) {
	if limit <= 0 {
		return nil, nil
	}
	hashes, err := c.rdb.ZRangeByScore(ctx, liveCallRecoveryIndexKey, &redis.ZRangeBy{
		Min:   "-inf",
		Max:   strconv.FormatInt(before.UnixMilli(), 10),
		Count: limit,
	}).Result()
	if err != nil {
		return nil, err
	}
	records := make([]*service.LiveCallRecord, 0, len(hashes))
	for _, callHash := range hashes {
		record, getErr := c.GetLiveCall(ctx, callHash)
		if errors.Is(getErr, service.ErrLiveCallNotFound) {
			_ = c.rdb.ZRem(ctx, liveCallRecoveryIndexKey, callHash).Err()
			continue
		}
		if getErr != nil {
			return nil, getErr
		}
		if record.Controller == service.LiveControllerClosed {
			_ = c.rdb.ZRem(ctx, liveCallRecoveryIndexKey, callHash).Err()
			continue
		}
		records = append(records, record)
	}
	return records, nil
}
