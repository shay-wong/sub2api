import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const { importData } = vi.hoisted(() => ({
  importData: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('ImportDataModal', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    showError.mockReset()
    showSuccess.mockReset()
    importData.mockReset()
  })

  const mountModal = () => mount(ImportDataModal, {
    attachTo: document.body,
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
      }
    }
  })

  const selectDataFormat = async (wrapper: ReturnType<typeof mount>, optionLabel: string) => {
    await wrapper.get('[data-testid="account-import-format-select"] .select-trigger').trigger('click')
    await flushPromises()

    const option = Array.from(document.body.querySelectorAll<HTMLElement>('[role="option"]'))
      .find((item) => item.textContent?.includes(optionLabel))
    expect(option).toBeTruthy()
    option!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await flushPromises()
  }

  it('未选择文件时提示错误', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时提示解析失败', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = new File(['invalid json'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('invalid json')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await Promise.resolve()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('数据格式选择器使用全宽自定义 Select', () => {
    const wrapper = mountModal()

    const formatSelect = wrapper.get('[data-testid="account-import-format-select"]')
    expect(formatSelect.classes()).toContain('w-full')
    expect(formatSelect.find('.select-trigger').exists()).toBe(true)
    expect(wrapper.find('select').exists()).toBe(false)
  })

  it('选择 CPA 格式时把格式传给后端', async () => {
    importData.mockResolvedValue({
      account_created: 1,
      account_failed: 0,
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0
    })
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCPA')
    const input = wrapper.find('input[type="file"]')
    const file = new File(['{"access_token":"token","account_id":"acct"}'], 'acct.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('{"access_token":"token","account_id":"acct"}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await Promise.resolve()

    expect(importData).toHaveBeenCalledWith({
      data: { access_token: 'token', account_id: 'acct' },
      format: 'cpa',
      skip_default_group_bind: true
    })
  })
})
