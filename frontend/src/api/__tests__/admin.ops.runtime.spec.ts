import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get
  },
  buildGatewayUrl: vi.fn((path: string) => path)
}))

import { getUsageRecordRuntimeStats, type UsageRecordRuntimeStats } from '@/api/admin/ops'

describe('admin ops runtime api', () => {
  beforeEach(() => {
    get.mockReset()
  })

  it('reads process-lifetime usage record runtime stats from the admin endpoint', async () => {
    const payload: UsageRecordRuntimeStats = {
      scope: 'process',
      process_started_at: '2026-07-04T00:00:00Z',
      uptime_seconds: 42,
      worker_pool: {
        max_concurrency: 2,
        queue_size: 64,
        running_workers: 1,
        waiting_tasks: 0,
        submitted_tasks: 10,
        completed_tasks: 9,
        successful_tasks: 8,
        failed_tasks: 1,
        dropped_tasks: 0,
        dropped_queue_full: 0,
        dropped_pool_stopped: 0,
        sync_fallback_tasks: 1,
        task_timeout_ms: 2000,
        overflow_policy: 'sync',
        overflow_sample_pct: 0
      },
      persistence: {
        create_not_persisted_total: 0,
        create_dropped_total: 1,
        best_effort_sync_fallback_total: 1,
        best_effort_sync_fallback_succeeded_total: 1,
        best_effort_sync_fallback_failed_total: 0,
        post_usage_billing_timeout_seconds: 5
      }
    }
    get.mockResolvedValue({ data: payload })

    const stats = await getUsageRecordRuntimeStats()

    expect(get).toHaveBeenCalledWith('/admin/ops/runtime/usage-record')
    expect(stats.process_started_at).toBe('2026-07-04T00:00:00Z')
    expect(stats.uptime_seconds).toBe(42)
    expect(stats.worker_pool?.queue_size).toBe(64)
    expect(stats.persistence).not.toHaveProperty('process_started_at')
    expect(stats.persistence.create_dropped_total).toBe(1)
  })
})
