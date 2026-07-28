import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const passkeyAPI = vi.hoisted(() => ({
  isSupported: vi.fn(() => true),
  getVerificationMethod: vi.fn(),
  sendVerifyCode: vi.fn(),
  list: vi.fn(),
  register: vi.fn(),
  rename: vi.fn(),
  remove: vi.fn()
}))

vi.mock('@/api', () => ({ passkeyAPI }))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() })
}))

import ProfilePasskeyCard from '@/components/user/profile/ProfilePasskeyCard.vue'

describe('ProfilePasskeyCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    passkeyAPI.isSupported.mockReturnValue(true)
    passkeyAPI.getVerificationMethod.mockResolvedValue('email')
    passkeyAPI.sendVerifyCode.mockResolvedValue(undefined)
    passkeyAPI.register.mockResolvedValue({})
    passkeyAPI.remove.mockResolvedValue(undefined)
  })

  it('uses email codes for registration when selected by the backend', async () => {
    passkeyAPI.list.mockResolvedValue([])
    const wrapper = mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })
    await flushPromises()

    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.find('#passkey-add-password').exists()).toBe(false)
    await wrapper.get('#passkey-name').setValue('Laptop')
    await wrapper.get('#passkey-add-email-code').setValue('123456')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(passkeyAPI.register).toHaveBeenCalledWith('Laptop', { email_code: '123456' })
  })

  it('uses email codes for deletion when selected by the backend', async () => {
    passkeyAPI.list.mockResolvedValue([
      { id: 12, name: 'Laptop', created_at: '2026-07-28T00:00:00Z', backup: false }
    ])
    const wrapper = mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })
    await flushPromises()

    const deleteButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'common.delete')
    expect(deleteButton).toBeDefined()
    await deleteButton!.trigger('click')
    expect(wrapper.find('#passkey-delete-password').exists()).toBe(false)
    await wrapper.get('#passkey-delete-email-code').setValue('654321')
    await wrapper.get('form.mt-4').trigger('submit')
    await flushPromises()

    expect(passkeyAPI.remove).toHaveBeenCalledWith(12, { email_code: '654321' })
  })
})
