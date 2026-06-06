import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
    post
  }
}))

import { exportData, importData } from '@/api/admin/accounts'

describe('admin accounts data api', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    get.mockResolvedValue({ data: { type: 'cpa-auth-files', exported_at: '2026-06-06T00:00:00Z', accounts: [] } })
    post.mockResolvedValue({ data: { account_created: 0, account_failed: 0, proxy_created: 0, proxy_reused: 0, proxy_failed: 0 } })
  })

  it('sends CPA format when exporting account data', async () => {
    await exportData({ format: 'cpa', ids: [3, 5], includeProxies: false })

    expect(get).toHaveBeenCalledWith('/admin/accounts/data', {
      params: {
        format: 'cpa',
        ids: '3,5',
        include_proxies: 'false'
      }
    })
  })

  it('sends CPA format when importing account data', async () => {
    await importData({
      data: { account_id: 'acct-1', access_token: 'access-token' },
      format: 'cpa',
      skip_default_group_bind: true
    })

    expect(post).toHaveBeenCalledWith('/admin/accounts/data', {
      data: { account_id: 'acct-1', access_token: 'access-token' },
      format: 'cpa',
      skip_default_group_bind: true
    })
  })
})
