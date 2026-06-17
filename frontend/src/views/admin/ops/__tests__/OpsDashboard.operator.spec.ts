import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'

import OpsDashboard from '../OpsDashboard.vue'

const {
  mockAuthRole,
  mockRouterReplace,
  mockAdminSettingsFetch,
  mockGetDashboardSnapshotV2,
  mockGetThroughputTrend,
  mockGetLatencyHistogram,
  mockGetErrorDistribution,
  mockGetAdvancedSettings,
  mockGetMetricThresholds,
  mockGetConcurrencyStats,
  mockGetUserConcurrencyStats,
  mockGetAccountAvailabilityStats,
  mockListAlertEvents,
  mockAdminSettingsState
} = vi.hoisted(() => ({
  mockAuthRole: { value: 'admin' as 'admin' | 'user' },
  mockRouterReplace: vi.fn(),
  mockAdminSettingsFetch: vi.fn(),
  mockGetDashboardSnapshotV2: vi.fn(),
  mockGetThroughputTrend: vi.fn(),
  mockGetLatencyHistogram: vi.fn(),
  mockGetErrorDistribution: vi.fn(),
  mockGetAdvancedSettings: vi.fn(),
  mockGetMetricThresholds: vi.fn(),
  mockGetConcurrencyStats: vi.fn(),
  mockGetUserConcurrencyStats: vi.fn(),
  mockGetAccountAvailabilityStats: vi.fn(),
  mockListAlertEvents: vi.fn(),
  mockAdminSettingsState: {
    opsMonitoringEnabled: true,
    opsRealtimeMonitoringEnabled: true,
    opsQueryModeDefault: 'auto'
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  }),
  useRouter: () => ({
    replace: mockRouterReplace
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/stores', () => {
  return {
    useAppStore: () => ({
      showError: vi.fn(),
      showSuccess: vi.fn()
    }),
    useAuthStore: () => ({
      get isAdmin() {
        return mockAuthRole.value === 'admin'
      },
      get isOperator() {
        return false
      }
    }),
    useAdminSettingsStore: () => ({
      get opsMonitoringEnabled() {
        return mockAdminSettingsState.opsMonitoringEnabled
      },
      get opsRealtimeMonitoringEnabled() {
        return mockAdminSettingsState.opsRealtimeMonitoringEnabled
      },
      get opsQueryModeDefault() {
        return mockAdminSettingsState.opsQueryModeDefault
      },
      fetch: mockAdminSettingsFetch,
      setOpsMonitoringEnabledLocal: (value: boolean) => {
        mockAdminSettingsState.opsMonitoringEnabled = value
      },
      setOpsRealtimeMonitoringEnabledLocal: (value: boolean) => {
        mockAdminSettingsState.opsRealtimeMonitoringEnabled = value
      },
      setOpsQueryModeDefaultLocal: (value: string) => {
        mockAdminSettingsState.opsQueryModeDefault = value || 'auto'
      }
    })
  }
})

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getDashboardSnapshotV2: (...args: any[]) => mockGetDashboardSnapshotV2(...args),
    getThroughputTrend: (...args: any[]) => mockGetThroughputTrend(...args),
    getLatencyHistogram: (...args: any[]) => mockGetLatencyHistogram(...args),
    getErrorDistribution: (...args: any[]) => mockGetErrorDistribution(...args),
    getAdvancedSettings: (...args: any[]) => mockGetAdvancedSettings(...args),
    getMetricThresholds: (...args: any[]) => mockGetMetricThresholds(...args),
    getConcurrencyStats: (...args: any[]) => mockGetConcurrencyStats(...args),
    getUserConcurrencyStats: (...args: any[]) => mockGetUserConcurrencyStats(...args),
    getAccountAvailabilityStats: (...args: any[]) => mockGetAccountAvailabilityStats(...args),
    listAlertEvents: (...args: any[]) => mockListAlertEvents(...args)
  }
}))

const HeaderStub = defineComponent({
  name: 'OpsDashboardHeader',
  props: {
    canManageOps: {
      type: Boolean,
      default: false
    }
  },
  emits: ['openSettings', 'openAlertRules'],
  template: `
    <div data-testid="ops-header">
      <button v-if="canManageOps" data-testid="settings" @click="$emit('openSettings')">settings</button>
      <button v-if="canManageOps" data-testid="alert-rules" @click="$emit('openAlertRules')">alert rules</button>
    </div>
  `
})

const ConcurrencyStub = defineComponent({
  name: 'OpsConcurrencyCard',
  props: {
    canViewUserConcurrency: {
      type: Boolean,
      default: false
    }
  },
  template: '<div data-testid="concurrency">{{ String(canViewUserConcurrency) }}</div>'
})

const AlertEventsStub = defineComponent({
  name: 'OpsAlertEventsCard',
  mounted() {
    mockListAlertEvents({ limit: 10, time_range: '24h' })
  },
  template: '<div data-testid="alert-events" />'
})

const componentStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  BaseDialog: { template: '<div data-testid="base-dialog"><slot /></div>' },
  OpsDashboardHeader: HeaderStub,
  OpsDashboardSkeleton: true,
  OpsConcurrencyCard: ConcurrencyStub,
  OpsErrorDetailModal: true,
  OpsErrorDistributionChart: true,
  OpsErrorDetailsModal: true,
  OpsErrorTrendChart: true,
  OpsLatencyChart: true,
  OpsThroughputTrendChart: true,
  OpsSwitchRateTrendChart: true,
  OpsAlertEventsCard: AlertEventsStub,
  OpsOpenAITokenStatsCard: true,
  OpsSystemLogTable: { template: '<div data-testid="system-logs" />' },
  OpsRequestDetailsModal: true,
  OpsSettingsDialog: { template: '<div data-testid="settings-dialog" />' },
  OpsAlertRulesCard: { template: '<div data-testid="alert-rules-card" />' }
}

function setupSuccessfulOpsMocks() {
  mockGetDashboardSnapshotV2.mockResolvedValue({
    overview: null,
    throughput_trend: { points: [], by_platform: [], top_groups: [] },
    error_trend: { points: [] }
  })
  mockGetThroughputTrend.mockResolvedValue({ points: [], by_platform: [], top_groups: [] })
  mockGetLatencyHistogram.mockResolvedValue({ buckets: [], total_requests: 0 })
  mockGetErrorDistribution.mockResolvedValue({ items: [] })
  mockGetConcurrencyStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
  mockGetAccountAvailabilityStats.mockResolvedValue({ enabled: true, platform: {}, group: {}, account: {} })
  mockListAlertEvents.mockResolvedValue([])
  mockGetAdvancedSettings.mockResolvedValue({
    display_alert_events: true,
    display_openai_token_stats: false,
    auto_refresh_enabled: false,
    auto_refresh_interval_seconds: 30
  })
  mockGetMetricThresholds.mockResolvedValue(null)
  mockAdminSettingsFetch.mockResolvedValue(undefined)
}

describe('OpsDashboard admin permissions', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuthRole.value = 'user'
    mockAdminSettingsState.opsMonitoringEnabled = true
    mockAdminSettingsState.opsRealtimeMonitoringEnabled = true
    mockAdminSettingsState.opsQueryModeDefault = 'auto'
    setupSuccessfulOpsMocks()
  })

  it('non-admin loads monitor data without calling admin-only settings endpoints', async () => {
    const wrapper = mount(OpsDashboard, {
      global: {
        stubs: componentStubs
      }
    })

    await flushPromises()
    await nextTick()

    expect(mockAdminSettingsFetch).not.toHaveBeenCalled()
    expect(mockGetAdvancedSettings).not.toHaveBeenCalled()
    expect(mockGetMetricThresholds).not.toHaveBeenCalled()
    expect(mockGetDashboardSnapshotV2).toHaveBeenCalled()
    expect(mockGetThroughputTrend).toHaveBeenCalled()
    expect(mockListAlertEvents).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="settings"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="alert-rules"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="system-logs"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="concurrency"]').text()).toBe('false')
  })

  it('admin keeps management controls and settings loading', async () => {
    mockAuthRole.value = 'admin'

    const wrapper = mount(OpsDashboard, {
      global: {
        stubs: componentStubs
      }
    })

    await flushPromises()
    await nextTick()

    expect(mockAdminSettingsFetch).toHaveBeenCalled()
    expect(mockGetAdvancedSettings).toHaveBeenCalled()
    expect(mockGetMetricThresholds).toHaveBeenCalled()
    expect(wrapper.find('[data-testid="settings"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="alert-rules"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="system-logs"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="concurrency"]').text()).toBe('true')
  })
})
