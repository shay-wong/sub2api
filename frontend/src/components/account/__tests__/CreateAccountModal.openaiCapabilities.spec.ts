import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { createAccountMock, checkMixedChannelRiskMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAdmin: false,
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({})
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

import CreateAccountModal from '../CreateAccountModal.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: {
      show: true,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        ConfirmDialog: true,
        Select: true,
        PlatformIcon: true,
        Icon: true,
        ProxySelector: true,
        ProxyAdBanner: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: true
      }
    }
  })
}

async function prepareOpenAIAPIKeyForm() {
  const wrapper = mountModal()
  const openAIButton = wrapper.findAll('button').find((button) => button.text().trim() === 'OpenAI')
  expect(openAIButton).toBeDefined()
  await openAIButton!.trigger('click')
  await nextTick()

  const apiKeyButton = wrapper.findAll('button').find((button) => button.text().includes('API Key'))
  expect(apiKeyButton).toBeDefined()
  await apiKeyButton!.trigger('click')
  await nextTick()

  await wrapper.get('input[data-tour="account-form-name"]').setValue('OpenAI Alpha Search')
  await wrapper.get('input[type="password"][required]').setValue('sk-test')
  return wrapper
}

describe('CreateAccountModal OpenAI endpoint capabilities', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
  })

  it('inherits official OpenAI Alpha Search support without writing an override', async () => {
    const wrapper = await prepareOpenAIAPIKeyForm()

    const alphaSearchCheckbox = wrapper.get<HTMLInputElement>(
      '[data-testid="openai-endpoint-capability-alpha_search"]'
    )
    expect(alphaSearchCheckbox.element.checked).toBe(true)

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).not.toHaveProperty(
      'openai_capabilities'
    )
  })

  it('persists an explicit Alpha Search opt-out', async () => {
    const wrapper = await prepareOpenAIAPIKeyForm()

    await wrapper
      .get<HTMLInputElement>('[data-testid="openai-endpoint-capability-alpha_search"]')
      .setValue(false)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.credentials?.openai_capabilities).toEqual([
      'chat_completions',
      'embeddings'
    ])
  })

  it('re-infers Alpha Search when the untouched base URL becomes custom', async () => {
    const wrapper = await prepareOpenAIAPIKeyForm()

    await wrapper.get('input[placeholder="https://api.openai.com"]').setValue(
      'https://compat.example/v1'
    )
    expect(
      wrapper.get<HTMLInputElement>('[data-testid="openai-endpoint-capability-alpha_search"]')
        .element.checked
    ).toBe(false)

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).not.toHaveProperty(
      'openai_capabilities'
    )
  })
})
