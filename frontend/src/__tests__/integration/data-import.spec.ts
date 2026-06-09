import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const { importData, importCodexSession } = vi.hoisted(() => ({
  importData: vi.fn(),
  importCodexSession: vi.fn()
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
      importData,
      importCodexSession
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

const successfulImportResult = (accountCreated = 1) => ({
  account_created: accountCreated,
  account_failed: 0,
  proxy_created: 0,
  proxy_reused: 0,
  proxy_failed: 0
})

const successfulCodexImportResult = (created = 1, updated = 0, skipped = 0) => ({
  total: created + updated + skipped,
  created,
  updated,
  skipped,
  failed: 0
})

const codexAccessToken = 'eyJhbGciOiJub25lIn0.eyJleHAiOjQxMDI0NDQ4MDB9.signature'

const sub2apiContent = (
  accountIds: number[],
  proxies: Array<Record<string, unknown>> = [],
  proxyKey?: string
) => JSON.stringify({
  type: 'sub2api-data',
  version: 1,
  exported_at: '2026-06-08T00:00:00Z',
  proxies,
  accounts: accountIds.map((id) => ({
    name: `account-${id}`,
    platform: 'openai',
    type: 'oauth',
    credentials: { access_token: `token-${id}` },
    ...(proxyKey ? { proxy_key: proxyKey } : {}),
    concurrency: 3,
    priority: 50
  }))
})

const createJsonFile = (name: string, content: string) => {
  const file = new File([content], name, { type: 'application/json' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

const createTextFile = (name: string, content: string) => {
  const file = new File([content], name, { type: 'text/plain' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

const withRelativePath = (file: File, path: string) => {
  Object.defineProperty(file, 'webkitRelativePath', {
    value: path,
    configurable: true
  })
  return file
}

const createFileEntry = (path: string, file: File) => ({
  isFile: true,
  isDirectory: false,
  name: file.name,
  fullPath: `/${path}`,
  file: (success: (entryFile: File) => void) => success(file)
})

const createDirectoryEntry = (path: string, entries: Array<ReturnType<typeof createFileEntry> | ReturnType<typeof createDirectoryEntry>>) => ({
  isFile: false,
  isDirectory: true,
  name: path.split('/').pop() || path,
  fullPath: `/${path}`,
  createReader: () => {
    let read = false
    return {
      readEntries: (success: (batch: typeof entries) => void) => {
        if (read) {
          success([])
          return
        }
        read = true
        success(entries)
      }
    }
  }
})

describe('ImportDataModal', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    showError.mockReset()
    showSuccess.mockReset()
    importData.mockReset()
    importCodexSession.mockReset()
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

  it('JSON 文件里的普通文本不会被误识别为裸 RT', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = new File(['plain-token'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('plain-token')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(importCodexSession).not.toHaveBeenCalled()
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

  it('支持拖入数据文件后导入', async () => {
    importData.mockResolvedValue(successfulImportResult())
    const wrapper = mountModal()
    const dropZone = wrapper.get('[data-testid="account-import-drop-zone"]')
    const file = createJsonFile('dropped.json', sub2apiContent([1]))

    await dropZone.trigger('dragenter', {
      dataTransfer: { files: [file] }
    })
    expect(dropZone.classes().join(' ')).toContain('border-primary-400')

    await dropZone.trigger('drop', {
      dataTransfer: { files: [file] }
    })
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('dropped.json')
    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            name: 'account-1'
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
  })

  it('支持清空已选择的导入文件', async () => {
    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')
    const file = createJsonFile('accounts.json', sub2apiContent([1]))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    expect(wrapper.text()).toContain('accounts.json')
    expect(wrapper.get('[data-testid="account-import-selected-files"]').text()).toContain('accounts.json')

    await wrapper.get('[data-testid="account-import-clear-files"]').trigger('click')
    await wrapper.find('form').trigger('submit')

    expect(wrapper.text()).not.toContain('accounts.json')
    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('支持移除单个已选择的导入文件', async () => {
    importData.mockResolvedValue(successfulImportResult())
    const wrapper = mountModal()
    const input = wrapper.find('[data-testid="account-import-file-input"]')
    const firstFile = createJsonFile('first.json', sub2apiContent([1]))
    const secondFile = createJsonFile('second.json', sub2apiContent([2]))
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, secondFile]
    })

    await input.trigger('change')
    const removeButtons = wrapper.findAll('[data-testid="account-import-remove-file"]')
    await removeButtons[0].trigger('click')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).not.toContain('first.json')
    expect(wrapper.text()).toContain('second.json')
    expect(importData).toHaveBeenCalledTimes(1)
    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            name: 'account-2'
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
  })

  it('支持选择文件夹并导入其中的 JSON / TXT 文件', async () => {
    importData.mockResolvedValue(successfulImportResult())
    const wrapper = mountModal()
    const directoryInput = wrapper.find('[data-testid="account-import-directory-input"]')
    const jsonFile = withRelativePath(createJsonFile('accounts.json', sub2apiContent([1])), 'backup/accounts.json')
    const txtFile = withRelativePath(createTextFile('tokens.txt', 'rt----refresh-token'), 'backup/nested/tokens.txt')
    const ignoredFile = withRelativePath(createTextFile('readme.md', 'ignored'), 'backup/readme.md')
    Object.defineProperty(directoryInput.element, 'files', {
      value: [jsonFile, txtFile, ignoredFile]
    })

    await directoryInput.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('backup/accounts.json')
    expect(wrapper.text()).toContain('backup/nested/tokens.txt')
    expect(wrapper.text()).not.toContain('backup/readme.md')
    expect(importData).toHaveBeenCalledTimes(2)
    expect(importData.mock.calls[0][0].data.accounts[0].name).toBe('account-1')
    expect(importData.mock.calls[1][0].data.accounts[0].credentials.refresh_token).toBe('refresh-token')
  })

  it('支持拖入文件夹并递归读取其中的数据文件', async () => {
    importData.mockResolvedValue(successfulImportResult())
    const wrapper = mountModal()
    const dropZone = wrapper.get('[data-testid="account-import-drop-zone"]')
    const jsonFile = createJsonFile('accounts.json', sub2apiContent([1]))
    const txtFile = createTextFile('tokens.txt', 'rt----refresh-token')
    const ignoredFile = createTextFile('readme.md', 'ignored')
    const directoryEntry = createDirectoryEntry('backup', [
      createFileEntry('backup/accounts.json', jsonFile),
      createDirectoryEntry('backup/nested', [
        createFileEntry('backup/nested/tokens.txt', txtFile),
        createFileEntry('backup/nested/readme.md', ignoredFile)
      ])
    ])

    await dropZone.trigger('drop', {
      dataTransfer: {
        items: [
          {
            webkitGetAsEntry: () => directoryEntry
          }
        ],
        files: []
      }
    })
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('backup/accounts.json')
    expect(wrapper.text()).toContain('backup/nested/tokens.txt')
    expect(wrapper.text()).not.toContain('backup/nested/readme.md')
    expect(importData).toHaveBeenCalledTimes(2)
  })

  it('导入中忽略拖入的新文件', async () => {
    let resolveImport: ((value: unknown) => void) | null = null
    importData.mockImplementation(() => new Promise((resolve) => {
      resolveImport = resolve
    }))
    const wrapper = mountModal()
    const dropZone = wrapper.get('[data-testid="account-import-drop-zone"]')
    const input = wrapper.find('input[type="file"]')
    const firstFile = createJsonFile('first.json', sub2apiContent([1]))
    const droppedFile = createJsonFile('dropped.json', sub2apiContent([2]))
    Object.defineProperty(input.element, 'files', {
      value: [firstFile]
    })

    await input.trigger('change')
    const submitPromise = wrapper.find('form').trigger('submit')
    await flushPromises()

    await dropZone.trigger('drop', {
      dataTransfer: { files: [droppedFile] }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('first.json')
    expect(wrapper.text()).not.toContain('dropped.json')
    expect(importData).toHaveBeenCalledTimes(1)

    resolveImport!(successfulImportResult())
    await submitPromise
    await flushPromises()

    expect(showSuccess).toHaveBeenCalled()
  })

  it('支持邮箱密码 RT TXT 格式导入为 OpenAI OAuth 账号', async () => {
    importData.mockResolvedValue(successfulImportResult(3))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile(
      'accounts.txt',
      [
        '\uFEFFfirst@example.com----password-1----at----access-token-1----rt----refresh-token-1',
        'access_token----access-token-2----password-2----refresh_token----refresh-token-2----second@example.com',
        'rt----refresh-token-3'
      ].join('\r\n')
    )
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        type: 'sub2api-data',
        accounts: [
          expect.objectContaining({
            name: 'first@example.com',
            platform: 'openai',
            type: 'oauth',
            credentials: {
              access_token: 'access-token-1',
              refresh_token: 'refresh-token-1',
              email: 'first@example.com',
              expires_at: '1970-01-01T00:00:00Z'
            },
            extra: {
              email: 'first@example.com',
              import_source: 'txt_refresh_token'
            },
            concurrency: 10,
            priority: 1,
            rate_multiplier: 1,
            auto_pause_on_expired: true
          }),
          expect.objectContaining({
            name: 'second@example.com',
            credentials: expect.objectContaining({
              access_token: 'access-token-2',
              refresh_token: 'refresh-token-2'
            })
          }),
          expect.objectContaining({
            name: 'openai-rt-3',
            credentials: {
              refresh_token: 'refresh-token-3',
              expires_at: '1970-01-01T00:00:00Z'
            },
            extra: {
              import_source: 'txt_refresh_token'
            }
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
    expect(JSON.stringify(importData.mock.calls[0][0])).not.toContain('password-1')
    expect(JSON.stringify(importData.mock.calls[0][0])).not.toContain('password-2')
  })

  it('自动模式不会识别无标记裸 RT TXT', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('refresh-tokens.txt', 'refresh-token-1\nrefresh-token-2')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('refresh-tokens.txt'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('admin.accounts.dataImportParseFailed'))
  })

  it('显式选择 RT TXT 时支持裸 RT 多行导入', async () => {
    importData.mockResolvedValue(successfulImportResult(2))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatOpenAIRTTxt')
    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('refresh-tokens.txt', 'refresh-token-1\nrefresh-token-2')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            name: 'openai-rt-1',
            credentials: {
              refresh_token: 'refresh-token-1',
              expires_at: '1970-01-01T00:00:00Z'
            }
          }),
          expect.objectContaining({
            name: 'openai-rt-2',
            credentials: {
              refresh_token: 'refresh-token-2',
              expires_at: '1970-01-01T00:00:00Z'
            }
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
  })

  it('自动模式支持 Mobile RT 明确标记导入', async () => {
    importData.mockResolvedValue(successfulImportResult())
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('mobile-rt.txt', 'mobile_rt----mobile-refresh-token')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            credentials: expect.objectContaining({
              refresh_token: 'mobile-refresh-token',
              client_id: 'app_LlGpXReQgckcGGUo2JrYvtJK'
            })
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
  })

  it('显式选择 Mobile RT TXT 时裸行按 Mobile RT 导入', async () => {
    importData.mockResolvedValue(successfulImportResult(2))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatOpenAIMobileRTTxt')
    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('mobile-refresh-tokens.txt', 'mobile-token-1\nmobile-token-2')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            credentials: expect.objectContaining({
              refresh_token: 'mobile-token-1',
              client_id: 'app_LlGpXReQgckcGGUo2JrYvtJK'
            })
          }),
          expect.objectContaining({
            credentials: expect.objectContaining({
              refresh_token: 'mobile-token-2',
              client_id: 'app_LlGpXReQgckcGGUo2JrYvtJK'
            })
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
  })

  it('无效邮箱密码 RT TXT 行不会调用导入接口', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('bad-accounts.txt', 'first@example.com----password----bad----refresh-token')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('bad-accounts.txt'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('Invalid TXT refresh token import line 1'))
  })

  it('只有 access_token 没有 RT 的 TXT 行不会调用导入接口', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('access-only.txt', 'at----access-token')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('access-only.txt'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('Invalid TXT refresh token import line 1'))
  })

  it('邮箱不唯一的 TXT 行不会调用导入接口', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile(
      'ambiguous-accounts.txt',
      'first@example.com----second@example.com----password----rt----refresh-token'
    )
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('ambiguous-accounts.txt'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('Invalid TXT refresh token import line 1'))
  })

  it('自动模式不会识别无结构 Codex AT 文本', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('codex-at.txt', `${codexAccessToken}\n${codexAccessToken}`)
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importCodexSession).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('codex-at.txt'))
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('admin.accounts.dataImportParseFailed'))
  })

  it('显式选择 Codex JSON / AT 时支持 AT 文本导入', async () => {
    importCodexSession.mockResolvedValue(successfulCodexImportResult(2))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCodexSession')
    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('codex-at.txt', `${codexAccessToken}\n${codexAccessToken}`)
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(importCodexSession).toHaveBeenCalledWith({
      content: `${codexAccessToken}\n${codexAccessToken}`,
      update_existing: true,
      skip_default_group_bind: true
    })
    expect(showSuccess.mock.calls[0][0]).toContain('"account_created":2')
  })

  it('自动识别 Codex JSON 并走 Codex 导入接口', async () => {
    importCodexSession.mockResolvedValue(successfulCodexImportResult(1, 1, 1))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const content = JSON.stringify([
      { accessToken: codexAccessToken, user: { email: 'first@example.com' } },
      { tokens: { access_token: codexAccessToken }, user: { email: 'second@example.com' } },
      codexAccessToken
    ])
    const file = createJsonFile('codex.json', content)
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(importCodexSession).toHaveBeenCalledWith({
      content,
      update_existing: true,
      skip_default_group_bind: true
    })
    const successMessage = showSuccess.mock.calls[0][0]
    expect(successMessage).toContain('"account_created":1')
    expect(successMessage).toContain('"account_updated":1')
    expect(successMessage).toContain('"account_skipped":1')
  })

  it('显式选择 Codex JSON / AT 时交给 Codex 后端解析', async () => {
    importCodexSession.mockResolvedValue(successfulCodexImportResult())
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCodexSession')
    const input = wrapper.find('input[type="file"]')
    const file = createTextFile('codex-plain.txt', 'plain-access-token')
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).not.toHaveBeenCalled()
    expect(importCodexSession).toHaveBeenCalledWith({
      content: 'plain-access-token',
      update_existing: true,
      skip_default_group_bind: true
    })
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
      data: expect.objectContaining({
        type: 'cpa-auth-files',
        accounts: [
          {
            access_token: 'token',
            account_id: 'acct'
          }
        ]
      }),
      format: 'cpa',
      skip_default_group_bind: true
    })
  })

  it('自动识别带 type 的 CPA wrapper 并按 CPA 格式导入', async () => {
    importData.mockResolvedValue(successfulImportResult(2))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('cpa-wrapper.json', JSON.stringify({
      type: 'cpa-auth-files',
      accounts: [
        { access_token: 'token-1', account_id: 'acct-1' },
        { access_token: 'token-2', account_id: 'acct-2' }
      ]
    }))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        type: 'cpa-auth-files',
        accounts: [
          { access_token: 'token-1', account_id: 'acct-1' },
          { access_token: 'token-2', account_id: 'acct-2' }
        ]
      }),
      format: 'cpa',
      skip_default_group_bind: true
    })
  })

  it('显式选择 CPA 时顶层账号数组按 CPA 格式导入', async () => {
    importData.mockResolvedValue(successfulImportResult(2))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCPA')
    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('cpa-array.json', JSON.stringify([
      { access_token: 'token-1', account_id: 'acct-1' },
      { access_token: 'token-2', account_id: 'acct-2' }
    ]))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        type: 'cpa-auth-files',
        accounts: [
          { access_token: 'token-1', account_id: 'acct-1' },
          { access_token: 'token-2', account_id: 'acct-2' }
        ]
      }),
      format: 'cpa',
      skip_default_group_bind: true
    })
  })

  it('显式选择 CPA 时顶层数组包含无效账号仍交给后端校验', async () => {
    importData.mockRejectedValue(new Error('CPA account 2 access_token is required'))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCPA')
    const input = wrapper.find('input[type="file"]')
    const data = [
      { access_token: 'token-1', account_id: 'acct-1' },
      { account_id: 'acct-2' }
    ]
    const file = createJsonFile('mixed-cpa-array.json', JSON.stringify(data))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data,
      format: 'cpa',
      skip_default_group_bind: true
    })
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"account_failed":2'))
    expect(wrapper.text()).toContain('mixed-cpa-array.json')
    expect(wrapper.text()).toContain('CPA account 2 access_token is required')
  })

  it('手动选择 CPA 时 unsupported wrapper 保持原始数据交给后端校验', async () => {
    importData.mockRejectedValue(new Error('unsupported CPA data type: unexpected-auth-files'))
    const wrapper = mountModal()

    await selectDataFormat(wrapper, 'admin.accounts.dataFormatCPA')
    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('unsupported-cpa.json', JSON.stringify({
      type: 'unexpected-auth-files',
      accounts: [
        { access_token: 'token-1', account_id: 'acct-1' }
      ]
    }))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: {
        type: 'unexpected-auth-files',
        accounts: [
          { access_token: 'token-1', account_id: 'acct-1' }
        ]
      },
      format: 'cpa',
      skip_default_group_bind: true
    })
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"account_failed":1'))
    expect(wrapper.text()).toContain('unsupported-cpa.json')
    expect(wrapper.text()).toContain('unsupported CPA data type')
  })

  it('CPA 空账号列表不应被前端跳过成导入成功', async () => {
    importData.mockRejectedValue(new Error('CPA accounts is required'))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('empty-cpa.json', JSON.stringify({
      type: 'cpa-auth-files',
      accounts: []
    }))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: {
        type: 'cpa-auth-files',
        accounts: []
      },
      format: 'cpa',
      skip_default_group_bind: true
    })
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"account_failed":1'))
    expect(showSuccess).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('empty-cpa.json')
    expect(wrapper.text()).toContain('CPA accounts is required')
  })

  it('可以一次选择多个 JSON 文件并聚合导入结果', async () => {
    importData
      .mockResolvedValueOnce({
        account_created: 0,
        account_failed: 0,
        proxy_created: 1,
        proxy_reused: 0,
        proxy_failed: 0
      })
      .mockResolvedValueOnce({
        account_created: 1,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0
      })
      .mockResolvedValueOnce({
        account_created: 2,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0
      })
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    expect(input.attributes('multiple')).toBeDefined()
    const sharedProxy = {
      proxy_key: 'custom-proxy',
      name: 'Shared Proxy',
      protocol: 'http',
      host: '127.0.0.1',
      port: 8080,
      username: 'u',
      password: 'p',
      status: 'active'
    }
    const firstFile = createJsonFile('first.json', sub2apiContent([1], [sharedProxy], 'custom-proxy'))
    const secondFile = createJsonFile('second.json', sub2apiContent([2, 3], [sharedProxy], 'custom-proxy'))
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, secondFile]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(3)
    expect(importData).toHaveBeenNthCalledWith(1, {
      data: expect.objectContaining({
        accounts: [],
        proxies: [
          expect.objectContaining({
            proxy_key: 'http|127.0.0.1|8080|u|p'
          })
        ]
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
    expect(importData).toHaveBeenNthCalledWith(2, {
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            name: 'account-1',
            proxy_key: 'http|127.0.0.1|8080|u|p'
          })
        ],
        proxies: []
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
    expect(importData).toHaveBeenNthCalledWith(3, {
      data: expect.objectContaining({
        accounts: [
          expect.objectContaining({
            name: 'account-2',
            proxy_key: 'http|127.0.0.1|8080|u|p'
          }),
          expect.objectContaining({
            name: 'account-3',
            proxy_key: 'http|127.0.0.1|8080|u|p'
          })
        ],
        proxies: []
      }),
      format: 'sub2api',
      skip_default_group_bind: true
    })
    const successMessage = showSuccess.mock.calls[0][0]
    expect(successMessage).toContain('"account_created":3')
    expect(successMessage).toContain('"proxy_created":1')
  })

  it('多文件导入过程中显示并发文件级进度', async () => {
    let resolveFirstImport: ((value: unknown) => void) | null = null
    let resolveSecondImport: ((value: unknown) => void) | null = null
    importData
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveFirstImport = resolve
      }))
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolveSecondImport = resolve
      }))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const firstFile = createJsonFile('first.json', sub2apiContent([1]))
    const secondFile = createJsonFile('second.json', sub2apiContent([2]))
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, secondFile]
    })

    await input.trigger('change')
    const submitPromise = wrapper.find('form').trigger('submit')
    await flushPromises()

    const progress = wrapper.get('[data-testid="account-import-progress"]')
    expect(progress.text()).toContain('admin.accounts.dataImportProgressImporting {"completed":0,"total":2}')
    expect(progress.text()).toContain('admin.accounts.dataImportProgressConcurrency {"count":2}')
    expect(progress.text()).toContain('admin.accounts.dataImportProgressCurrentFiles {"files":"first.json, second.json"}')
    expect(progress.text()).toContain('0%')
    expect(progress.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('0')

    resolveFirstImport!({
      account_created: 1,
      account_failed: 0,
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0
    })
    await flushPromises()

    const nextProgress = wrapper.get('[data-testid="account-import-progress"]')
    expect(nextProgress.text()).toContain('admin.accounts.dataImportProgressImporting {"completed":1,"total":2}')
    expect(nextProgress.text()).toContain('admin.accounts.dataImportProgressCurrentFiles {"files":"second.json"}')
    expect(nextProgress.text()).toContain('50%')
    expect(nextProgress.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('50')

    resolveSecondImport!({
      account_created: 1,
      account_failed: 0,
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0
    })
    await submitPromise
    await flushPromises()

    expect(showSuccess).toHaveBeenCalled()
  })

  it('多文件导入最多同时提交 10 个请求', async () => {
    const resolvers: Array<(value: unknown) => void> = []
    importData.mockImplementation(() => new Promise((resolve) => {
      resolvers.push(resolve)
    }))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const importFiles = Array.from({ length: 11 }, (_, index) => {
      const fileName = `file-${index + 1}.json`
      return createJsonFile(fileName, sub2apiContent([index + 1]))
    })
    Object.defineProperty(input.element, 'files', {
      value: importFiles
    })

    await input.trigger('change')
    const submitPromise = wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(10)
    expect(wrapper.get('[data-testid="account-import-progress"]').text())
      .toContain('admin.accounts.dataImportProgressConcurrency {"count":10}')

    resolvers[0](successfulImportResult())
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(11)
    expect(wrapper.get('[data-testid="account-import-progress"]').text())
      .toContain('admin.accounts.dataImportProgressImporting {"completed":1,"total":11}')

    resolvers.slice(1).forEach((resolve) => resolve(successfulImportResult()))
    await submitPromise
    await flushPromises()

    expect(showSuccess).toHaveBeenCalled()
  })

  it('Sub2API 账号分片导入进度按账号数量显示', async () => {
    const resolvers: Array<(value: unknown) => void> = []
    importData.mockImplementation(() => new Promise((resolve) => {
      resolvers.push(resolve)
    }))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('large.json', sub2apiContent(Array.from({ length: 26 }, (_, index) => index + 1)))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    const submitPromise = wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-testid="account-import-progress"]').text())
      .toContain('admin.accounts.dataImportProgressImporting {"completed":0,"total":26}')
    expect(wrapper.get('[data-testid="account-import-progress"]').text())
      .toContain('admin.accounts.dataImportProgressConcurrency {"count":2}')

    resolvers[0](successfulImportResult(25))
    await flushPromises()

    expect(wrapper.get('[data-testid="account-import-progress"]').text())
      .toContain('admin.accounts.dataImportProgressImporting {"completed":25,"total":26}')
    expect(wrapper.get('[data-testid="account-import-progress"]').text()).toContain('96%')

    resolvers[1](successfulImportResult(1))
    await submitPromise
    await flushPromises()

    expect(showSuccess).toHaveBeenCalled()
  })

  it('账号分片请求失败时按分片账号数统计失败', async () => {
    importData
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce(successfulImportResult(1))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = createJsonFile('large.json', sub2apiContent(Array.from({ length: 26 }, (_, index) => index + 1)))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(2)
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"account_failed":25'))
    expect(wrapper.text()).toContain('large.json #1')
    expect(wrapper.text()).toContain('network down')
  })

  it('代理预处理请求失败时按代理数量统计失败', async () => {
    importData.mockRejectedValueOnce(new Error('proxy import failed'))
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const proxies = [
      {
        proxy_key: 'proxy-1',
        name: 'Proxy 1',
        protocol: 'http',
        host: '127.0.0.1',
        port: 8080,
        status: 'active'
      },
      {
        proxy_key: 'proxy-2',
        name: 'Proxy 2',
        protocol: 'socks5',
        host: '127.0.0.2',
        port: 1080,
        status: 'active'
      }
    ]
    const file = createJsonFile('proxy-only.json', sub2apiContent([], proxies))
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(1)
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"proxy_failed":2'))
    expect(wrapper.text()).toContain('admin.accounts.dataImportProxyPreparation')
    expect(wrapper.text()).toContain('proxy import failed')
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

  it('单个文件导入接口失败时记录失败并继续导入后续文件', async () => {
    importData
      .mockResolvedValueOnce({
        account_created: 1,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0
      })
      .mockRejectedValueOnce(new Error('duplicate account'))
      .mockResolvedValueOnce({
        account_created: 2,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0
      })
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const firstFile = new File(['{"accounts":[{"id":1}]}'], 'ok.json', { type: 'application/json' })
    const failedFile = new File(['{"accounts":[{"id":2}]}'], 'failed.json', { type: 'application/json' })
    const lastFile = new File(['{"accounts":[{"id":3},{"id":4}]}'], 'last.json', { type: 'application/json' })
    Object.defineProperty(firstFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":1}]}')
    })
    Object.defineProperty(failedFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":2}]}')
    })
    Object.defineProperty(lastFile, 'text', {
      value: () => Promise.resolve('{"accounts":[{"id":3},{"id":4}]}')
    })
    Object.defineProperty(input.element, 'files', {
      value: [firstFile, failedFile, lastFile]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledTimes(3)
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"account_failed":1'))
    expect(wrapper.text()).toContain('failed.json')
    expect(wrapper.text()).toContain('duplicate account')
    expect(wrapper.text()).toContain('"account_created":3')
    expect(wrapper.emitted('imported')).toHaveLength(1)
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
    expect(wrapper.emitted('imported')).toBeUndefined()
  })
})
