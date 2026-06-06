import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  showError,
  showSuccess,
  showInfo
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showInfo
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (
        params ? `${key} ${JSON.stringify(params)}` : key
      )
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  template: `
    <div data-test="data-table">
      <div v-for="row in data" :key="row.id" data-test="account-row">
        <slot name="cell-select" :row="row" />
        <slot name="cell-name" :value="row.name" :row="row" />
      </div>
    </div>
  `
}

const AccountBatchTestModalStub = {
  props: ['show', 'accounts'],
  template: `
    <div data-test="account-batch-test-modal" :data-show="String(show)" :data-count="String(accounts.length)">
      <button data-test="emit-running-start" @click="$emit('running-change', true)">start</button>
      <button data-test="emit-running-stop" @click="$emit('running-change', false)">stop</button>
    </div>
  `
}

describe('admin AccountsView batch test', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_token', 'test-token')
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()
    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()

    listAccounts.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'account-one',
          platform: 'anthropic',
          type: 'oauth',
          status: 'active',
          schedulable: true,
          created_at: '2026-03-07T10:00:00Z',
          updated_at: '2026-03-07T10:00:00Z'
        },
        {
          id: 2,
          name: 'account-two',
          platform: 'openai',
          type: 'apikey',
          status: 'active',
          schedulable: true,
          created_at: '2026-03-07T10:00:00Z',
          updated_at: '2026-03-07T10:00:00Z'
        }
      ],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  it('opens the batch test dialog for selected accounts', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountActionMenu: true,
          AccountBatchTestModal: AccountBatchTestModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: true,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    const rows = wrapper.findAll('[data-test="account-row"]')
    await rows[0].find('input[type="checkbox"]').trigger('change')
    await rows[1].find('input[type="checkbox"]').trigger('change')

    await wrapper.get('[data-test="batch-test-accounts"]').trigger('click')
    await flushPromises()

    const modal = wrapper.get('[data-test="account-batch-test-modal"]')
    expect(modal.attributes('data-show')).toBe('true')
    expect(modal.attributes('data-count')).toBe('2')
  })

  it('shows the batch test button as testing while the dialog is running', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
          AccountTableFilters: { template: '<div></div>' },
          AccountActionMenu: true,
          AccountBatchTestModal: AccountBatchTestModalStub,
          ImportDataModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          SyncFromCrsModal: true,
          TempUnschedStatusModal: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          BulkEditAccountModal: true,
          PlatformTypeBadge: true,
          AccountCapacityCell: true,
          AccountStatusIndicator: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountUsageCell: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    const rows = wrapper.findAll('[data-test="account-row"]')
    await rows[0].find('input[type="checkbox"]').trigger('change')
    await wrapper.get('[data-test="batch-test-accounts"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-test="emit-running-start"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="batch-test-accounts"]').text()).toBe('admin.accounts.bulkActions.testing')
    expect(wrapper.get('[data-test="batch-test-accounts"]').attributes('disabled')).toBeDefined()

    await wrapper.get('[data-test="emit-running-stop"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="batch-test-accounts"]').text()).toBe('admin.accounts.bulkActions.test')
    expect(wrapper.get('[data-test="batch-test-accounts"]').attributes('disabled')).toBeUndefined()
  })
})
