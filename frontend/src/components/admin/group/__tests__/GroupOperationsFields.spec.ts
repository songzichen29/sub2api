import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import GroupOperationsFields from '../GroupOperationsFields.vue'
import { getAll } from '@/api/admin/groups'
import type { AdminGroup } from '@/types'

vi.mock('@/api/admin/groups', () => ({ getAll: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

function group(id: number, platform = 'openai', status = 'active') {
  return { id, name: 'group-' + id, platform, status } as AdminGroup
}

const props = { idPrefix: 'test', platform: 'openai' as const, groupId: 1, concurrency: 0, fallback: 0 }

describe('GroupOperationsFields', () => {
  beforeEach(() => vi.mocked(getAll).mockReset())

  it('offers only active same-platform destinations and keeps disabled defaults', async () => {
    vi.mocked(getAll).mockResolvedValue([group(1), group(2), group(3, 'anthropic'), group(4, 'openai', 'disabled')])
    const wrapper = mount(GroupOperationsFields, { props })
    await flushPromises()
    expect(wrapper.get('input').element.value).toBe('0')
    expect(wrapper.findAll('option').map(option => option.attributes('value'))).toEqual(['0', '2'])
    expect(wrapper.emitted('update:fallback')).toBeUndefined()
    await wrapper.get('input').setValue('3')
    expect(wrapper.emitted('update:concurrency')).toEqual([[3]])
    await wrapper.get('select').setValue('2')
    await wrapper.get('select').setValue('0')
    expect(wrapper.emitted('update:fallback')).toEqual([[2], [0]])
  })

  it('preserves a saved destination on loading failure and supports retry', async () => {
    vi.mocked(getAll).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([group(2)])
    const wrapper = mount(GroupOperationsFields, { props: { ...props, fallback: 2 } })
    await flushPromises()
    expect(wrapper.get('select').element.value).toBe('2')
    expect(wrapper.get('select').attributes('disabled')).toBeDefined()
    expect(wrapper.emitted('update:fallback')).toBeUndefined()
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.get('select').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('select').element.value).toBe('2')
  })

  it('ignores stale platform loads and clears the route after a platform change', async () => {
    let resolveOld!: (groups: AdminGroup[]) => void
    vi.mocked(getAll).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
      .mockResolvedValueOnce([group(3, 'anthropic')])
    const wrapper = mount(GroupOperationsFields, { props })
    await wrapper.setProps({ platform: 'anthropic' })
    await flushPromises()
    resolveOld([group(2)])
    await flushPromises()
    expect(wrapper.findAll('option').map(option => option.attributes('value'))).toEqual(['0', '3'])
    expect(wrapper.emitted('update:fallback')).toEqual([[0]])
  })

  it('keeps concurrency available for composite groups without offering fallback', async () => {
    vi.mocked(getAll).mockResolvedValue([])
    const wrapper = mount(GroupOperationsFields, { props: { ...props, platform: 'composite' } })
    await flushPromises()
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.find('input').exists()).toBe(true)
  })
})
