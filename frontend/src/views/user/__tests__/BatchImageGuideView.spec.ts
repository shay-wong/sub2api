import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { nextTick } from 'vue'

import BatchImageGuideView from '../BatchImageGuideView.vue'
import type { ApiKey } from '@/types'
import type { BatchImageJob, BatchImageJobsListOptions } from '@/api/batchImage'

const {
  fetchPublicSettings,
  showError,
  keysList,
  listAllBatchImageJobs,
  listBatchImageJobs,
  getBatchImageJob,
  listBatchImageItems,
  listBatchImageModels,
  copyToClipboard,
} = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn(),
  showError: vi.fn(),
  keysList: vi.fn(),
  listAllBatchImageJobs: vi.fn(),
  listBatchImageJobs: vi.fn(),
  getBatchImageJob: vi.fn(),
  listBatchImageItems: vi.fn(),
  listBatchImageModels: vi.fn(),
  copyToClipboard: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    fetchPublicSettings,
    showError,
  }),
}))

vi.mock('@/api', () => ({
  keysAPI: {
    list: keysList,
  },
}))

vi.mock('@/api/batchImage', async () => {
  const actual = await vi.importActual<typeof import('@/api/batchImage')>('@/api/batchImage')
  return {
    ...actual,
    listAllBatchImageJobs,
    listBatchImageJobs,
    getBatchImageJob,
    listBatchImageItems,
    listBatchImageModels,
    cancelBatchImageJob: vi.fn(),
    deleteBatchImageJobRecord: vi.fn(),
    downloadBatchImageZip: vi.fn(),
    getBatchImageItemContent: vi.fn(),
    saveBlob: vi.fn(),
    submitBatchImageJob: vi.fn(),
  }
})

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard,
  }),
}))

vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20,
  setPersistedPageSize: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      locale: { value: 'zh-CN' },
    }),
  }
})

function batchKey(id: number, key: string): ApiKey {
  return {
    id,
    user_id: 1,
    key,
    name: `key-${id}`,
    group_id: id,
    group: {
      id,
      name: `group-${id}`,
      platform: 'gemini',
      allow_batch_image_generation: true,
    },
    status: 'active',
    ip_whitelist: [],
    ip_blacklist: [],
    quota: 0,
    quota_used: 0,
    expires_at: null,
    last_used_at: null,
    created_at: '2026-07-07T00:00:00Z',
    updated_at: '2026-07-07T00:00:00Z',
    current_concurrency: 0,
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_7d: 0,
    usage_5h: 0,
    usage_1d: 0,
    usage_7d: 0,
    window_5h_start: null,
    window_1d_start: null,
    window_7d_start: null,
    reset_5h_at: null,
    reset_1d_at: null,
    reset_7d_at: null,
  } as ApiKey
}

function batchJob(id: string, createdAt: number): BatchImageJob {
  return {
    id,
    object: 'image.batch',
    task_name: id,
    parent_batch_id: null,
    status: 'completed',
    model: 'gemini-3.1-flash-image',
    provider: 'vertex',
    item_count: 1,
    success_count: 1,
    fail_count: 0,
    estimated_cost: 0.1,
    hold_amount: 0.1,
    actual_cost: 0.1,
    created_at: createdAt,
    submitted_at: null,
    settled_at: null,
    downloaded_at: null,
  }
}

describe('BatchImageGuideView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchPublicSettings.mockResolvedValue(undefined)
    keysList.mockResolvedValue({
      items: [batchKey(1, 'sk-one'), batchKey(2, 'sk-two')],
    })
    listBatchImageModels.mockResolvedValue({ object: 'list', data: [] })
    getBatchImageJob.mockResolvedValue(batchJob('detail', 10_000))
    listBatchImageItems.mockResolvedValue({ object: 'list', data: [], has_more: false })
    listAllBatchImageJobs.mockImplementation(async (options: BatchImageJobsListOptions) => {
      const cursor = Number(options.cursor || 0)
      const limit = Number(options.limit || 20)
      return {
        object: 'list',
        has_more: true,
        data: Array.from({ length: limit }, (_, index) => ({
          ...batchJob(`all-${cursor + index}`, 10_000 - cursor - index),
          key_id: index % 2 === 0 ? 1 : 2,
        })),
      }
    })
    listBatchImageJobs.mockImplementation(async (apiKey: string, options: BatchImageJobsListOptions) => {
      const cursor = Number(options.cursor || 0)
      const limit = Number(options.limit || 20)
      return {
        object: 'list',
        has_more: true,
        data: Array.from({ length: limit }, (_, index) =>
          batchJob(`${apiKey}-${cursor + index}`, 10_000 - cursor - index),
        ),
      }
    })
  })

  it('fetches all-key history once for page two in all API key mode', async () => {
    const wrapper = shallowMount(BatchImageGuideView)
    await flushPromises()
    await nextTick()

    listAllBatchImageJobs.mockClear()
    listBatchImageJobs.mockClear()
    ;(wrapper.vm as any).handlePageChange(2)
    await flushPromises()
    await nextTick()

    const calls = listAllBatchImageJobs.mock.calls.map(([options]) => ({
      cursor: (options as BatchImageJobsListOptions).cursor,
      limit: (options as BatchImageJobsListOptions).limit,
    }))
    expect(calls).toEqual([{ cursor: '20', limit: 20 }])
    expect(listBatchImageJobs).not.toHaveBeenCalled()
    expect((wrapper.vm as any).batchJobs.map((job: { id: string }) => job.id)).toEqual(
      Array.from({ length: 20 }, (_, index) => `all-${20 + index}`),
    )
    expect((wrapper.vm as any).batchJobs.map((job: { api_key_name: string }) => job.api_key_name).slice(0, 4)).toEqual([
      'key-1',
      'key-2',
      'key-1',
      'key-2',
    ])
    expect((wrapper.vm as any).pagination.has_more).toBe(true)

    wrapper.unmount()
  })

  it('keeps historical all-key rows visible when the key is not currently usable', async () => {
    listAllBatchImageJobs.mockResolvedValue({
      object: 'list',
      has_more: false,
      data: [{
        ...batchJob('archived-key-job', 10_000),
        key_id: 99,
      }],
    })

    const wrapper = shallowMount(BatchImageGuideView)
    await flushPromises()
    await nextTick()

    expect((wrapper.vm as any).batchJobs).toEqual([
      expect.objectContaining({
        id: 'archived-key-job',
        api_key_id: 99,
        api_key_name: 'API Key #99',
      }),
    ])

    wrapper.unmount()
  })

  it('uses all-key history in all mode even when only one key is currently usable', async () => {
    keysList.mockResolvedValue({
      items: [batchKey(1, 'sk-one')],
    })
    listAllBatchImageJobs.mockResolvedValue({
      object: 'list',
      has_more: false,
      data: [{
        ...batchJob('archived-only', 10_000),
        key_id: 99,
      }],
    })

    const wrapper = shallowMount(BatchImageGuideView)
    await flushPromises()
    await nextTick()

    expect(listAllBatchImageJobs).toHaveBeenCalledWith(expect.objectContaining({ cursor: '0', limit: 20 }))
    expect(listBatchImageJobs).not.toHaveBeenCalled()
    expect((wrapper.vm as any).batchJobs).toEqual([
      expect.objectContaining({
        id: 'archived-only',
        api_key_id: 99,
        api_key_name: 'API Key #99',
      }),
    ])

    wrapper.unmount()
  })

  it('does not use the selected usable key to refresh details for a historical unavailable-key row', async () => {
    keysList.mockResolvedValue({
      items: [batchKey(1, 'sk-one')],
    })
    listAllBatchImageJobs.mockResolvedValue({
      object: 'list',
      has_more: false,
      data: [{
        ...batchJob('archived-detail', 10_000),
        key_id: 99,
      }],
    })

    const wrapper = shallowMount(BatchImageGuideView)
    await flushPromises()
    await nextTick()

    getBatchImageJob.mockClear()
    listBatchImageItems.mockClear()
    ;(wrapper.vm as any).selectJob('archived-detail')
    await flushPromises()
    await nextTick()

    expect(getBatchImageJob).not.toHaveBeenCalled()
    expect(listBatchImageItems).not.toHaveBeenCalled()
    expect((wrapper.vm as any).currentJob).toEqual(expect.objectContaining({
      id: 'archived-detail',
      key_id: 99,
    }))

    wrapper.unmount()
  })
})
