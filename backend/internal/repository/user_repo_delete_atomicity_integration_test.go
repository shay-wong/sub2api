//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// TestUserRepository_DeleteUser_AtomicWithAPIKeys 复现 AdminService.DeleteUser 的事务编排场景：
// 把"tombstone 并删 API Key"(apiKeyRepo.DeleteWithAudit) 与"删 User"(userRepo.Delete) 放进同一个外部事务时，
// userRepo.Delete 必须复用 context 中的事务，而不是用 base client 自起一个独立事务并提前提交。
//
// 用例用"回滚外层事务"来模拟 commit 失败 / 中止：
//   - 修复前：userRepo.Delete 用 base client 自起独立事务并 commit，回滚外层事务后用户仍被删除，
//     而 API Key 随外层事务回滚 → Case 1 断言失败，暴露原子性缺陷（即 issue #3021 的不可恢复状态）。
//   - 修复后：两者落在同一事务，回滚后用户与 API Key 一起恢复。
//
// 关键点：repo 必须用 base client 构造（NewUserRepository/NewAPIKeyRepository），并由本测试手动
// 开启外层事务，这与生产环境 wire 注入的方式一致；不能复用 APIKeyRepoSuite 的 testEntTx
// （那会让 repo 持有 tx client，走的是另一条 ErrTxStarted 复用路径，无法覆盖本场景）。
func TestUserRepository_DeleteUser_AtomicWithAPIKeys(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)

	userRepo := NewUserRepository(client, integrationDB)
	apiKeyRepo := NewAPIKeyRepository(client, integrationDB)
	subRepo := NewUserSubscriptionRepository(client)

	// 已提交的初始数据：1 个用户 + 2 个 active API Key。
	user := mustCreateUser(t, client, &service.User{})
	key1 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-a-%d", user.ID)})
	key2 := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: fmt.Sprintf("sk-atomic-b-%d", user.ID)})
	var subscriptions []*service.UserSubscription
	var groupIDs []int64
	for _, status := range []string{service.SubscriptionStatusActive, service.SubscriptionStatusExpired, service.SubscriptionStatusSuspended, service.SubscriptionStatusRevoked} {
		group := mustCreateGroup(t, client, &service.Group{Name: fmt.Sprintf("delete-user-%d-%s", user.ID, status)})
		groupIDs = append(groupIDs, group.ID)
		sub := mustCreateSubscription(t, client, &service.UserSubscription{
			UserID: user.ID, GroupID: group.ID, Status: service.SubscriptionStatusActive,
			DailyUsageUSD: 1, WeeklyUsageUSD: 2, MonthlyUsageUSD: 3, Notes: "retained history",
		})
		if status == service.SubscriptionStatusRevoked {
			require.NoError(t, subRepo.Delete(ctx, sub.ID))
		} else {
			require.NoError(t, subRepo.UpdateStatus(ctx, sub.ID, status))
			sub.Status = status
			if status == service.SubscriptionStatusExpired {
				sub.ExpiresAt = time.Now().Add(-time.Hour)
				require.NoError(t, subRepo.ExtendExpiry(ctx, sub.ID, sub.ExpiresAt))
			}
		}
		subscriptions = append(subscriptions, sub)
	}
	previouslyRevoked, err := client.UserSubscription.Get(mixins.SkipSoftDelete(ctx), subscriptions[3].ID)
	require.NoError(t, err)

	t.Cleanup(func() {
		// testEntClient 的写入不会自动回滚，best-effort 清理避免污染共享库。
		_, _ = integrationDB.Exec(`DELETE FROM deleted_api_key_audits WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM api_keys WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM user_subscriptions WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
		for _, groupID := range groupIDs {
			_, _ = integrationDB.Exec(`DELETE FROM groups WHERE id = $1`, groupID)
		}
	})

	listParams := pagination.PaginationParams{Page: 1, PageSize: 10}

	// --- Case 1: 外层事务回滚 → 删 Key 与删 User 必须一起回滚 ---
	tx, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx")
	opCtx := dbent.NewTxContext(ctx, tx)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx, user.ID))
	subsInTx, err := subRepo.ListByUserID(opCtx, user.ID)
	require.NoError(t, err)
	require.Empty(t, subsInTx, "订阅撤销必须在外层事务内发生")

	require.NoError(t, tx.Rollback(), "rollback outer tx (模拟 commit 失败/中止)")

	// 已提交视图：不应有任何写入"逃逸"到独立已提交事务。
	gotUser, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err, "回滚后用户必须仍然存在（未被独立事务提前删除）")
	require.Equal(t, user.ID, gotUser.ID)

	keys, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Len(t, keys, 2, "回滚后 2 个 API Key 必须仍为 active")
	subsAfterRollback, err := subRepo.ListByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, subsAfterRollback, 3, "回滚后有效、过期、暂停订阅均未撤销")

	var auditCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Zero(t, auditCount, "回滚后不应有已提交的审计行")

	// --- Case 2: 外层事务提交 → 删 Key 与删 User 一起生效 ---
	tx2, err := client.Tx(ctx)
	require.NoError(t, err, "begin outer tx #2")
	opCtx2 := dbent.NewTxContext(ctx, tx2)

	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key1.ID))
	require.NoError(t, apiKeyRepo.DeleteWithAudit(opCtx2, key2.ID))
	require.NoError(t, userRepo.Delete(opCtx2, user.ID))

	require.NoError(t, tx2.Commit(), "commit outer tx")

	_, err = userRepo.GetByID(ctx, user.ID)
	require.Error(t, err, "提交后用户应被软删除，GetByID 应返回未找到")

	keysAfter, _, err := apiKeyRepo.ListByUserID(ctx, user.ID, listParams, service.APIKeyListFilters{})
	require.NoError(t, err, "ListByUserID")
	require.Empty(t, keysAfter, "提交后 API Key 应全部被软删除")
	subsAfterCommit, err := subRepo.ListByUserID(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, subsAfterCommit, "提交后所有未撤销订阅均应撤销")
	history, result, err := subRepo.List(ctx, listParams, &user.ID, nil, service.SubscriptionStatusRevoked, "", "", "")
	require.NoError(t, err)
	require.Len(t, history, 4, "撤销后仍保留包含已删除用户信息的历史")
	require.EqualValues(t, 4, result.Total)
	for _, sub := range history {
		require.Equal(t, service.SubscriptionStatusRevoked, sub.Status)
		require.NotNil(t, sub.User)
		require.Equal(t, user.ID, sub.User.ID)
	}
	for _, before := range subscriptions {
		after, err := client.UserSubscription.Get(mixins.SkipSoftDelete(ctx), before.ID)
		require.NoError(t, err)
		require.NotNil(t, after.DeletedAt)
		require.Equal(t, before.Status, after.Status, "撤销不覆盖原始状态")
		require.WithinDuration(t, before.StartsAt, after.StartsAt, time.Microsecond)
		require.WithinDuration(t, before.ExpiresAt, after.ExpiresAt, time.Microsecond)
		require.Equal(t, before.DailyUsageUSD, after.DailyUsageUsd)
		require.Equal(t, before.WeeklyUsageUSD, after.WeeklyUsageUsd)
		require.Equal(t, before.MonthlyUsageUSD, after.MonthlyUsageUsd)
		require.NotNil(t, after.Notes)
		require.Equal(t, before.Notes, *after.Notes)
		if before.ID == previouslyRevoked.ID {
			require.Equal(t, previouslyRevoked.DeletedAt, after.DeletedAt, "已撤销记录不重写撤销时间")
		}
	}

	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1`, user.ID).Scan(&auditCount))
	require.Equal(t, 2, auditCount, "提交后应保留不可逆摘要归因记录")

	var plaintextCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM deleted_api_key_audits WHERE user_id = $1 AND key <> ''`, user.ID).Scan(&plaintextCount))
	require.Zero(t, plaintextCount, "提交后不得保留被删 Key 的明文凭据")
}

func TestUserRepository_DeleteUser_RollsBackSubscriptionFailures(t *testing.T) {
	for _, stage := range []string{"subscription", "user"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, integrationDB)))
			user := mustCreateUser(t, client, &service.User{})
			group := mustCreateGroup(t, client, &service.Group{Name: fmt.Sprintf("delete-failure-%d", user.ID)})
			sub := mustCreateSubscription(t, client, &service.UserSubscription{UserID: user.ID, GroupID: group.ID})
			t.Cleanup(func() {
				_, _ = integrationDB.Exec(`DELETE FROM user_subscriptions WHERE user_id = $1`, user.ID)
				_, _ = integrationDB.Exec(`DELETE FROM users WHERE id = $1`, user.ID)
				_, _ = integrationDB.Exec(`DELETE FROM groups WHERE id = $1`, group.ID)
			})
			injectedErr := errors.New("injected delete failure")
			failDelete := func(next dbent.Mutator) dbent.Mutator {
				return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
					return nil, injectedErr
				})
			}
			if stage == "subscription" {
				client.UserSubscription.Use(failDelete)
			} else {
				client.User.Use(failDelete)
			}

			err := NewUserRepository(client, integrationDB).Delete(ctx, user.ID)

			require.ErrorIs(t, err, injectedErr)
			_, err = client.User.Get(ctx, user.ID)
			require.NoError(t, err, "订阅或用户删除失败必须保留用户")
			_, err = client.UserSubscription.Get(ctx, sub.ID)
			require.NoError(t, err, "最终删除用户失败必须回滚此前的订阅撤销")
		})
	}
}
