import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import Select from '@/components/common/Select.vue'
import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const filters = {
  platform: '',
  type: '',
  status: '',
  privacy_mode: '',
  group: '',
  tags: []
}

describe('AccountTableFilters', () => {
  it('暂停筛选与列表文案一致，并发送后端支持的 unschedulable 状态', async () => {
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters,
        groups: [],
        availableTags: []
      }
    })

    const statusSelect = wrapper.findAllComponents(Select)[2]
    const options = statusSelect.props('options') as Array<{ value: string; label: string }>

    expect(options).toContainEqual({
      value: 'unschedulable',
      label: 'admin.accounts.status.paused'
    })
    expect(options).toContainEqual({
      value: 'inactive',
      label: 'admin.accounts.status.inactive'
    })

    statusSelect.vm.$emit('update:modelValue', 'unschedulable')
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('update:filters')?.[0]?.[0]).toMatchObject({
      status: 'unschedulable'
    })
  })
})
