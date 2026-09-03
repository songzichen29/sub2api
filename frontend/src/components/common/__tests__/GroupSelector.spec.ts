import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import GroupSelector from '../GroupSelector.vue'
import type { AdminGroup } from '@/types'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const group = (id: number, name: string, platform: AdminGroup['platform']): AdminGroup => ({
  id,
  name,
  platform,
  description: `${name} description`,
  rate_multiplier: 1,
  account_count: 0,
  subscription_type: 'standard'
} as AdminGroup)

const groups = [
  group(1, 'Alpha', 'openai'),
  group(2, 'Beta', 'openai'),
  group(3, 'Claude', 'anthropic'),
  group(4, 'Composite', 'composite')
]

const mountSelector = (modelValue: number[] = []) => mount(GroupSelector, {
  props: {
    modelValue,
    groups,
    platform: 'openai',
    searchable: true,
    selectionActions: true
  },
  global: {
    stubs: {
      GroupBadge: true,
      Icon: true
    }
  }
})

describe('GroupSelector selection actions', () => {
  it('selects all compatible visible groups while preserving selections outside the filter', async () => {
    const wrapper = mountSelector([3])
    const selectVisible = wrapper.get('input[type="checkbox"]')

    await selectVisible.setValue(true)

    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([3, 1, 2, 4])
  })

  it('applies bulk actions only to the current search result', async () => {
    const wrapper = mountSelector([1])
    await wrapper.get('input[type="text"]').setValue('Beta')

    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([1, 2])
  })

  it('supports indeterminate state, invert, and clear', async () => {
    const wrapper = mountSelector([1])
    const selectVisible = wrapper.get<HTMLInputElement>('input[type="checkbox"]')
    expect(selectVisible.element.indeterminate).toBe(true)

    const buttons = wrapper.findAll('button')
    const invert = buttons.find(button => button.text() === 'common.invertSelection')
    const clear = buttons.find(button => button.text() === 'common.clearSelection')

    await invert!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([2, 4])

    await clear!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual([])
  })
})
