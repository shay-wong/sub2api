import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  deleteAccount,
  showError,
  showSuccess,
  showInfo
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  deleteAccount: vi.fn(),
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
      delete: deleteAccount,
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
    token: 'test-token',
    isAdmin: true,
    isSimpleMode: false
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

const ImportDataModalStub = {
  props: ['show'],
  emits: ['close', 'imported'],
  template: `
    <div data-test="import-data-modal" :data-show="String(show)">
      <button data-test="emit-imported-keep-open" @click="$emit('imported', { close: false })">partial</button>
      <button data-test="emit-imported-close" @click="$emit('imported')">complete</button>
    </div>
  `
}

const ConfirmDialogStub = {
  props: ['show', 'title', 'message', 'confirmText', 'cancelText', 'danger', 'confirmDisabled'],
  emits: ['confirm', 'cancel'],
  template: `
    <div v-if="show" data-test="confirm-dialog">
      <div data-test="confirm-title">{{ title }}</div>
      <div data-test="confirm-message">{{ message }}</div>
      <slot />
      <button data-test="confirm-cancel" @click="$emit('cancel')">{{ cancelText }}</button>
      <button data-test="confirm-submit" :disabled="confirmDisabled" @click="$emit('confirm')">{{ confirmText }}</button>
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
    deleteAccount.mockReset()

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
    deleteAccount.mockResolvedValue({ message: 'ok' })
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
          ImportDataModal: ImportDataModalStub,
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
          ImportDataModal: ImportDataModalStub,
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

  it('refreshes after partial import success without closing the import dialog', async () => {
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
          ImportDataModal: ImportDataModalStub,
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
    expect(listAccounts).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="import-data-modal"]').attributes('data-show')).toBe('false')

    await wrapper.get('button[title="admin.accounts.moreActions"]').trigger('click')
    const importButton = wrapper.findAll('.account-tools-menu-item')
      .find((button) => button.text().includes('admin.accounts.dataImport'))
    expect(importButton).toBeTruthy()
    await importButton!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="import-data-modal"]').attributes('data-show')).toBe('true')

    await wrapper.get('[data-test="emit-imported-keep-open"]').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="import-data-modal"]').attributes('data-show')).toBe('true')

    await wrapper.get('[data-test="emit-imported-close"]').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-test="import-data-modal"]').attributes('data-show')).toBe('false')
  })

  it('uses the custom confirm dialog for bulk delete and deletes selected accounts', async () => {
    const nativeConfirm = vi.fn()
    vi.stubGlobal('confirm', nativeConfirm)

    try {
      const wrapper = mount(AccountsView, {
        global: {
          stubs: {
            AppLayout: { template: '<div><slot /></div>' },
            TablePageLayout: {
              template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
            },
            DataTable: DataTableStub,
            Pagination: true,
            ConfirmDialog: ConfirmDialogStub,
            AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
            AccountTableFilters: { template: '<div></div>' },
            AccountActionMenu: true,
            AccountBatchTestModal: AccountBatchTestModalStub,
            ImportDataModal: ImportDataModalStub,
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

      await wrapper.get('[data-test="bulk-delete-accounts"]').trigger('click')
      await flushPromises()

      expect(nativeConfirm).not.toHaveBeenCalled()
      const dialog = wrapper.get('[data-test="confirm-dialog"]')
      expect(dialog.text()).toContain('admin.accounts.bulkDeleteTitle')
      expect(dialog.text()).toContain('admin.accounts.bulkDeleteConfirm {"count":2}')
      expect(dialog.text()).toContain('admin.accounts.bulkDeleteSummary {"count":2}')
      expect(dialog.text()).toContain('account-one')
      expect(dialog.text()).toContain('account-two')

      await wrapper.get('[data-test="confirm-submit"]').trigger('click')
      await flushPromises()

      expect(deleteAccount).toHaveBeenCalledTimes(2)
      expect(deleteAccount.mock.calls.map(([id]) => id)).toEqual([1, 2])
      expect(showSuccess).toHaveBeenCalledWith('admin.accounts.bulkDeleteSuccess {"count":2}')
      expect(listAccounts).toHaveBeenCalledTimes(2)
    } finally {
      vi.unstubAllGlobals()
    }
  })
})
