import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const { list, count, getStats, getSnapshotV2, getById, getModelStats, searchModels, listErrorLogs, authState } = await vi.hoisted(async () => {
  const { reactive } = await import('vue')
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })

  return {
    list: vi.fn(),
    count: vi.fn(),
    getStats: vi.fn(),
    getSnapshotV2: vi.fn(),
    getById: vi.fn(),
    getModelStats: vi.fn(),
    searchModels: vi.fn(),
    listErrorLogs: vi.fn(),
    authState: reactive({
      isAdmin: true,
      adminPermissions: ['admin.ops.read'],
      hasAdminPermission(permission: string) {
        return this.isAdmin || this.adminPermissions.includes(permission)
      },
    }),
  }
})

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time Range',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
}

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      count,
      getStats,
    },
    dashboard: {
      getSnapshotV2,
      getModelStats,
    },
    users: {
      getById,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: vi.fn(),
    count,
    searchModels,
  },
}))

vi.mock('@/api/admin/ops', () => ({
  listErrorLogs,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authState,
}))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  })
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const UsageFiltersStub = {
  props: ['showCleanup'],
  template: '<div data-test="usage-filters" :data-show-cleanup="String(showCleanup)"><slot name="after-reset" /></div>',
}
const UsageTableStub = {
  props: ['allowUserDetail'],
  emits: ['userClick'],
  template: '<div data-test="usage-table" :data-allow-user-detail="String(allowUserDetail)"><button class="user-click" @click="$emit(\'userClick\', 2)">user</button></div>',
}
const OpsErrorLogTableStub = {
  props: ['rows', 'userClickable'],
  emits: ['openErrorDetail'],
  template: `
    <div data-test="ops-error-table" :data-user-clickable="String(userClickable)">
      <span v-for="row in rows" :key="row.id">{{ row.message }}</span>
      <button class="open-error" @click="$emit('openErrorDetail', rows[0]?.id)">open</button>
    </div>
  `,
}
const OpsErrorDetailModalStub = {
  props: ['show', 'errorId'],
  template: '<div v-if="show" data-test="ops-error-detail">{{ errorId }}</div>',
}
const UserTokenRankingStub = {
  emits: ['select-user'],
  template: '<div data-test="ranking"><button class="pick-user" @click="$emit(\'select-user\', 5, \'rank@test.com\')">pick</button></div>',
}
const ModelDistributionChartStub = {
  props: ['metric', 'source'],
  emits: ['update:metric', 'update:source'],
  template: `
    <div data-test="model-chart">
      <span class="metric">{{ metric }}</span>
      <span class="source">{{ source }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
      <button class="switch-source" @click="$emit('update:source', 'upstream')">source</button>
    </div>
  `,
}
const GroupDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="group-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}

const stubWindowScrollTo = () => {
  Object.defineProperty(window, 'scrollTo', {
    writable: true,
    value: vi.fn(),
  })
}

describe('admin UsageView distribution metric toggles', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    stubWindowScrollTo()
    list.mockReset()
    count.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()
    getModelStats.mockReset()
    searchModels.mockReset()
    listErrorLogs.mockReset()
    authState.isAdmin = true
    authState.adminPermissions = ['admin.ops.read']

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    count.mockResolvedValue({ total: 0 })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({
      trend: [],
      models: [],
      groups: [],
    })
    getModelStats.mockResolvedValue({ models: [] })
    searchModels.mockResolvedValue([{ name: 'claude-3' }, { name: 'gpt-4o' }])
    listErrorLogs.mockResolvedValue({ items: [], total: 0, pages: 0 })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads usage rows with fast pagination and counts exact total separately when needed', async () => {
    let resolveCount: (value: { total: number }) => void = () => {}
    list.mockResolvedValueOnce({
      items: [{ id: 1 }],
      total: 21,
      pages: 2,
    })
    count.mockReturnValueOnce(new Promise((resolve) => {
      resolveCount = resolve
    }))

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          AuditLogModal: true,
          Pagination: {
            props: ['total', 'totalKnown', 'totalLoading', 'hasNextPage'],
            template: '<div data-test="pagination" :data-total="total" :data-total-known="String(totalKnown)" :data-total-loading="String(totalLoading)" :data-has-next-page="String(hasNextPage)" />',
          },
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          EndpointDistributionChart: true,
        },
      },
    })

    await flushPromises()

    expect(list.mock.calls[0]?.[0]).toEqual(expect.objectContaining({ exact_total: false }))
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total-known')).toBe('false')
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total-loading')).toBe('true')
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total')).toBe('0')

    await flushPromises()

    expect(count).toHaveBeenCalledWith(
      expect.objectContaining({ exact_total: false }),
      expect.objectContaining({ signal: expect.any(AbortSignal), timeout: expect.any(Number) })
    )
    resolveCount({ total: 25 })
    await flushPromises()

    expect(wrapper.get('[data-test="pagination"]').attributes('data-total-known')).toBe('true')
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total')).toBe('25')
    wrapper.unmount()
  })

  it('reuses the exact total while only the usage page changes', async () => {
    list
      .mockResolvedValueOnce({
        items: [{ id: 1 }],
        total: 21,
        pages: 2,
      })
      .mockResolvedValueOnce({
        items: [{ id: 2 }],
        total: 41,
        pages: 3,
      })
      .mockResolvedValueOnce({
        items: [{ id: 2 }],
        total: 41,
        pages: 3,
      })
    count
      .mockResolvedValueOnce({ total: 45 })
      .mockResolvedValueOnce({ total: 46 })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          AuditLogModal: true,
          Pagination: {
            props: ['total', 'totalKnown', 'totalLoading', 'hasNextPage'],
            template: '<div data-test="pagination" :data-total="total" :data-total-known="String(totalKnown)" :data-total-loading="String(totalLoading)" :data-has-next-page="String(hasNextPage)" />',
          },
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          EndpointDistributionChart: true,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(count).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total')).toBe('45')

    ;(wrapper.vm as any).handlePageChange(2)
    await flushPromises()
    await flushPromises()

    expect(list.mock.calls[1]?.[0]).toEqual(expect.objectContaining({ page: 2 }))
    expect(count).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total')).toBe('45')

    ;(wrapper.vm as any).refreshData()
    await flushPromises()
    await flushPromises()

    expect(count).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="pagination"]').attributes('data-total')).toBe('46')
    wrapper.unmount()
  })

  it('keeps previous model stats visible during refresh until new data arrives', async () => {
    // 首次加载返回 A
    getModelStats.mockResolvedValueOnce({ models: [{ model: 'A', total_tokens: 10 }] })

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: ModelDistributionChartStub, GroupDistributionChart: GroupDistributionChartStub,
        EndpointDistributionChart: true, UserTokenRanking: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'A', total_tokens: 10 }])

    // 刷新:让第二次 getModelStats 处于 pending,断言旧数据 A 仍在(不被清空成 [])
    let resolveSecond: (v: any) => void = () => {}
    getModelStats.mockReturnValueOnce(new Promise((res) => { resolveSecond = res }))
    ;(wrapper.vm as any).refreshData()
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'A', total_tokens: 10 }])

    // 新数据到达后替换为 B
    resolveSecond({ models: [{ model: 'B', total_tokens: 20 }] })
    await flushPromises()
    expect((wrapper.vm as any).requestedModelStats).toEqual([{ model: 'B', total_tokens: 20 }])
  })

  it('keeps model and group metric toggles independent without refetching chart data', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          UserTokenRanking: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))

    const modelChart = wrapper.find('[data-test="model-chart"]')
    const groupChart = wrapper.find('[data-test="group-chart"]')

    expect(modelChart.find('.metric').text()).toBe('tokens')
    expect(groupChart.find('.metric').text()).toBe('tokens')

    await modelChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('tokens')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    await groupChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('actual_cost')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
  })

  it('forwards billing mode to dashboard chart requests', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          EndpointDistributionChart: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    ;(wrapper.vm as any).filters.billing_mode = 'image'
    ;(wrapper.vm as any).applyFilters()
    await flushPromises()

    expect(getModelStats).toHaveBeenLastCalledWith(expect.objectContaining({
      billing_mode: 'image',
      model_source: 'requested',
    }))
    expect(getSnapshotV2).toHaveBeenLastCalledWith(expect.objectContaining({
      billing_mode: 'image',
    }))

    wrapper.unmount()
  })

  it('keeps model source scoped to model stats when source changes', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: true,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
          EndpointDistributionChart: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).not.toHaveBeenLastCalledWith(expect.objectContaining({
      model_source: expect.any(String),
    }))

    await wrapper.get('[data-test="model-chart"] .switch-source').trigger('click')
    await flushPromises()

    expect(getModelStats).toHaveBeenLastCalledWith(expect.objectContaining({
      model_source: 'upstream',
    }))
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })
})

describe('admin UsageView cleanup visibility', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    stubWindowScrollTo()
    list.mockReset()
    count.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    searchModels.mockReset()
    authState.isAdmin = true
    authState.adminPermissions = ['admin.ops.read']

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    count.mockResolvedValue({ total: 0 })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
    searchModels.mockResolvedValue([{ name: 'claude-3' }, { name: 'gpt-4o' }])
  })

  afterEach(() => {
    authState.isAdmin = true
    authState.adminPermissions = ['admin.ops.read']
    vi.useRealTimers()
  })

  it('hides cleanup action for project admins', async () => {
    authState.isAdmin = false

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })

    await flushPromises()

    expect(wrapper.get('[data-test="usage-filters"]').attributes('data-show-cleanup')).toBe('false')
    wrapper.unmount()
  })

  it('hides ops error tab without ops permission', async () => {
    authState.isAdmin = false
    authState.adminPermissions = ['admin.usage.read']

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })

    await flushPromises()

    expect(wrapper.text()).not.toContain('usage.tabs.errors')
    expect(listErrorLogs).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('does not call dashboard APIs when only usage permission is granted', async () => {
    authState.isAdmin = false
    authState.adminPermissions = ['admin.usage.read']

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(list).toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('usage.tabs.ranking')
    expect(getStats).toHaveBeenCalled()
    expect(searchModels).toHaveBeenCalledWith(expect.objectContaining({ model: undefined }))
    expect(getModelStats).not.toHaveBeenCalled()
    expect(getSnapshotV2).not.toHaveBeenCalled()

    ;(wrapper.vm as any).refreshData()
    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(searchModels).toHaveBeenCalledTimes(2)
    expect(getModelStats).not.toHaveBeenCalled()
    expect(getSnapshotV2).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('disables usage row user detail without user management permission', async () => {
    authState.isAdmin = false
    authState.adminPermissions = ['admin.usage.read']

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: UsageTableStub, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(wrapper.get('[data-test="usage-table"]').attributes('data-allow-user-detail')).toBe('false')
    await wrapper.find('[data-test="usage-table"] .user-click').trigger('click')
    await flushPromises()

    expect(getById).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('disables ops error user detail without user management permission', async () => {
    authState.isAdmin = false
    authState.adminPermissions = ['admin.usage.read', 'admin.ops.read']

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: OpsErrorLogTableStub, OpsErrorDetailModal: true,
      } },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="ops-error-table"]').attributes('data-user-clickable')).toBe('false')
    wrapper.unmount()
  })

  it('clears stale ops errors when ops permission is removed', async () => {
    authState.isAdmin = false
    authState.adminPermissions = ['admin.usage.read', 'admin.ops.read']
    listErrorLogs.mockResolvedValueOnce({
      items: [{ id: 99, message: 'stale ops error' }],
      total: 1,
    })

    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: OpsErrorLogTableStub,
        OpsErrorDetailModal: OpsErrorDetailModalStub,
      } },
    })

    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'usage.tabs.errors')!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('stale ops error')
    await wrapper.find('[data-test="ops-error-table"] .open-error').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="ops-error-detail"]').exists()).toBe(true)

    authState.adminPermissions = ['admin.usage.read']
    await flushPromises()

    expect(wrapper.text()).not.toContain('stale ops error')
    expect(wrapper.find('[data-test="ops-error-detail"]').exists()).toBe(false)
    expect((wrapper.vm as any).activeTab).toBe('usage')
    wrapper.unmount()
  })
})

describe('admin UsageView handleUserClick', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    stubWindowScrollTo()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getById.mockReset()
    searchModels.mockReset()
    authState.isAdmin = true
    authState.adminPermissions = ['admin.ops.read']

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    searchModels.mockResolvedValue([{ name: 'claude-3' }])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens user via include_deleted when clicking a usage row user', async () => {
    getById.mockResolvedValue({ id: 2, email: 'd@test.com', deleted_at: '2026-05-28T00:00:00Z' })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          AuditLogModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: true,
          GroupDistributionChart: true,
          EndpointDistributionChart: true,
          UserTokenRanking: true,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    await wrapper.find('[data-test="usage-table"] .user-click').trigger('click')
    await flushPromises()

    expect(getById).toHaveBeenCalledWith(2, true)
  })
})

describe('admin UsageView errors tab filter forwarding', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    stubWindowScrollTo()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    searchModels.mockReset()
    listErrorLogs.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
    searchModels.mockResolvedValue([{ name: 'claude-3' }])
    listErrorLogs.mockResolvedValue({ items: [], total: 0, pages: 0 })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('forwards model/account_id/group_id to listErrorLogs on the errors tab', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: true, OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    // 模拟用户在过滤器里选择了模型/账户/分组
    const vm = wrapper.vm as any
    vm.filters.model = 'gpt-5.3-codex'
    vm.filters.account_id = 7
    vm.filters.group_id = 3
    await flushPromises()

    // 切换到「错误请求」标签（第二个 tab 按钮）触发 loadAdminErrors
    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await flushPromises()

    expect(listErrorLogs).toHaveBeenCalledWith(expect.objectContaining({
      view: 'all',
      model: 'gpt-5.3-codex',
      account_id: 7,
      group_id: 3,
    }))
  })

  it('refreshes only admin errors for error-only filters', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await flushPromises()

    list.mockClear()
    count.mockClear()
    getStats.mockClear()
    getSnapshotV2.mockClear()
    getModelStats.mockClear()
    searchModels.mockClear()
    listErrorLogs.mockClear()

    const vm = wrapper.vm as any
    vm.filters.error_phase = 'auth'
    vm.filters.error_category = 'quota'
    vm.filters.status_code = 429
    vm.applyErrorFilters()
    await flushPromises()

    expect(listErrorLogs).toHaveBeenCalledTimes(1)
    expect(listErrorLogs).toHaveBeenCalledWith(expect.objectContaining({
      phase: 'auth',
      category: 'quota',
      status_codes: '429',
    }))
    expect(list).not.toHaveBeenCalled()
    expect(count).not.toHaveBeenCalled()
    expect(getStats).not.toHaveBeenCalled()
    expect(getSnapshotV2).not.toHaveBeenCalled()
    expect(getModelStats).not.toHaveBeenCalled()
    expect(searchModels).not.toHaveBeenCalled()
  })

  it('refreshes only admin errors for shared filters on the errors tab', async () => {
    const ErrorModeUsageFiltersStub = {
      props: ['modelValue', 'mode'],
      emits: ['update:modelValue', 'error-change'],
      template: `
        <div data-test="usage-filters" :data-mode="mode">
          <button class="set-model" @click="$emit('update:modelValue', { ...modelValue, model: 'gpt-5.3-codex' }); $emit('error-change')">model</button>
          <button class="set-user" @click="$emit('update:modelValue', { ...modelValue, user_id: 2 }); $emit('error-change')">user</button>
          <button class="set-api-key" @click="$emit('update:modelValue', { ...modelValue, api_key_id: 5 }); $emit('error-change')">api</button>
          <button class="set-account" @click="$emit('update:modelValue', { ...modelValue, account_id: 7 }); $emit('error-change')">account</button>
          <button class="set-group" @click="$emit('update:modelValue', { ...modelValue, group_id: 3 }); $emit('error-change')">group</button>
        </div>
      `,
    }
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: ErrorModeUsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, AuditLogModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    await tabs[1].trigger('click')
    await flushPromises()

    list.mockClear()
    count.mockClear()
    getStats.mockClear()
    getSnapshotV2.mockClear()
    getModelStats.mockClear()
    searchModels.mockClear()
    listErrorLogs.mockClear()

    for (const selector of ['.set-model', '.set-user', '.set-api-key', '.set-account', '.set-group']) {
      await wrapper.find(selector).trigger('click')
      await flushPromises()
    }

    expect(listErrorLogs).toHaveBeenCalledTimes(5)
    expect(listErrorLogs).toHaveBeenLastCalledWith(expect.objectContaining({
      model: 'gpt-5.3-codex',
      user_id: 2,
      api_key_id: 5,
      account_id: 7,
      group_id: 3,
    }))
    expect(list).not.toHaveBeenCalled()
    expect(count).not.toHaveBeenCalled()
    expect(getStats).not.toHaveBeenCalled()
    expect(getSnapshotV2).not.toHaveBeenCalled()
    expect(getModelStats).not.toHaveBeenCalled()
    expect(searchModels).not.toHaveBeenCalled()
  })
})

describe('admin UsageView ranking tab', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()

    list.mockResolvedValue({ items: [], total: 0, pages: 0 })
    getStats.mockResolvedValue({
      total_requests: 0, total_input_tokens: 0, total_output_tokens: 0,
      total_cache_tokens: 0, total_tokens: 0, total_cost: 0, total_actual_cost: 0, average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({ trend: [], models: [], groups: [] })
    getModelStats.mockResolvedValue({ models: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('mounts ranking lazily and drill-down sets user filter then jumps back to usage tab', async () => {
    const wrapper = mount(UsageView, {
      global: { stubs: {
        AppLayout: AppLayoutStub, UsageStatsCards: true, UsageFilters: UsageFiltersStub,
        UsageTable: true, UsageExportProgress: true, UsageCleanupDialog: true,
        UserBalanceHistoryModal: true, Pagination: true, Select: true,
        DateRangePicker: true, Icon: true, TokenUsageTrend: true,
        ModelDistributionChart: true, GroupDistributionChart: true, EndpointDistributionChart: true,
        UserTokenRanking: UserTokenRankingStub, OpsErrorLogTable: true, OpsErrorDetailModal: true,
      } },
    })
    vi.advanceTimersByTime(120)
    await flushPromises()

    // 懒挂载:切到排行 tab 前不渲染
    expect(wrapper.find('[data-test="ranking"]').exists()).toBe(false)

    const tabs = wrapper.findAll('[data-testid="usage-detail-tab"]')
    expect(tabs).toHaveLength(3)
    await tabs[2].trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-test="ranking"]').exists()).toBe(true)

    // 下钻:设置 user_id、切回用量明细 tab 并按新筛选重新拉取列表
    list.mockClear()
    await wrapper.find('[data-test="ranking"] .pick-user').trigger('click')
    await flushPromises()

    expect((wrapper.vm as any).activeTab).toBe('usage')
    expect((wrapper.vm as any).filters.user_id).toBe(5)
    expect(list).toHaveBeenCalledWith(expect.objectContaining({ user_id: 5 }), expect.anything())
  })
})
