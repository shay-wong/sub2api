import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountBatchTestModal from '../AccountBatchTestModal.vue'

const { getAvailableModels, deleteAccount } = vi.hoisted(() => ({
  getAvailableModels: vi.fn(),
  deleteAccount: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getAvailableModels,
      delete: deleteAccount
    }
  }
}))

const showSuccess = vi.fn()
const showError = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError
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

function createStreamResponse(success: boolean, error?: string) {
  const encoder = new TextEncoder()
  const lines = success
    ? [
        'data: {"type":"test_start","model":"claude-sonnet-4-5"}\n',
        'data: {"type":"content","text":"ok"}\n',
        'data: {"type":"test_complete","success":true}\n'
      ]
    : [
        'data: {"type":"test_start","model":"claude-sonnet-4-5"}\n',
        `data: {"type":"error","error":"${error || 'failed'}"}\n`
      ]
  let index = 0
  return {
    ok: true,
    body: {
      getReader: () => ({
        read: vi.fn().mockImplementation(async () => {
          if (index < lines.length) {
            return { done: false, value: encoder.encode(lines[index++]) }
          }
          return { done: true, value: undefined }
        })
      })
    }
  } as Response
}

function createNeverEndingResponse() {
  let rejectRead: (error: unknown) => void = () => {}
  return {
    response: {
      ok: true,
      body: {
        getReader: () => ({
          read: vi.fn().mockImplementation(() => new Promise((_, reject) => {
            rejectRead = reject
          }))
        })
      }
    } as Response,
    abortRead: () => rejectRead(new DOMException('Aborted', 'AbortError'))
  }
}

const accounts = [
  {
    id: 1,
    name: 'account-one',
    platform: 'anthropic',
    type: 'oauth',
    status: 'active'
  },
  {
    id: 2,
    name: 'account-two',
    platform: 'openai',
    type: 'apikey',
    status: 'active'
  }
] as any

const BATCH_TEST_CONCURRENCY = 20
const MODEL_LOAD_CONCURRENCY = 10
const BATCH_TEST_START_STAGGER_MIN_MS = 5
const BATCH_TEST_START_STAGGER_MAX_MS = 25
const BATCH_TEST_STAGGER_RANGE_MS = BATCH_TEST_START_STAGGER_MAX_MS - BATCH_TEST_START_STAGGER_MIN_MS + 1

const mockBatchTestStartStagger = (value = 0) => (
  vi.spyOn(Math, 'random').mockReturnValue(value)
)

const mockedBatchTestStaggerMs = (value: number) => (
  BATCH_TEST_START_STAGGER_MIN_MS + Math.floor(value * BATCH_TEST_STAGGER_RANGE_MS)
)

const manyAccounts = Array.from({ length: BATCH_TEST_CONCURRENCY + 1 }, (_, index) => ({
  id: index + 1,
  name: `account-${index + 1}`,
  platform: index % 2 === 0 ? 'anthropic' : 'openai',
  type: 'oauth',
  status: 'active'
})) as any

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  setup(props: { options: Array<{ value: unknown; label: string }> }, { emit }: { emit: (event: string, ...args: unknown[]) => void }) {
    const onChange = (event: Event) => {
      const raw = (event.target as HTMLSelectElement).value
      const option = props.options.find((item) => String(item.value ?? '') === raw)
      const value = option ? option.value : raw
      emit('update:modelValue', value)
      emit('change', value, option ?? null)
    }
    return { onChange }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue ?? ''" @change="onChange">
      <option v-for="option in options" :key="String(option.value ?? '')" :value="option.value ?? ''">
        {{ option.label }}
      </option>
    </select>
  `
}

function mountModal(accountList = accounts) {
  return mount(AccountBatchTestModal, {
    props: {
      show: true,
      accounts: accountList
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div data-test="account-batch-test-modal"><slot /><slot name="footer" /></div>' },
        Select: SelectStub,
        Icon: true
      }
    }
  })
}

describe('AccountBatchTestModal', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_token', 'test-token')
    getAvailableModels.mockReset()
    deleteAccount.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    getAvailableModels.mockImplementation(async (id: number) => {
      if (id === 1) {
        return [
          { id: 'claude-sonnet-4-5', type: 'model', display_name: 'Claude Sonnet 4.5', created_at: '2026-01-01T00:00:00Z' }
        ]
      }
      return [
        { id: 'gpt-5.4', type: 'model', display_name: 'GPT 5.4', created_at: '2026-01-01T00:00:00Z' }
      ]
    })
    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse(true))
      .mockResolvedValueOnce(createStreamResponse(false, 'token expired')) as any
  })

  it('runs selected accounts with limited concurrency and per-account model choices', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()

    try {
      const wrapper = mountModal()

      await flushPromises()
      const modelSelects = wrapper.findAll('[data-test="batch-test-model-select"]')
      expect(modelSelects).toHaveLength(2)

      await modelSelects[0].setValue('claude-sonnet-4-5')
      await wrapper.get('[data-test="batch-test-mode-compact"]').trigger('click')
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync(mockedBatchTestStaggerMs(0))
      await flushPromises()
      await flushPromises()
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(2)
      expect((global.fetch as any).mock.calls.map((call: any[]) => call[0])).toEqual([
        '/api/v1/admin/accounts/1/test',
        '/api/v1/admin/accounts/2/test'
      ])
      expect((global.fetch as any).mock.calls.map((call: any[]) => JSON.parse(call[1].body))).toEqual([
        { model_id: 'claude-sonnet-4-5', mode: 'compact' },
        { mode: 'compact' }
      ])
      expect(wrapper.text()).toContain('account-one')
      expect(wrapper.text()).toContain('account-two')
      expect(wrapper.text()).toContain('ok')
      expect(wrapper.text()).toContain('token expired')
      expect(wrapper.get('[data-test="batch-test-summary"]').text()).toContain('1')
      expect(wrapper.emitted('running-change')).toEqual([[true], [false]])
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('limits available model loading for selected accounts', async () => {
    const modelLoadResolvers: Array<(value: any[]) => void> = []
    getAvailableModels.mockImplementation(() => new Promise((resolve) => {
      modelLoadResolvers.push(resolve)
    }))

    mountModal(Array.from({ length: MODEL_LOAD_CONCURRENCY + 1 }, (_, index) => ({
      id: index + 1,
      name: `account-${index + 1}`,
      platform: 'anthropic',
      type: 'oauth',
      status: 'active'
    })) as any)

    await flushPromises()

    expect(getAvailableModels).toHaveBeenCalledTimes(MODEL_LOAD_CONCURRENCY)

    modelLoadResolvers[0]([
      { id: 'claude-sonnet-4-5', type: 'model', display_name: 'Claude Sonnet 4.5', created_at: '2026-01-01T00:00:00Z' }
    ])
    await flushPromises()

    expect(getAvailableModels).toHaveBeenCalledTimes(MODEL_LOAD_CONCURRENCY + 1)

    modelLoadResolvers.slice(1).forEach(resolve => resolve([]))
    await flushPromises()
  })

  it('limits batch test requests to 20 concurrent accounts and randomly staggers starts', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()
    const resolvers: Array<(value: Response) => void> = []
    global.fetch = vi.fn().mockImplementation(() => new Promise((resolve) => {
      resolvers.push(resolve)
    })) as any

    try {
      const wrapper = mountModal(manyAccounts)

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync((BATCH_TEST_CONCURRENCY - 1) * mockedBatchTestStaggerMs(0))
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(BATCH_TEST_CONCURRENCY)

      resolvers[0](createStreamResponse(true))
      await flushPromises()
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(BATCH_TEST_CONCURRENCY)

      await vi.advanceTimersByTimeAsync(mockedBatchTestStaggerMs(0))
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(BATCH_TEST_CONCURRENCY + 1)

      resolvers.slice(1).forEach(resolve => resolve(createStreamResponse(true)))
      await flushPromises()
      await flushPromises()

      expect(wrapper.emitted('completed')?.[0]?.[0]).toMatchObject({
        success: BATCH_TEST_CONCURRENCY + 1,
        failed: 0
      })
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('uses random delay values between batch test starts', async () => {
    vi.useFakeTimers()
    const randomSpy = vi.spyOn(Math, 'random')
      .mockReturnValueOnce(0)
      .mockReturnValueOnce(1 - Number.EPSILON)
      .mockReturnValue(0)
    global.fetch = vi.fn().mockImplementation(() => Promise.resolve(createStreamResponse(true))) as any

    try {
      const wrapper = mountModal([
        { id: 1, name: 'account-one', platform: 'anthropic', type: 'oauth', status: 'active' },
        { id: 2, name: 'account-two', platform: 'openai', type: 'apikey', status: 'active' },
        { id: 3, name: 'account-three', platform: 'anthropic', type: 'oauth', status: 'active' }
      ] as any)

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(1)

      await vi.advanceTimersByTimeAsync(BATCH_TEST_START_STAGGER_MIN_MS)
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(2)

      await vi.advanceTimersByTimeAsync(BATCH_TEST_START_STAGGER_MAX_MS - 1)
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(2)

      await vi.advanceTimersByTimeAsync(1)
      await flushPromises()

      expect(global.fetch).toHaveBeenCalledTimes(3)
      expect(randomSpy).toHaveBeenCalled()
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('filters failed accounts and deletes selected failures', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()
    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse(true))
      .mockResolvedValueOnce(createStreamResponse(false, 'token expired')) as any
    deleteAccount.mockResolvedValue({ message: 'ok' })
    window.confirm = vi.fn(() => true)

    try {
      const wrapper = mountModal()

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()
      await vi.advanceTimersByTimeAsync(mockedBatchTestStaggerMs(0))
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-test="batch-test-filter-failed"]').trigger('click')
      expect(wrapper.text()).not.toContain('account-one')
      expect(wrapper.text()).toContain('account-two')

      await wrapper.get('[data-test="batch-test-failed-checkbox"]').setValue(true)
      await wrapper.get('[data-test="batch-test-delete-selected-failed"]').trigger('click')
      await flushPromises()

      expect(deleteAccount).toHaveBeenCalledWith(2)
      expect(wrapper.text()).not.toContain('account-two')
      expect(wrapper.emitted('deleted')?.[0]?.[0]).toEqual({
        success: 1,
        failed: 0,
        deletedIds: [2]
      })
      expect(showSuccess).toHaveBeenCalledWith('admin.accounts.bulkTest.deleteFailedSuccess {"count":1}')
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('fans out all failed account deletions like the original account bulk delete', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()
    const failedAccounts = Array.from({ length: 6 }, (_, index) => ({
      id: index + 1,
      name: `account-${index + 1}`,
      platform: 'anthropic',
      type: 'oauth',
      status: 'active'
    })) as any
    const deleteResolvers: Array<(value: { message: string }) => void> = []
    global.fetch = vi.fn().mockImplementation(() => Promise.resolve(createStreamResponse(false, 'token expired'))) as any
    deleteAccount.mockImplementation(() => new Promise((resolve) => {
      deleteResolvers.push(resolve)
    }))
    window.confirm = vi.fn(() => true)

    try {
      const wrapper = mountModal(failedAccounts)

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()
      await vi.advanceTimersByTimeAsync((failedAccounts.length - 1) * mockedBatchTestStaggerMs(0))
      await flushPromises()
      await flushPromises()

      await wrapper.get('[data-test="batch-test-delete-all-failed"]').trigger('click')
      await flushPromises()

      expect(deleteAccount).toHaveBeenCalledTimes(failedAccounts.length)
      expect(deleteAccount.mock.calls.map(([id]) => id)).toEqual([1, 2, 3, 4, 5, 6])

      deleteResolvers.forEach(resolve => resolve({ message: 'ok' }))
      await flushPromises()

      expect(wrapper.emitted('deleted')?.[0]?.[0]).toEqual({
        success: failedAccounts.length,
        failed: 0,
        deletedIds: [1, 2, 3, 4, 5, 6]
      })
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('keeps start and close actions disabled while failed-account deletion is pending', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()
    const failedAccounts = [
      { id: 1, name: 'account-1', platform: 'anthropic', type: 'oauth', status: 'active' }
    ] as any
    let resolveDelete: ((value: { message: string }) => void) | undefined
    global.fetch = vi.fn().mockImplementation(() => Promise.resolve(createStreamResponse(false, 'token expired'))) as any
    deleteAccount.mockImplementation(() => new Promise((resolve) => {
      resolveDelete = resolve
    }))
    window.confirm = vi.fn(() => true)

    try {
      const wrapper = mountModal(failedAccounts)

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()
      await wrapper.get('[data-test="batch-test-delete-all-failed"]').trigger('click')
      await flushPromises()

      expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-test="batch-test-close"]').attributes('disabled')).toBeDefined()

      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      expect(global.fetch).toHaveBeenCalledTimes(1)
      expect(wrapper.emitted('close')).toBeUndefined()

      resolveDelete?.({ message: 'ok' })
      await flushPromises()

      expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[data-test="batch-test-close"]').attributes('disabled')).toBeUndefined()
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })

  it('keeps the start action disabled while cancellation is still unwinding', async () => {
    vi.useFakeTimers()
    const randomSpy = mockBatchTestStartStagger()
    const streams = Array.from({ length: BATCH_TEST_CONCURRENCY }, () => createNeverEndingResponse())
    let streamIndex = 0
    global.fetch = vi.fn().mockImplementation(() => Promise.resolve(streams[streamIndex++].response)) as any

    try {
      const wrapper = mountModal(manyAccounts)

      await flushPromises()
      await wrapper.get('[data-test="batch-test-start"]').trigger('click')
      await flushPromises()
      await vi.advanceTimersByTimeAsync((BATCH_TEST_CONCURRENCY - 1) * mockedBatchTestStaggerMs(0))
      await flushPromises()
      await wrapper.get('[data-test="batch-test-close"]').trigger('click')
      await flushPromises()

      expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeDefined()
      expect(global.fetch).toHaveBeenCalledTimes(BATCH_TEST_CONCURRENCY)
      expect(wrapper.emitted('completed')).toBeUndefined()
      expect(wrapper.emitted('running-change')).toEqual([[true]])

      streams.forEach(stream => stream.abortRead())
      await flushPromises()

      expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeUndefined()
      expect(wrapper.emitted('completed')).toBeUndefined()
      expect(wrapper.emitted('running-change')).toEqual([[true], [false]])
    } finally {
      randomSpy.mockRestore()
      vi.useRealTimers()
    }
  })
})
