import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import DataTable from '../DataTable.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

const columns = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'status', label: 'Status' }
]

const rows = [
  { id: 'row-1', name: 'Alpha', status: 'active' },
  { id: 'row-2', name: 'Beta', status: 'idle' }
]

const stubDesktopMatchMedia = (matches = true) => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn()
    }))
  })
}

const findRenderedCard = (wrapper: ReturnType<typeof mount>, text: string) => {
  const card = wrapper
    .findAll('div.rounded-lg')
    .find((node) => node.text().includes(text))
  if (!card) {
    throw new Error(`Expected rendered card containing "${text}"`)
  }
  return card
}

describe('DataTable', () => {
  beforeEach(() => {
    stubDesktopMatchMedia()
    Object.defineProperty(window, 'scrollTo', {
      writable: true,
      value: vi.fn()
    })
    localStorage.clear()
  })

  it('renders paired sort arrows and highlights the active direction', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns: [
          { key: 'name', label: 'Name', sortable: true },
          { key: 'created_at', label: 'Created', sortable: true }
        ],
        data: [
          { id: 1, name: 'Beta', created_at: '2026-01-02T00:00:00Z' },
          { id: 2, name: 'Alpha', created_at: '2026-01-01T00:00:00Z' }
        ],
        defaultSortKey: 'name',
        defaultSortOrder: 'asc'
      }
    })

    await wrapper.vm.$nextTick()

    const nameHeader = wrapper.findAll('th')[0]
    expect(nameHeader.attributes('aria-sort')).toBe('ascending')
    expect(nameHeader.findAll('svg')).toHaveLength(2)
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-primary-600')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-gray-300')

    await nameHeader.trigger('click')
    await wrapper.vm.$nextTick()

    expect(nameHeader.attributes('aria-sort')).toBe('descending')
    expect(nameHeader.findAll('svg')[0].classes()).toContain('text-gray-300')
    expect(nameHeader.findAll('svg')[1].classes()).toContain('text-primary-600')
  })

  it('renders every row with no virtual padding spacer for small datasets (virtualization off)', async () => {
    const data = Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data
      }
    })

    await wrapper.vm.$nextTick()

    expect((wrapper.vm as any).shouldVirtualize).toBe(false)
    expect(wrapper.findAll('tbody tr[data-index]')).toHaveLength(data.length)
    expect(wrapper.findAll('tbody tr[aria-hidden="true"]')).toHaveLength(0)
  })

  it('switches to windowed rendering once row count exceeds virtualizeThreshold', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: i + 1, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    expect((wrapper.vm as any).shouldVirtualize).toBe(true)
    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    expect(instance.options.count).toBe(data.length)
  })

  it('keys the virtualizer size cache by row identity, not index (avoids stale heights on sort/filter)', async () => {
    const data = Array.from({ length: 12 }, (_, i) => ({ id: 100 + i, name: `Row ${i + 1}` }))
    const wrapper = mount(DataTable, {
      props: {
        columns: [{ key: 'name', label: 'Name' }],
        data,
        rowKey: 'id',
        virtualizeThreshold: 3
      }
    })

    await wrapper.vm.$nextTick()

    const exposed = (wrapper.vm as any).virtualizer
    const instance = exposed?.value ?? exposed
    expect(instance.options.getItemKey(0)).toBe(100)
    expect(instance.options.getItemKey(5)).toBe(105)
  })

  it('emits rowClick from a clickable desktop row', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id'
      }
    })

    await wrapper.vm.$nextTick()

    await wrapper.find('tbody tr[data-row-id="row-1"]').trigger('click')

    expect(wrapper.emitted('rowClick')).toEqual([[rows[0]]])
  })

  it('does not emit rowClick from nested interactive controls', async () => {
    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id'
      },
      slots: {
        'cell-name': '<button class="nested-action" type="button">Edit</button>',
        'cell-status': '<span class="custom-stop" data-row-click-stop>Details</span>'
      }
    })

    await wrapper.vm.$nextTick()

    await wrapper.find('.nested-action').trigger('click')
    await wrapper.find('.custom-stop').trigger('click')

    expect(wrapper.emitted('rowClick')).toBeUndefined()
  })

  it('emits rowClick from a clickable mobile card', async () => {
    stubDesktopMatchMedia(false)

    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id'
      }
    })

    await wrapper.vm.$nextTick()

    await findRenderedCard(wrapper, 'Alpha').trigger('click')

    expect(wrapper.emitted('rowClick')).toEqual([[rows[0]]])
  })

  it('does not emit rowClick from nested mobile card controls', async () => {
    stubDesktopMatchMedia(false)

    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id'
      },
      slots: {
        'cell-name': '<button class="nested-action" type="button">Edit</button>'
      }
    })

    await wrapper.vm.$nextTick()

    await findRenderedCard(wrapper, 'Edit').find('.nested-action').trigger('click')

    expect(wrapper.emitted('rowClick')).toBeUndefined()
  })

  it('emits rowClick from a virtualized mobile card', async () => {
    stubDesktopMatchMedia(false)

    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id',
        virtualizeMobile: true,
        estimateMobileRowHeight: 120
      }
    })

    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    await findRenderedCard(wrapper, 'Alpha').trigger('click')

    expect(wrapper.emitted('rowClick')).toEqual([[rows[0]]])
  })

  it('does not emit rowClick from nested virtualized mobile card controls', async () => {
    stubDesktopMatchMedia(false)

    const wrapper = mount(DataTable, {
      props: {
        columns,
        data: rows,
        clickableRows: true,
        rowKey: 'id',
        virtualizeMobile: true,
        estimateMobileRowHeight: 120
      },
      slots: {
        'cell-name': '<span class="custom-stop" data-row-click-stop>Details</span>'
      }
    })

    await wrapper.vm.$nextTick()
    await wrapper.vm.$nextTick()

    await findRenderedCard(wrapper, 'Details').find('.custom-stop').trigger('click')

    expect(wrapper.emitted('rowClick')).toBeUndefined()
  })
})
