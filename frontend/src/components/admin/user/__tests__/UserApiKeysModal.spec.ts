import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { AdminUser, ApiKey, AdminGroup, UserGroupRateLimitWindow } from '@/types'
import UserApiKeysModal from '../UserApiKeysModal.vue'

const {
  getUserApiKeys,
  getUserGroupRateLimits,
  resetUserGroupRateLimit,
  getAllGroups,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getUserApiKeys: vi.fn(),
  getUserGroupRateLimits: vi.fn(),
  resetUserGroupRateLimit: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      getUserApiKeys,
      getUserGroupRateLimits,
      resetUserGroupRateLimit
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/utils/format', () => ({
  formatDateTime: (value: string | null | undefined) => (value ? `date:${value}` : ''),
  formatCurrency: (value: number | null | undefined) => `$${Number(value ?? 0).toFixed(2)}`
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        if (!params) return key
        return `${key}:${JSON.stringify(params)}`
      }
    })
  }
})

const user: AdminUser = {
  id: 7,
  username: 'limited-user',
  email: 'limited@example.com',
  role: 'user',
  balance: 0,
  concurrency: 1,
  status: 'active',
  allowed_groups: [],
  balance_notify_enabled: false,
  balance_notify_threshold: null,
  balance_notify_extra_emails: [],
  created_at: '2026-06-01T00:00:00Z',
  updated_at: '2026-06-01T00:00:00Z',
  notes: ''
}

const group: AdminGroup = {
  id: 3,
  name: 'standard',
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rate_limit_5h: 25,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '2026-06-01T00:00:00Z',
  updated_at: '2026-06-01T00:00:00Z',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  sort_order: 0
}

function createGroupRateLimitWindow(
  overrides: Partial<UserGroupRateLimitWindow> = {}
): UserGroupRateLimitWindow {
  return {
    user_id: user.id,
    group_id: group.id,
    group_name: group.name,
    rate_limit_5h: 25,
    usage_5h_usd: 6.5,
    window_5h_start: '2026-06-01T00:00:00Z',
    window_5h_reset_at: '2026-06-01T05:00:00Z',
    ...overrides
  }
}

function createApiKey(overrides: Partial<ApiKey> = {}): ApiKey {
  return {
    id: 1,
    user_id: user.id,
    key: 'sk-1234567890abcdefghijklmnop',
    name: 'limited-key',
    group_id: group.id,
    group,
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    last_used_at: null,
    quota: 0,
    quota_used: 0,
    expires_at: null,
    created_at: '2026-06-01T00:00:00Z',
    updated_at: '2026-06-01T00:00:00Z',
    rate_limit_5h: 10,
    rate_limit_1d: 0,
    rate_limit_7d: 50,
    usage_5h: 1.5,
    usage_1d: 0,
    usage_7d: 12,
    window_5h_start: '2026-06-01T00:00:00Z',
    window_1d_start: null,
    window_7d_start: '2026-06-01T00:00:00Z',
    reset_5h_at: '2026-06-01T05:00:00Z',
    reset_1d_at: null,
    reset_7d_at: '2026-06-08T00:00:00Z',
    ...overrides
  }
}

async function mountModal(keys: ApiKey[] = [createApiKey()]) {
  getUserApiKeys.mockResolvedValue({ items: keys })
  getUserGroupRateLimits.mockResolvedValue({
    group_rate_limits: [createGroupRateLimitWindow()]
  })
  getAllGroups.mockResolvedValue([group])

  const wrapper = mount(UserApiKeysModal, {
    props: {
      show: true,
      user
    },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show'],
          template: '<div v-if="show"><slot /></div>'
        },
        GroupBadge: {
          props: ['name'],
          template: '<span data-test="group-badge">{{ name }}</span>'
        },
        GroupOptionItem: true,
        Teleport: true
      }
    }
  })

  await flushPromises()
  return wrapper
}

describe('UserApiKeysModal', () => {
  beforeEach(() => {
    getUserApiKeys.mockReset()
    getUserGroupRateLimits.mockReset()
    resetUserGroupRateLimit.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
  })

  it('shows group 5-hour windows', async () => {
    const wrapper = await mountModal()

    expect(wrapper.get('[data-testid="user-group-rate-limit-3"]').text()).toContain('standard')
    expect(wrapper.get('[data-testid="user-group-rate-limit-3"]').text()).toContain('$6.50')
    expect(wrapper.get('[data-testid="user-group-rate-limit-3"]').text()).toContain('$25.00')
  })

  it('resets the user group 5-hour window and refreshes the row', async () => {
    resetUserGroupRateLimit.mockResolvedValue({
      group_rate_limit: createGroupRateLimitWindow({
        usage_5h_usd: 0,
        window_5h_start: null,
        window_5h_reset_at: null
      })
    })

    const wrapper = await mountModal()

    await wrapper.get('[data-testid="reset-user-group-rate-limit-3"]').trigger('click')
    await flushPromises()

    expect(getUserGroupRateLimits).toHaveBeenCalledWith(user.id)
    expect(resetUserGroupRateLimit).toHaveBeenCalledWith(user.id, group.id)
    expect(wrapper.get('[data-testid="user-group-rate-limit-3"]').text()).toContain('$0.00')
    expect(wrapper.get('[data-testid="user-group-rate-limit-3"]').text()).toContain('admin.users.groupRateLimitNoActiveWindow')
    expect(showSuccess).toHaveBeenCalledWith('admin.users.groupRateLimitResetSuccess')
  })
})
