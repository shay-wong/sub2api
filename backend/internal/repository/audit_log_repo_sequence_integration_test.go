//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func resetAuditLogState(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := integrationDB.ExecContext(ctx, "TRUNCATE TABLE audit_logs")
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, "UPDATE audit_log_state SET clear_id = 0 WHERE singleton = TRUE")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "TRUNCATE TABLE audit_logs")
		_, _ = integrationDB.ExecContext(context.Background(), "UPDATE audit_log_state SET clear_id = 0 WHERE singleton = TRUE")
	})
}

func TestAuditLogRepositoryClearWatermarkSurvivesLateBatchesAndTraceRetention(t *testing.T) {
	resetAuditLogState(t)
	ctx := context.Background()
	repoA := NewAuditLogRepository(integrationDB)
	repoB := NewAuditLogRepository(integrationDB)

	oldID1, err := repoA.NextID(ctx)
	require.NoError(t, err)
	oldID2, err := repoB.NextID(ctx)
	require.NoError(t, err)

	trace := &service.AuditLog{Action: service.AuditActionAuditLogClear}
	_, err = repoA.ClearAll(ctx, trace)
	require.NoError(t, err)
	require.Greater(t, trace.ID, oldID2)

	newID1, err := repoB.NextID(ctx)
	require.NoError(t, err)
	inserted, err := repoB.BatchInsert(ctx, []*service.AuditLog{
		{ID: oldID1, Action: "late-before-clear"},
		{ID: newID1, Action: "after-clear"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, inserted)

	_, err = integrationDB.ExecContext(ctx, "DELETE FROM audit_logs WHERE id = $1", trace.ID)
	require.NoError(t, err)
	newID2, err := repoA.NextID(ctx)
	require.NoError(t, err)
	inserted, err = repoA.BatchInsert(ctx, []*service.AuditLog{
		{ID: oldID2, Action: "late-after-trace-retention"},
		{ID: newID2, Action: "after-trace-retention"},
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, inserted)

	var oldCount, newCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_logs WHERE id IN ($1, $2)", oldID1, oldID2,
	).Scan(&oldCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM audit_logs WHERE id IN ($1, $2)", newID1, newID2,
	).Scan(&newCount))
	require.Zero(t, oldCount)
	require.Equal(t, 2, newCount)

	var clearID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT clear_id FROM audit_log_state WHERE singleton = TRUE",
	).Scan(&clearID))
	require.Equal(t, trace.ID, clearID)
}

func TestAuditLogRepositoryClearWaitsForInFlightInsert(t *testing.T) {
	resetAuditLogState(t)
	ctx := context.Background()
	repo := NewAuditLogRepository(integrationDB)

	oldID, err := repo.NextID(ctx)
	require.NoError(t, err)
	insertTx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = insertTx.ExecContext(ctx, "LOCK TABLE audit_logs IN ROW EXCLUSIVE MODE")
	require.NoError(t, err)
	_, err = insertTx.ExecContext(ctx, "INSERT INTO audit_logs (id, action) VALUES ($1, $2)", oldID, "in-flight")
	require.NoError(t, err)

	type clearResult struct {
		trace *service.AuditLog
		err   error
	}
	done := make(chan clearResult, 1)
	go func() {
		trace := &service.AuditLog{}
		_, clearErr := repo.ClearAll(context.Background(), trace)
		done <- clearResult{trace: trace, err: clearErr}
	}()

	select {
	case result := <-done:
		t.Fatalf("ClearAll completed before in-flight insert committed: %v", result.err)
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, insertTx.Commit())

	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Greater(t, result.trace.ID, oldID)
	case <-time.After(3 * time.Second):
		t.Fatal("ClearAll did not complete after insert transaction committed")
	}
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_logs WHERE id = $1", oldID).Scan(&count))
	require.Zero(t, count)
}

func TestAuditLogRepositoryConcurrentClearsFollowDatabaseLockOrder(t *testing.T) {
	resetAuditLogState(t)
	ctx := context.Background()
	repoA := NewAuditLogRepository(integrationDB)
	repoB := NewAuditLogRepository(integrationDB)

	blocker, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = blocker.ExecContext(ctx, "LOCK TABLE audit_logs IN ROW EXCLUSIVE MODE")
	require.NoError(t, err)

	type clearResult struct {
		trace *service.AuditLog
		err   error
	}
	done := make(chan clearResult, 2)
	for _, repo := range []service.AuditLogRepository{repoA, repoB} {
		go func(repo service.AuditLogRepository) {
			trace := &service.AuditLog{Action: service.AuditActionAuditLogClear}
			_, clearErr := repo.ClearAll(context.Background(), trace)
			done <- clearResult{trace: trace, err: clearErr}
		}(repo)
	}
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, blocker.Commit())

	first := <-done
	second := <-done
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	latestID := max(first.trace.ID, second.trace.ID)

	var clearID, traceID int64
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT clear_id FROM audit_log_state WHERE singleton = TRUE",
	).Scan(&clearID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT id FROM audit_logs WHERE action = $1",
		service.AuditActionAuditLogClear,
	).Scan(&traceID))
	require.Equal(t, latestID, clearID)
	require.Equal(t, latestID, traceID)
}
