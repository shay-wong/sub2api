import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import OpsDashboardHeader from '../OpsDashboardHeader.vue'

vi.mock('@/api', () => ({
  adminAPI: {
    groups: {
      getAll: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getRealtimeTrafficSummary: vi.fn().mockResolvedValue({
      enabled: true,
      summary: {
        qps: { current: 0, peak: 0, avg: 0 },
        tps: { current: 0, peak: 0, avg: 0 }
      }
    })
  }
}))

vi.mock('@/stores', () => ({
  useAdminSettingsStore: () => ({
    opsRealtimeMonitoringEnabled: true,
    setOpsRealtimeMonitoringEnabledLocal: vi.fn()
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />'
}

function mountHeader(canManageOps: boolean) {
  return mount(OpsDashboardHeader, {
    props: {
      overview: null,
      platform: '',
      groupId: null,
      timeRange: '1h',
      queryMode: 'auto',
      loading: false,
      lastUpdated: new Date('2026-06-16T00:00:00Z'),
      canManageOps
    },
    global: {
      stubs: {
        Select: SelectStub,
        HelpTooltip: true,
        BaseDialog: true,
        Icon: true
      }
    }
  })
}

describe('OpsDashboardHeader permissions', () => {
  it('keeps refresh visible but hides management buttons for operator', async () => {
    const wrapper = mountHeader(false)
    await flushPromises()

    const text = wrapper.text()
    expect(wrapper.find('button[title="common.refresh"]').exists()).toBe(true)
    expect(text).not.toContain('admin.ops.alertRules.manage')
    expect(text).not.toContain('common.settings')
  })

  it('shows management buttons for admin', async () => {
    const wrapper = mountHeader(true)
    await flushPromises()

    const text = wrapper.text()
    expect(wrapper.find('button[title="common.refresh"]').exists()).toBe(true)
    expect(text).toContain('admin.ops.alertRules.manage')
    expect(text).toContain('common.settings')
  })
})
