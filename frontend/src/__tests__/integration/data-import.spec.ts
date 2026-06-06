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
    t: (key: string, params?: Record<string, unknown>) => (
      params ? `${key} ${JSON.stringify(params)}` : key
    )
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

    expect(showError).toHaveBeenCalledWith(expect.stringContaining('data.json'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('admin.accounts.dataImportParseFailed'))
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

  it('可以一次选择多个 JSON 文件并逐个导入', async () => {
    importData
      .mockResolvedValueOnce({
        account_created: 1,
        account_failed: 0,
        proxy_created: 1,
        proxy_reused: 0,
        proxy_failed: 0
      })
      .mockResolvedValueOnce({
        account_created: 2,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 1,
        proxy_failed: 0
      })
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    expect(input.attributes('multiple')).toBeDefined()
    const firstFile = new File(['{"accounts":[{"id":1}]}'], 'first.json', { type: 'application/json' })
    const secondFile = new File(['{"accounts":[{"id":2},{"id":3}]}'], 'second.json', { type: 'application/json' })
    Object.defineProperty(firstFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":1}]}')
    })
    Object.defineProperty(secondFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":2},{"id":3}]}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, secondFile]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(2)
    expect(importData).toHaveBeenNthCalledWith(1, {
      data: { accounts: [{ id: 1 }] },
      format: undefined,
      skip_default_group_bind: true
    })
    expect(importData).toHaveBeenNthCalledWith(2, {
      data: { accounts: [{ id: 2 }, { id: 3 }] },
      format: undefined,
      skip_default_group_bind: true
    })
    const successMessage = showSuccess.mock.calls[0][0]
    expect(successMessage).toContain('"account_created":3')
    expect(successMessage).toContain('"proxy_created":1')
    expect(successMessage).toContain('"proxy_reused":1')
  })

  it('多文件中存在无效 JSON 时不会提前调用导入接口', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const validFile = new File(['{"accounts":[]}'], 'valid.json', { type: 'application/json' })
    const invalidFile = new File(['invalid json'], 'invalid.json', { type: 'application/json' })
    Object.defineProperty(validFile, 'text', {
      value: () => Promise.resolve('{"accounts":[]}')
    })
    Object.defineProperty(invalidFile, 'text', {
      value: () => Promise.resolve('invalid json')
    })
    Object.defineProperty(input.element, 'files', {
      value: [validFile, invalidFile]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('invalid.json'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('admin.accounts.dataImportParseFailed'))
  })

  it('单个文件导入接口失败时提示对应文件名', async () => {
    importData
      .mockResolvedValueOnce({
        account_created: 1,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0
      })
      .mockRejectedValueOnce(new Error('duplicate account'))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const firstFile = new File(['{"accounts":[{"id":1}]}'], 'ok.json', { type: 'application/json' })
    const failedFile = new File(['{"accounts":[{"id":2}]}'], 'failed.json', { type: 'application/json' })
    Object.defineProperty(firstFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":1}]}')
    })
    Object.defineProperty(failedFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":2}]}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, failedFile]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(2)
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('failed.json'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('duplicate account'))
  })

  it('后端返回的失败详情会标记来源文件', async () => {
    importData.mockResolvedValue({
      account_created: 0,
      account_failed: 1,
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      errors: [
        {
          kind: 'account',
          name: 'acct',
          message: 'already exists'
        }
      ]
    })
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = new File(['{"accounts":[{"id":1}]}'], 'source.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":1}]}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('source.json')
    expect(wrapper.text()).toContain('already exists')
  })
})
