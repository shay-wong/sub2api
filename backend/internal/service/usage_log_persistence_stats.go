package service

import (
	"sync/atomic"
	"time"
)

var (
	usageRecordRuntimeProcessStartedAt           = time.Now().UTC()
	usageLogCreateNotPersistedTotal              atomic.Int64
	usageLogCreateDroppedTotal                   atomic.Int64
	usageLogBestEffortSyncFallbackTotal          atomic.Int64
	usageLogBestEffortSyncFallbackSucceededTotal atomic.Int64
	usageLogBestEffortSyncFallbackFailedTotal    atomic.Int64
)

// UsageLogPersistenceStats 是 usage_log 持久化运行时快照。
// 这些计数器用于 ops/health 面板或告警读取：dropped/not_persisted 表示队列等待或
// DB 写入出现压力，sync_fallback 表示已进入“已扣费后必须补 usage_log”的兜底路径。
type UsageLogPersistenceStats struct {
	CreateNotPersistedTotal              int64 `json:"create_not_persisted_total"`
	CreateDroppedTotal                   int64 `json:"create_dropped_total"`
	BestEffortSyncFallbackTotal          int64 `json:"best_effort_sync_fallback_total"`
	BestEffortSyncFallbackSucceededTotal int64 `json:"best_effort_sync_fallback_succeeded_total"`
	BestEffortSyncFallbackFailedTotal    int64 `json:"best_effort_sync_fallback_failed_total"`
	PostUsageBillingTimeoutSeconds       int64 `json:"post_usage_billing_timeout_seconds"`
}

// UsageRecordRuntimeStats 汇总 usage record worker 与 usage_log 持久化运行时状态。
type UsageRecordRuntimeStats struct {
	// Scope 标明这些计数是当前进程全局状态，不随 ops traffic 的 platform/group filter 缩小。
	Scope string `json:"scope"`
	// ProcessStartedAt / UptimeSeconds make all runtime counters explicitly process-lifetime totals.
	ProcessStartedAt time.Time                   `json:"process_started_at"`
	UptimeSeconds    int64                       `json:"uptime_seconds"`
	WorkerPool       *UsageRecordWorkerPoolStats `json:"worker_pool,omitempty"`
	Persistence      UsageLogPersistenceStats    `json:"persistence"`
}

// GatewayUsageLogPersistenceStats 暴露 usage_log 持久化兜底与失败计数，供 ops/health 面板做阈值告警。
func GatewayUsageLogPersistenceStats() UsageLogPersistenceStats {
	return UsageLogPersistenceStats{
		CreateNotPersistedTotal:              usageLogCreateNotPersistedTotal.Load(),
		CreateDroppedTotal:                   usageLogCreateDroppedTotal.Load(),
		BestEffortSyncFallbackTotal:          usageLogBestEffortSyncFallbackTotal.Load(),
		BestEffortSyncFallbackSucceededTotal: usageLogBestEffortSyncFallbackSucceededTotal.Load(),
		BestEffortSyncFallbackFailedTotal:    usageLogBestEffortSyncFallbackFailedTotal.Load(),
		PostUsageBillingTimeoutSeconds:       int64(postUsageBillingTimeout / time.Second),
	}
}

// GatewayUsageRecordRuntimeStats 暴露 usage record 队列与持久化状态，供 ops runtime endpoint 读取。
func GatewayUsageRecordRuntimeStats(pool *UsageRecordWorkerPool) UsageRecordRuntimeStats {
	now := time.Now().UTC()
	stats := UsageRecordRuntimeStats{
		Scope:            "process",
		ProcessStartedAt: usageRecordRuntimeProcessStartedAt,
		UptimeSeconds:    usageRecordRuntimeUptimeSeconds(now),
		Persistence:      GatewayUsageLogPersistenceStats(),
	}
	if pool != nil {
		poolStats := pool.Stats()
		stats.WorkerPool = &poolStats
	}
	return stats
}

func usageRecordRuntimeUptimeSeconds(now time.Time) int64 {
	uptimeSeconds := int64(now.Sub(usageRecordRuntimeProcessStartedAt) / time.Second)
	if uptimeSeconds < 0 {
		return 0
	}
	return uptimeSeconds
}

func recordUsageLogCreateNotPersisted() {
	usageLogCreateNotPersistedTotal.Add(1)
}

func recordUsageLogCreateDropped() {
	usageLogCreateDroppedTotal.Add(1)
}

func recordUsageLogBestEffortSyncFallback(succeeded bool) {
	usageLogBestEffortSyncFallbackTotal.Add(1)
	if succeeded {
		usageLogBestEffortSyncFallbackSucceededTotal.Add(1)
		return
	}
	usageLogBestEffortSyncFallbackFailedTotal.Add(1)
}

func recordUsageLogCreateError(err error) {
	switch {
	case IsUsageLogCreateDropped(err):
		recordUsageLogCreateDropped()
	case IsUsageLogCreateNotPersisted(err):
		recordUsageLogCreateNotPersisted()
	}
}
