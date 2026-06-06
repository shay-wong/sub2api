import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import AccountBatchTestModal from '../AccountBatchTestModal.vue'

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

function mountModal() {
  return mount(AccountBatchTestModal, {
    props: {
      show: true,
      accounts
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div data-test="account-batch-test-modal"><slot /><slot name="footer" /></div>' },
        Icon: true
      }
    }
  })
}

describe('AccountBatchTestModal', () => {
  beforeEach(() => {
    localStorage.clear()
    localStorage.setItem('auth_token', 'test-token')
    global.fetch = vi.fn()
      .mockResolvedValueOnce(createStreamResponse(true))
      .mockResolvedValueOnce(createStreamResponse(false, 'token expired')) as any
  })

  it('runs selected accounts sequentially with a model id and shows progress details', async () => {
    const wrapper = mountModal()

    await wrapper.get('[data-test="batch-test-model-id"]').setValue('claude-sonnet-4-5')
    await wrapper.get('[data-test="batch-test-mode"]').setValue('compact')
    await wrapper.get('[data-test="batch-test-start"]').trigger('click')
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
      { model_id: 'claude-sonnet-4-5', mode: 'compact' }
    ])
    expect(wrapper.text()).toContain('account-one')
    expect(wrapper.text()).toContain('account-two')
    expect(wrapper.text()).toContain('ok')
    expect(wrapper.text()).toContain('token expired')
    expect(wrapper.get('[data-test="batch-test-summary"]').text()).toContain('1')
    expect(wrapper.emitted('running-change')).toEqual([[true], [false]])
  })

  it('keeps the start action disabled while cancellation is still unwinding', async () => {
    const stream = createNeverEndingResponse()
    global.fetch = vi.fn().mockResolvedValue(stream.response) as any
    const wrapper = mountModal()

    await wrapper.get('[data-test="batch-test-start"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="batch-test-close"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeDefined()
    expect(global.fetch).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('completed')).toBeUndefined()
    expect(wrapper.emitted('running-change')).toEqual([[true]])

    stream.abortRead()
    await flushPromises()

    expect(wrapper.get('[data-test="batch-test-start"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('completed')).toBeUndefined()
    expect(wrapper.emitted('running-change')).toEqual([[true], [false]])
  })
})
