package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type liveTestFrame struct {
	messageType coderws.MessageType
	payload     []byte
	err         error
}

type liveTestFrameConn struct {
	reads     chan liveTestFrame
	writes    chan liveTestFrame
	closed    chan struct{}
	closeOnce sync.Once
}

func newLiveTestFrameConn() *liveTestFrameConn {
	return &liveTestFrameConn{
		reads:  make(chan liveTestFrame, 8),
		writes: make(chan liveTestFrame, 8),
		closed: make(chan struct{}),
	}
}

func (c *liveTestFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	select {
	case frame := <-c.reads:
		return frame.messageType, frame.payload, frame.err
	case <-c.closed:
		return coderws.MessageText, nil, coderws.CloseError{Code: coderws.StatusNormalClosure}
	case <-ctx.Done():
		return coderws.MessageText, nil, context.Cause(ctx)
	}
}

func (c *liveTestFrameConn) WriteFrame(ctx context.Context, messageType coderws.MessageType, payload []byte) error {
	frame := liveTestFrame{messageType: messageType, payload: append([]byte(nil), payload...)}
	select {
	case c.writes <- frame:
		return nil
	case <-c.closed:
		return errors.New("connection closed")
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (c *liveTestFrameConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func (c *liveTestFrameConn) ReadMessage(ctx context.Context) ([]byte, error) {
	_, payload, err := c.ReadFrame(ctx)
	return payload, err
}

func (c *liveTestFrameConn) Ping(context.Context) error { return nil }

func (c *liveTestFrameConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

type liveTestDialer struct {
	conn    *liveTestFrameConn
	url     string
	headers http.Header
}

func (d *liveTestDialer) Dial(
	_ context.Context,
	wsURL string,
	headers http.Header,
	_ string,
) (openAIWSClientConn, int, http.Header, error) {
	d.url = wsURL
	d.headers = headers.Clone()
	return d.conn, http.StatusSwitchingProtocols, nil, nil
}

type liveTestAccountRepo struct {
	AccountRepository
	account *Account
}

func (r *liveTestAccountRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *liveTestAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]Account, error) {
	return []Account{*r.account}, nil
}

type liveTestStore struct {
	GatewayCache
	mu     sync.Mutex
	record *LiveCallRecord
	// 注入 store 故障（模拟 Redis 抖动），区别于 ErrLiveCallNotFound。
	claimErr         error
	getCallErr       error
	getControllerErr error
	promoteErr       error
	promoteCommits   bool
	freezeFailures   int
	markClosedFails  int
	markClosedCalls  int
	recoveryAt       time.Time
}

func (s *liveTestStore) SaveLiveCall(_ context.Context, record *LiveCallRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *record
	s.record = &copy
	s.recoveryAt = record.ExpiresAt
	return nil
}

func (s *liveTestStore) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}

func (s *liveTestStore) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *liveTestStore) PromoteLiveCall(_ context.Context, intentHash string, record *LiveCallRecord, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.promoteErr != nil && !s.promoteCommits {
		return s.promoteErr
	}
	if s.record == nil || s.record.CallHash != intentHash {
		return ErrLiveCallNotFound
	}
	copy := *record
	s.record = &copy
	s.recoveryAt = record.ExpiresAt
	return s.promoteErr
}

func (s *liveTestStore) GetLiveCall(_ context.Context, callHash string) (*LiveCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getCallErr != nil {
		return nil, s.getCallErr
	}
	if s.record == nil || s.record.CallHash != callHash {
		return nil, ErrLiveCallNotFound
	}
	copy := *s.record
	return &copy, nil
}

func (s *liveTestStore) ClaimLiveController(_ context.Context, callHash, controller, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return false, s.claimErr
	}
	if s.record == nil || s.record.CallHash != callHash || s.record.Controller == LiveControllerClosed {
		return false, nil
	}
	if controller == LiveControllerObserver && s.record.Controller != LiveControllerPending {
		return false, nil
	}
	if controller == LiveControllerProxy && s.record.Controller != LiveControllerPending && s.record.Controller != LiveControllerObserver {
		return false, nil
	}
	s.record.Controller = controller
	s.record.ControllerOwner = owner
	return true, nil
}

func (s *liveTestStore) ReleaseLiveController(_ context.Context, callHash, owner string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil || s.record.CallHash != callHash || s.record.ControllerOwner != owner {
		return false, nil
	}
	s.record.Controller = LiveControllerPending
	s.record.ControllerOwner = ""
	return true, nil
}

func (s *liveTestStore) GetLiveController(_ context.Context, callHash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getControllerErr != nil {
		return "", s.getControllerErr
	}
	if s.record == nil || s.record.CallHash != callHash {
		return "", ErrLiveCallNotFound
	}
	return s.record.Controller, nil
}

func (s *liveTestStore) FreezeLiveCallFinishedAt(
	_ context.Context,
	callHash string,
	finishedAt time.Time,
) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.freezeFailures > 0 {
		s.freezeFailures--
		return time.Time{}, errors.New("redis: i/o timeout")
	}
	if s.record == nil || s.record.CallHash != callHash {
		return time.Time{}, ErrLiveCallNotFound
	}
	if s.record.FinishedAt.IsZero() {
		s.record.FinishedAt = finishedAt
	}
	return s.record.FinishedAt, nil
}

func (s *liveTestStore) MarkLiveCallClosed(_ context.Context, callHash string, _ time.Duration) (LiveCallCloseStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.markClosedCalls++
	if s.markClosedFails > 0 {
		s.markClosedFails--
		return LiveCallCloseMissing, errors.New("redis: i/o timeout")
	}
	if s.record == nil || s.record.CallHash != callHash {
		return LiveCallCloseMissing, nil
	}
	if s.record.Controller == LiveControllerClosed {
		s.recoveryAt = time.Time{}
		return LiveCallCloseAlready, nil
	}
	s.record.Controller = LiveControllerClosed
	s.record.ControllerOwner = ""
	s.recoveryAt = time.Time{}
	return LiveCallCloseFirst, nil
}

func (s *liveTestStore) ScheduleLiveCallRecovery(_ context.Context, record *LiveCallRecord, at time.Time, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *record
	s.record = &copy
	s.recoveryAt = at
	return nil
}

func (s *liveTestStore) ListRecoverableLiveCalls(_ context.Context, before time.Time, limit int64) ([]*LiveCallRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || s.record == nil || s.record.Controller == LiveControllerClosed || s.recoveryAt.After(before) {
		return nil, nil
	}
	copy := *s.record
	return []*LiveCallRecord{&copy}, nil
}

type liveTestConcurrencyCache struct {
	ConcurrencyCache
	mu             sync.Mutex
	releasedLeases map[string]struct{}
	releases       int
}

func (c *liveTestConcurrencyCache) AcquireLiveLease(
	context.Context,
	int64,
	int,
	int64,
	int,
	int64,
	string,
	bool,
) (bool, error) {
	return true, nil
}

func (c *liveTestConcurrencyCache) AcquireAccountSlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (c *liveTestConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func (c *liveTestConcurrencyCache) RefreshLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) (bool, error) {
	return true, nil
}

func (c *liveTestConcurrencyCache) ReleaseLiveLease(
	_ context.Context,
	_ int64,
	_ int64,
	_ int64,
	leaseID string,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.releasedLeases == nil {
		c.releasedLeases = make(map[string]struct{})
	}
	if _, released := c.releasedLeases[leaseID]; released {
		return nil
	}
	c.releasedLeases[leaseID] = struct{}{}
	c.releases++
	return nil
}

type liveTestUsageRepo struct {
	UsageLogRepository
	mu             sync.Mutex
	logs           []*UsageLog
	projectIDs     []int64
	createFailures int
	createCalls    int
}

type liveBlockingUsageRepo struct {
	UsageLogRepository
	started chan struct{}
	once    sync.Once
}

func (r *liveBlockingUsageRepo) Create(ctx context.Context, _ *UsageLog) (bool, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return false, ctx.Err()
}

func (r *liveTestUsageRepo) Create(ctx context.Context, log *UsageLog) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCalls++
	if r.createFailures > 0 {
		r.createFailures--
		return false, errors.New("database unavailable")
	}
	for _, existing := range r.logs {
		if existing.RequestID == log.RequestID && existing.APIKeyID == log.APIKeyID {
			return false, nil
		}
	}
	copy := *log
	r.logs = append(r.logs, &copy)
	projectID, _ := ProjectIDFromContext(ctx)
	r.projectIDs = append(r.projectIDs, projectID)
	return true, nil
}

func TestRunLiveControllerClosesExpiredSession(t *testing.T) {
	upstream := newLiveTestFrameConn()
	record := &LiveCallRecord{ExpiresAt: time.Now().Add(20 * time.Millisecond)}
	service := &OpenAIGatewayService{}

	err := service.runLiveController(context.Background(), record, upstream, make(chan error))
	require.ErrorIs(t, err, context.DeadlineExceeded)

	select {
	case frame := <-upstream.writes:
		require.Equal(t, coderws.MessageText, frame.messageType)
		require.JSONEq(t, `{"type":"session.close"}`, string(frame.payload))
	case <-time.After(time.Second):
		t.Fatal("没有向上游发送 session.close")
	}
}

func TestFinalizeLiveCallIsIdempotentAndWritesZeroUsage(t *testing.T) {
	record := &LiveCallRecord{
		CallID:          "call_secret",
		CallHash:        hashLiveCallID("call_secret"),
		AccountID:       11,
		APIKeyID:        22,
		ProjectID:       169,
		SessionID:       "sess-live-usage",
		UserID:          33,
		GroupID:         44,
		LeaseID:         "lease-1",
		Model:           "gpt-live-test",
		CreatedAt:       time.Now().Add(-time.Second),
		ExpiresAt:       time.Now().Add(time.Hour),
		Controller:      LiveControllerPending,
		InboundEndpoint: "/v1/live",
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	service := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	service.finalizeLiveCall(record)
	service.finalizeLiveCall(record)

	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	log := usageRepo.logs[0]
	require.Equal(t, []int64{record.ProjectID}, usageRepo.projectIDs)
	usageRepo.mu.Unlock()
	require.Equal(t, record.ProjectID, log.ProjectID)
	require.NotNil(t, log.SessionID)
	require.Equal(t, record.SessionID, *log.SessionID)
	require.Equal(t, RequestTypeLive, log.RequestType)
	require.Equal(t, record.CallHash, log.RequestID)
	require.NotEqual(t, record.CallID, log.RequestID)
	require.NotNil(t, log.DurationMs)
	require.Zero(t, log.InputTokens)
	require.Zero(t, log.OutputTokens)
	require.Zero(t, log.TotalCost)
	require.Zero(t, log.ActualCost)
}

func TestFinalizeLiveCallMissingStoreRecordFallsBackToDatabaseIdempotency(t *testing.T) {
	record := &LiveCallRecord{
		CallHash:  hashLiveCallID("call_missing_store_record"),
		AccountID: 11,
		APIKeyID:  22,
		UserID:    33,
		LeaseID:   "lease-1",
		Model:     "gpt-live-test",
		CreatedAt: time.Now().Add(-time.Second),
	}
	store := &liveTestStore{}
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCall(record)

	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, record.CallHash, usageRepo.logs[0].RequestID)
	usageRepo.mu.Unlock()
}

func TestFinalizeLiveCallClosedStoreRecordFallsBackToDatabaseIdempotency(t *testing.T) {
	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_closed_store_record"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Second),
		Controller: LiveControllerClosed,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCall(record)

	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, record.CallHash, usageRepo.logs[0].RequestID)
	usageRepo.mu.Unlock()
}

func TestGetLiveCallForIdentityRejectsMismatchedCaller(t *testing.T) {
	groupID := int64(44)
	record := &LiveCallRecord{
		CallID:     "call_identity",
		CallHash:   hashLiveCallID("call_identity"),
		APIKeyID:   22,
		ProjectID:  55,
		UserID:     33,
		GroupID:    groupID,
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	service := &OpenAIGatewayService{cache: store}

	_, err := service.GetLiveCallForIdentity(context.Background(), record.CallID, LiveCallIdentity{
		APIKeyID:  99,
		ProjectID: record.ProjectID,
		UserID:    record.UserID,
		GroupID:   &groupID,
	})
	require.ErrorIs(t, err, ErrLiveIdentityMismatch)

	_, err = service.GetLiveCallForIdentity(context.Background(), record.CallID, LiveCallIdentity{
		APIKeyID:  record.APIKeyID,
		ProjectID: 99,
		UserID:    record.UserID,
		GroupID:   &groupID,
	})
	require.ErrorIs(t, err, ErrLiveIdentityMismatch)

	loaded, err := service.GetLiveCallForIdentity(context.Background(), record.CallID, LiveCallIdentity{
		APIKeyID:  record.APIKeyID,
		ProjectID: record.ProjectID,
		UserID:    record.UserID,
		GroupID:   &groupID,
	})
	require.NoError(t, err)
	require.Equal(t, record.AccountID, loaded.AccountID)
}

func TestProxyLiveSidebandForwardsTextAndBinary(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	account := &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	record := &LiveCallRecord{
		CallID:     "call_proxy",
		CallHash:   hashLiveCallID("call_proxy"),
		AccountID:  account.ID,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		CreatedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Minute),
		Controller: LiveControllerPending,
	}
	attestationCipher := newLiveAttestationCipher(&config.Config{
		JWT: config.JWTConfig{Secret: "live-sideband-test-secret"},
	})
	var err error
	record.AttestationCiphertext, err = attestationCipher.Encrypt(`{"v":1,"s":0,"t":"v1.sideband"}`)
	require.NoError(t, err)
	store := &liveTestStore{markClosedFails: 2}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	upstream := newLiveTestFrameConn()
	dialer := &liveTestDialer{conn: upstream}
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	service := &OpenAIGatewayService{
		accountRepo:               &liveTestAccountRepo{account: account},
		cache:                     store,
		concurrencyService:        NewConcurrencyService(concurrencyCache),
		usageLogRepo:              usageRepo,
		openaiWSPassthroughDialer: dialer,
		liveAttestationCipher:     attestationCipher,
	}
	proxyResult := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		downstream, err := coderws.Accept(writer, request, nil)
		if err != nil {
			proxyResult <- err
			return
		}
		defer func() { _ = downstream.CloseNow() }()
		proxyResult <- service.ProxyLiveSideband(request.Context(), record, downstream)
	}))
	defer server.Close()

	client, _, err := coderws.Dial(
		context.Background(),
		"ws"+strings.TrimPrefix(server.URL, "http"),
		nil,
	)
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(`{"type":"client.text"}`)))
	clientText := <-upstream.writes
	require.Equal(t, coderws.MessageText, clientText.messageType)
	require.JSONEq(t, `{"type":"client.text"}`, string(clientText.payload))

	require.NoError(t, client.Write(ctx, coderws.MessageBinary, []byte{1, 2, 3}))
	clientBinary := <-upstream.writes
	require.Equal(t, coderws.MessageBinary, clientBinary.messageType)
	require.Equal(t, []byte{1, 2, 3}, clientBinary.payload)

	upstream.reads <- liveTestFrame{messageType: coderws.MessageText, payload: []byte(`{"type":"server.text"}`)}
	messageType, payload, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageText, messageType)
	require.JSONEq(t, `{"type":"server.text"}`, string(payload))

	upstream.reads <- liveTestFrame{messageType: coderws.MessageBinary, payload: []byte{4, 5, 6}}
	messageType, payload, err = client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, coderws.MessageBinary, messageType)
	require.Equal(t, []byte{4, 5, 6}, payload)

	require.Equal(t, "wss://chatgpt.com/backend-api/codex/call_proxy", dialer.url)
	require.Equal(t, "Bearer test-access-token", dialer.headers.Get("Authorization"))
	require.Equal(t, "acct_test", dialer.headers.Get("Chatgpt-Account-Id"))
	require.Equal(t, `{"v":1,"s":0,"t":"v1.sideband"}`, dialer.headers.Get(liveAttestationHeader))
	upstream.reads <- liveTestFrame{err: coderws.CloseError{Code: coderws.StatusNormalClosure}}
	require.ErrorIs(t, <-proxyResult, ErrLiveCallNotFound)
	store.mu.Lock()
	require.Equal(t, 3, store.markClosedCalls)
	store.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	usageRepo.mu.Unlock()
}

// TestLiveSessionEndedTreatsLeaseLossAsTerminal 锁定：租约续租失败（ErrLiveUnavailable）
// 必须判为会话终结。RefreshLiveLease 的 Lua 在 leaseID 被 GC 后不会重新写入，若把它
// 当临时错误交给 observer 重连，会话会空转到 ExpiresAt 且不计入任何并发限制。
func TestLiveSessionEndedTreatsLeaseLossAsTerminal(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"租约丢失", ErrLiveUnavailable, true},
		{"租约丢失（被包装）", fmt.Errorf("refresh live lease: %w", ErrLiveUnavailable), true},
		{"上游报告会话已关闭", ErrLiveCallNotFound, true},
		{"到达会话时长上限", context.DeadlineExceeded, true},
		{"控制权被他人接管", ErrLiveControllerChanged, false},
		{"临时读错误", errors.New("unexpected EOF"), false},
		{"无错误", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, liveSessionEnded(tc.err))
		})
	}
}

// TestWaitForLiveObserverRetryLeavesExpiryToLoopFinalize 锁定：已过期但控制权仍在
// observer 手上时返回 true，让调用方回到 observeLiveCall 循环顶部的过期分支去
// finalize（写 usage log + 释放租约）。在此处直接返回 false 会让会话静默结束、不留记录。
func TestWaitForLiveObserverRetryLeavesExpiryToLoopFinalize(t *testing.T) {
	record := &LiveCallRecord{
		CallID:     "call_expired",
		CallHash:   hashLiveCallID("call_expired"),
		Controller: LiveControllerObserver,
		ExpiresAt:  time.Now().Add(-time.Minute),
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	svc := &OpenAIGatewayService{cache: store}

	require.True(t, svc.waitForLiveObserverRetry(record),
		"过期判定必须留给循环顶部，否则不会写 usage log")

	// 控制权已被他人接管时仍必须停止重试，避免与新控制者抢同一个 call。
	require.NoError(t, store.SaveLiveCall(context.Background(), &LiveCallRecord{
		CallID:     record.CallID,
		CallHash:   record.CallHash,
		Controller: LiveControllerProxy,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, time.Hour))
	require.False(t, svc.waitForLiveObserverRetry(record))
}

// TestWaitForLiveObserverRetryTreatsStoreErrorAsRetryable 锁定：store 报错（Redis
// 抖动）不等于控制权被接管，必须返回 true 交回 observeLiveCall 循环顶部，由它做
// 有限次重试与 ExpiresAt 兜底 finalize；记录丢失也要回到循环完成数据库幂等终结。
func TestWaitForLiveObserverRetryTreatsStoreErrorAsRetryable(t *testing.T) {
	record := &LiveCallRecord{
		CallID:     "call_flaky_store",
		CallHash:   hashLiveCallID("call_flaky_store"),
		Controller: LiveControllerObserver,
		ExpiresAt:  time.Now().Add(time.Hour),
	}
	store := &liveTestStore{getControllerErr: errors.New("redis: connection refused")}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	svc := &OpenAIGatewayService{cache: store}

	require.True(t, svc.waitForLiveObserverRetry(record),
		"store 报错必须继续重试，否则 Redis 抖动会让会话静默结束、不留记录")

	require.True(t, (&OpenAIGatewayService{cache: &liveTestStore{}}).waitForLiveObserverRetry(record))
}

func TestObserveLiveCallMissingStoreRecordFallsBackToDatabaseIdempotency(t *testing.T) {
	record := &LiveCallRecord{
		CallHash:  hashLiveCallID("call_observer_missing_store_record"),
		AccountID: 11,
		APIKeyID:  22,
		UserID:    33,
		LeaseID:   "lease-1",
		Model:     "gpt-live-test",
		CreatedAt: time.Now().Add(-time.Second),
	}
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              &liveTestStore{},
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.observeLiveCall(record)

	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, record.CallHash, usageRepo.logs[0].RequestID)
	usageRepo.mu.Unlock()
}

// TestObserveLiveCallStoreOutageFallsBackToExpiryFinalize 锁定：observer 遇到持续
// store 报错时不能静默退出，必须按 record.ExpiresAt 兜底 finalize（写 usage log +
// 释放租约）。
func TestObserveLiveCallStoreOutageFallsBackToExpiryFinalize(t *testing.T) {
	restore := liveObserverStoreRetryInterval
	liveObserverStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveObserverStoreRetryInterval = restore })

	cases := []struct {
		name   string
		inject func(*liveTestStore)
	}{
		{"GetLiveCall 持续报错", func(s *liveTestStore) { s.getCallErr = errors.New("redis: i/o timeout") }},
		{"ClaimLiveController 报错", func(s *liveTestStore) { s.claimErr = errors.New("redis: i/o timeout") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := &LiveCallRecord{
				CallID:     "call_store_outage",
				CallHash:   hashLiveCallID("call_store_outage"),
				AccountID:  11,
				APIKeyID:   22,
				UserID:     33,
				LeaseID:    "lease-1",
				Model:      "gpt-live-test",
				CreatedAt:  time.Now().Add(-time.Minute),
				ExpiresAt:  time.Now().Add(-time.Second), // 已到期：兜底无需等待
				Controller: LiveControllerPending,
			}
			store := &liveTestStore{}
			require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
			tc.inject(store)
			concurrencyCache := &liveTestConcurrencyCache{}
			usageRepo := &liveTestUsageRepo{}
			svc := &OpenAIGatewayService{
				cache:              store,
				concurrencyService: NewConcurrencyService(concurrencyCache),
				usageLogRepo:       usageRepo,
			}

			svc.observeLiveCall(record)

			concurrencyCache.mu.Lock()
			require.Equal(t, 1, concurrencyCache.releases, "store 故障时租约释放不能丢")
			concurrencyCache.mu.Unlock()
			usageRepo.mu.Lock()
			require.Len(t, usageRepo.logs, 1, "store 故障时 usage log 不能丢")
			require.Equal(t, RequestTypeLive, usageRepo.logs[0].RequestType)
			usageRepo.mu.Unlock()
		})
	}
}

func TestObserveLiveCallExpiryRetriesStoreFailure(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_observer_finalize_retry"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(-time.Second),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{markClosedFails: 2}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.observeLiveCall(record)

	store.mu.Lock()
	require.Equal(t, 3, store.markClosedCalls)
	store.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	usageRepo.mu.Unlock()
}

func TestFinalizeLiveCallAfterExpiryRetriesStoreFailure(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	record := &LiveCallRecord{
		CallHash:  hashLiveCallID("call_finalize_retry"),
		AccountID: 11,
		APIKeyID:  22,
		UserID:    33,
		LeaseID:   "lease-1",
		Model:     "gpt-live-test",
		CreatedAt: time.Now().Add(-time.Minute),
		ExpiresAt: time.Now().Add(-time.Second),
	}
	store := &liveTestStore{markClosedFails: 2}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCallAfterExpiry(record)

	store.mu.Lock()
	require.Equal(t, 3, store.markClosedCalls)
	store.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	usageRepo.mu.Unlock()
}

func TestFinalizeLiveCallStoreRetryExhaustionPreservesUsageLog(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_finalize_retry_exhausted"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(-time.Second),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{markClosedFails: liveFinalizeStoreRetryLimit}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCall(record)

	store.mu.Lock()
	require.Equal(t, liveFinalizeStoreRetryLimit, store.markClosedCalls)
	require.Equal(t, LiveControllerPending, store.record.Controller)
	require.True(t, store.recoveryAt.After(time.Now()))
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	store.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Zero(t, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1, "Redis 标记持续失败时仍必须写入幂等 usage log")
	require.Equal(t, record.CallHash, usageRepo.logs[0].RequestID)
	usageRepo.mu.Unlock()

	NewLiveCallRecoveryService(svc, time.Second).runOnce()

	store.mu.Lock()
	require.Equal(t, liveFinalizeStoreRetryLimit+1, store.markClosedCalls)
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	require.True(t, store.recoveryAt.IsZero())
	store.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
}

func TestFinalizeLiveCallPersistsUsageBeforeClosingStore(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	record := &LiveCallRecord{
		CallID:     "call_usage_ordering",
		CallHash:   hashLiveCallID("call_usage_ordering"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Second),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	usageRepo := &liveTestUsageRepo{createFailures: 2}
	concurrencyCache := &liveTestConcurrencyCache{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCall(record)

	store.mu.Lock()
	require.Equal(t, 1, store.markClosedCalls)
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
	usageRepo.mu.Lock()
	require.Equal(t, 3, usageRepo.createCalls)
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, record.CallHash, usageRepo.logs[0].RequestID)
	usageRepo.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
}

func TestFinalizeLiveCallSchedulesRecoveryUntilUsagePersists(t *testing.T) {
	restore := liveFinalizeStoreRetryInterval
	liveFinalizeStoreRetryInterval = time.Millisecond
	t.Cleanup(func() { liveFinalizeStoreRetryInterval = restore })

	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_usage_failure"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Second),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	usageRepo := &liveTestUsageRepo{createFailures: liveFinalizeStoreRetryLimit}
	concurrencyCache := &liveTestConcurrencyCache{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(concurrencyCache),
		usageLogRepo:       usageRepo,
	}

	svc.finalizeLiveCall(record)

	store.mu.Lock()
	require.Zero(t, store.markClosedCalls)
	require.Equal(t, LiveControllerPending, store.record.Controller)
	require.True(t, store.recoveryAt.After(time.Now()))
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	store.mu.Unlock()
	usageRepo.mu.Lock()
	require.Equal(t, liveFinalizeStoreRetryLimit, usageRepo.createCalls)
	require.Empty(t, usageRepo.logs)
	usageRepo.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Zero(t, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()

	NewLiveCallRecoveryService(svc, time.Second).runOnce()

	store.mu.Lock()
	require.Equal(t, 1, store.markClosedCalls)
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	require.True(t, store.recoveryAt.IsZero())
	store.mu.Unlock()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	usageRepo.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
}

func TestLiveCallDelayedRecoveryReusesFinishedAt(t *testing.T) {
	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_delayed_recovery"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Second),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{freezeFailures: 1}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	usageRepo := &liveTestUsageRepo{createFailures: 1}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}),
		usageLogRepo:       usageRepo,
	}

	require.False(t, svc.tryFinalizeLiveCall(context.Background(), record))
	finishedAt := record.FinishedAt
	require.False(t, finishedAt.IsZero())
	require.False(t, svc.tryFinalizeLiveCall(context.Background(), record))
	require.NoError(t, store.ScheduleLiveCallRecovery(
		context.Background(),
		record,
		time.Now().Add(-time.Millisecond),
		time.Hour,
	))
	time.Sleep(20 * time.Millisecond)
	NewLiveCallRecoveryService(svc, time.Second).runOnce()

	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, int(finishedAt.Sub(record.CreatedAt).Milliseconds()), *usageRepo.logs[0].DurationMs)
	usageRepo.mu.Unlock()
}

func TestLiveCallRecoveryRecreatesMissingStoreRecordUntilDatabaseRecovers(t *testing.T) {
	record := &LiveCallRecord{
		CallID:     "call_missing_store_recovery",
		CallHash:   hashLiveCallID("call_missing_store_recovery"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Second),
		ExpiresAt:  time.Now().Add(time.Hour),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	usageRepo := &liveTestUsageRepo{createFailures: 2}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}),
		usageLogRepo:       usageRepo,
	}

	require.False(t, svc.tryFinalizeLiveCall(context.Background(), record))
	svc.scheduleLiveCallRecovery(record)
	store.mu.Lock()
	require.NotNil(t, store.record)
	require.Equal(t, record.FinishedAt, store.record.FinishedAt)
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	store.mu.Unlock()

	recovery := NewLiveCallRecoveryService(svc, time.Second)
	recovery.runOnce()
	store.mu.Lock()
	require.NotNil(t, store.record)
	require.NotEqual(t, LiveControllerClosed, store.record.Controller)
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	store.mu.Unlock()

	recovery.runOnce()
	usageRepo.mu.Lock()
	require.Len(t, usageRepo.logs, 1)
	require.Equal(t, int(record.FinishedAt.Sub(record.CreatedAt).Milliseconds()), *usageRepo.logs[0].DurationMs)
	usageRepo.mu.Unlock()
	store.mu.Lock()
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
}

func TestPromoteLiveCallReconcilesCommittedError(t *testing.T) {
	intent := &LiveCallRecord{
		CallID:      "pending:lease-1",
		CallHash:    hashLiveCallID("pending:lease-1"),
		Provisional: true,
		LeaseID:     "lease-1",
	}
	record := *intent
	record.CallID = "call_committed"
	record.CallHash = hashLiveCallID(record.CallID)
	record.Provisional = false
	store := &liveTestStore{promoteErr: errors.New("redis response timeout"), promoteCommits: true}
	require.NoError(t, store.SaveLiveCall(context.Background(), intent, time.Hour))

	require.NoError(t, promoteLiveCall(context.Background(), store, intent.CallHash, &record, time.Hour))
	stored, err := store.GetLiveCall(context.Background(), record.CallHash)
	require.NoError(t, err)
	require.Equal(t, record.LeaseID, stored.LeaseID)
	require.False(t, stored.Provisional)
}

func TestPromoteLiveCallReturnsUncommittedError(t *testing.T) {
	intent := &LiveCallRecord{
		CallID:      "pending:lease-1",
		CallHash:    hashLiveCallID("pending:lease-1"),
		Provisional: true,
		LeaseID:     "lease-1",
	}
	record := *intent
	record.CallID = "call_not_committed"
	record.CallHash = hashLiveCallID(record.CallID)
	record.Provisional = false
	store := &liveTestStore{promoteErr: errors.New("redis unavailable")}
	require.NoError(t, store.SaveLiveCall(context.Background(), intent, time.Hour))

	err := promoteLiveCall(context.Background(), store, intent.CallHash, &record, time.Hour)
	require.ErrorContains(t, err, "redis unavailable")
	stored, getErr := store.GetLiveCall(context.Background(), intent.CallHash)
	require.NoError(t, getErr)
	require.True(t, stored.Provisional)
}

func TestCreateLiveCallPromotionFailureDefersCleanupToRecovery(t *testing.T) {
	account := &Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 2,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	store := &liveTestStore{promoteErr: errors.New("redis unavailable")}
	concurrencyCache := &liveTestConcurrencyCache{}
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		accountRepo:           &liveTestAccountRepo{account: account},
		cache:                 store,
		cfg:                   &config.Config{JWT: config.JWTConfig{Secret: "live-test-secret"}},
		concurrencyService:    NewConcurrencyService(concurrencyCache),
		usageLogRepo:          usageRepo,
		httpUpstream:          &liveHTTPUpstreamStub{},
		liveAttestation:       liveAttestationStub{header: `{"v":1,"s":0,"t":"v1.test"}`},
		liveAttestationCipher: newLiveAttestationCipher(&config.Config{JWT: config.JWTConfig{Secret: "live-test-secret"}}),
	}

	created, err := svc.CreateLiveCall(context.Background(), &LiveCallRequest{
		SDP:     "v=offer\r\n",
		Session: json.RawMessage(`{"model":"gpt-live-test"}`),
	}, LiveCallIdentity{APIKeyID: 22, UserID: 33}, 2)
	require.Nil(t, created)
	require.ErrorContains(t, err, "save live call mapping")
	require.Never(t, func() bool {
		concurrencyCache.mu.Lock()
		defer concurrencyCache.mu.Unlock()
		usageRepo.mu.Lock()
		defer usageRepo.mu.Unlock()
		return concurrencyCache.releases != 0 || len(usageRepo.logs) != 0
	}, 100*time.Millisecond, 10*time.Millisecond)
	store.mu.Lock()
	require.NotNil(t, store.record)
	require.True(t, store.record.Provisional)
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	store.mu.Unlock()

	NewLiveCallRecoveryService(svc, time.Second).runOnce()

	usageRepo.mu.Lock()
	require.Empty(t, usageRepo.logs)
	usageRepo.mu.Unlock()
	concurrencyCache.mu.Lock()
	require.Equal(t, 1, concurrencyCache.releases)
	concurrencyCache.mu.Unlock()
	store.mu.Lock()
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
}

func TestLiveCallRecoveryDiscardsUnconfirmedIntentWithoutUsage(t *testing.T) {
	record := &LiveCallRecord{
		CallID:      "pending:lease-1",
		CallHash:    hashLiveCallID("pending:lease-1"),
		Provisional: true,
		AccountID:   11,
		APIKeyID:    22,
		UserID:      33,
		LeaseID:     "lease-1",
		CreatedAt:   time.Now().Add(-time.Hour),
		ExpiresAt:   time.Now().Add(-time.Second),
		Controller:  LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	store.recoveryAt = time.Now().Add(-time.Millisecond)
	usageRepo := &liveTestUsageRepo{}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}),
		usageLogRepo:       usageRepo,
	}

	NewLiveCallRecoveryService(svc, time.Second).runOnce()

	usageRepo.mu.Lock()
	require.Empty(t, usageRepo.logs)
	usageRepo.mu.Unlock()
	store.mu.Lock()
	require.Equal(t, LiveControllerClosed, store.record.Controller)
	store.mu.Unlock()
}

func TestLiveCallRecoveryStopCancelsInFlightUsageWrite(t *testing.T) {
	record := &LiveCallRecord{
		CallHash:   hashLiveCallID("call_recovery_stop"),
		AccountID:  11,
		APIKeyID:   22,
		UserID:     33,
		LeaseID:    "lease-1",
		Model:      "gpt-live-test",
		CreatedAt:  time.Now().Add(-time.Minute),
		ExpiresAt:  time.Now().Add(-time.Second),
		Controller: LiveControllerPending,
	}
	store := &liveTestStore{}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	usageRepo := &liveBlockingUsageRepo{started: make(chan struct{})}
	svc := &OpenAIGatewayService{
		cache:              store,
		concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}),
		usageLogRepo:       usageRepo,
	}
	recovery := NewLiveCallRecoveryService(svc, time.Hour)
	recovery.Start()

	select {
	case <-usageRepo.started:
	case <-time.After(time.Second):
		t.Fatal("recovery did not start usage persistence")
	}
	done := make(chan struct{})
	go func() {
		recovery.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("recovery stop did not cancel usage persistence")
	}
}
