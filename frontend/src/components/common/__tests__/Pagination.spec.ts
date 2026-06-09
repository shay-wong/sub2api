import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import Pagination from '../Pagination.vue'

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

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: `
    <select
      data-test="page-size-select"
      :value="modelValue"
      @change="$emit('update:modelValue', Number($event.target.value))"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `
}

describe('Pagination', () => {
  it('keeps the page-size selector wide enough for four-digit options', () => {
    const wrapper = mount(Pagination, {
      props: {
        page: 1,
        pageSize: 1000,
        total: 5000,
        pageSizeOptions: [20, 100, 500, 1000]
      },
      global: {
        stubs: {
          Icon: true,
          Select: SelectStub
        }
      }
    })

    const selectContainer = wrapper.find('.page-size-select')
    expect(selectContainer.classes()).toContain('min-w-[6.5rem]')
    expect(wrapper.get('[data-test="page-size-select"]').text()).toContain('1000')
  })

  it('emits the selected custom page size option', async () => {
    const wrapper = mount(Pagination, {
      props: {
        page: 1,
        pageSize: 20,
        total: 5000,
        pageSizeOptions: [20, 100, 500, 1000]
      },
      global: {
        stubs: {
          Icon: true,
          Select: SelectStub
        }
      }
    })

    await wrapper.get('[data-test="page-size-select"]').setValue('1000')

    expect(wrapper.emitted('update:pageSize')).toEqual([[1000]])
  })

  it('keeps the sentinel total hidden while exact total is loading', async () => {
    const wrapper = mount(Pagination, {
      props: {
        page: 1,
        pageSize: 1000,
        itemCount: 20,
        total: 0,
        totalKnown: false,
        totalLoading: true,
        hasNextPage: true,
        pageSizeOptions: [20, 100, 500, 1000]
      },
      global: {
        stubs: {
          Icon: true,
          Select: SelectStub
        }
      }
    })

    expect(wrapper.text()).toContain('pagination.totalLoading')
    expect(wrapper.text()).not.toContain('1001')

    const buttons = wrapper.findAll('button')
    await buttons[buttons.length - 1].trigger('click')

    expect(wrapper.emitted('update:page')).toEqual([[2]])
  })
})
