import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN',
}))

describe('batch image api', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('uses the user JWT route for all-key history', async () => {
    localStorage.setItem('auth_token', 'jwt-token')
    localStorage.setItem('sub2api_selected_project_id', '169')

    const { apiClient } = await import('@/api/client')
    const { listAllBatchImageJobs } = await import('@/api/batchImage')
    const adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: {
        object: 'list',
        data: [],
        has_more: false,
      },
      headers: {},
      config: {},
      statusText: 'OK',
    })
    apiClient.defaults.adapter = adapter

    const result = await listAllBatchImageJobs({
      limit: 20,
      cursor: '40',
      status: 'completed',
      taskName: 'demo',
      downloaded: 'false',
    })

    expect(result).toEqual({ object: 'list', data: [], has_more: false })
    expect(adapter).toHaveBeenCalledTimes(1)
    const config = adapter.mock.calls[0][0]
    expect(config.url).toBe('/user/batch-image/jobs')
    expect(config.url).not.toContain('/v1/images/batches/all')
    expect(config.params).toEqual(expect.objectContaining({
      limit: '20',
      cursor: '40',
      status: 'completed',
      task_name: 'demo',
      downloaded: 'false',
      timezone: expect.any(String),
    }))
    expect(config.headers.get('Authorization')).toBe('Bearer jwt-token')
    expect(config.headers.get('X-Project-ID')).toBe('169')
  })
})
