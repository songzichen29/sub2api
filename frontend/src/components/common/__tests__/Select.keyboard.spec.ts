import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import Select from '../Select.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: ReturnType<typeof mount>
afterEach(() => { wrapper?.unmount(); document.body.innerHTML = '' })

async function press(key: string) {
  document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }))
  await nextTick()
}

async function open(searchable: boolean | 'auto' = 'auto') {
  wrapper = mount(Select, {
    attachTo: document.body,
    props: { modelValue: 'alpha', searchable, options: [
      { value: 'alpha', label: 'Alpha' },
      { value: 'disabled', label: 'Disabled', disabled: true },
      { value: 'beta', label: 'Beta' },
    ] },
  })
  wrapper.get<HTMLButtonElement>('button').element.focus()
  await press('ArrowDown')
  await nextTick()
}

describe('Select keyboard focus', () => {
  it('清空按钮可独立聚焦，使用本地化名称并在清空后返回选择器', async () => {
    wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: 'alpha', clearable: true, options: [{ value: 'alpha', label: 'Alpha' }] },
    })
    const clear = wrapper.get<HTMLButtonElement>('.select-clear')
    expect(clear.element.tagName).toBe('BUTTON')
    expect(clear.element.tabIndex).toBe(0)
    expect(clear.attributes('aria-label')).toBe('common.clearSelection')
    expect(wrapper.get('.select-trigger').element.contains(clear.element)).toBe(false)
    clear.element.focus()
    await clear.trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
    expect(wrapper.emitted('change')).toEqual([[null, null]])
    expect(document.activeElement).toBe(wrapper.get('.select-trigger').element)
  })

  it.each([
    ['Delete', 'alpha'],
    ['Backspace', 'alpha'],
    ['Delete', 0],
    ['Backspace', false],
  ])('支持 %s 清空选中值 %s', async (key, value) => {
    wrapper = mount(Select, {
      attachTo: document.body,
      props: { modelValue: value, clearable: true, options: [{ value, label: 'Value' }] },
    })
    wrapper.get<HTMLButtonElement>('.select-trigger').element.focus()
    await press(String(key))
    expect(wrapper.emitted('update:modelValue')).toEqual([[null]])
    expect(wrapper.emitted('change')).toEqual([[null, null]])
  })

  it.each([
    { modelValue: 'alpha', clearable: false, disabled: false, isComposing: false },
    { modelValue: 'alpha', clearable: true, disabled: true, isComposing: false },
    { modelValue: null, clearable: true, disabled: false, isComposing: false },
    { modelValue: 'alpha', clearable: true, disabled: false, isComposing: true },
  ])('保留不可清空、禁用、空值和输入法组合状态 $isComposing', async ({ isComposing, ...props }) => {
    wrapper = mount(Select, { props: { ...props, options: [{ value: 'alpha', label: 'Alpha' }] } })
    await wrapper.get('.select-trigger').trigger('keydown', { key: 'Delete', isComposing })
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('搜索框的删除键仅编辑搜索内容', async () => {
    await open(true)
    await wrapper.setProps({ clearable: true })
    const input = document.querySelector<HTMLInputElement>('.select-search-input')!
    input.value = 'Alph'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await press('Backspace')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(document.activeElement).toBe(input)
  })

  it.each([false, 'auto'] as const)('selects an enabled option without a search input (%s)', async (searchable) => {
    await open(searchable)
    expect(document.activeElement).toBe(document.querySelector('[role="listbox"]'))
    await press('ArrowDown')
    await press('Enter')
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('button').element)
  })

  it('allows Escape to close a non-searchable dropdown and restore trigger focus', async () => {
    await open(false)
    await press('Escape')
    expect(wrapper.get('button').attributes('aria-expanded')).toBe('false')
    expect(document.activeElement).toBe(wrapper.get('button').element)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('keeps focus in the search input when search is enabled', async () => {
    await open(true)
    expect(document.activeElement).toBe(document.querySelector('.select-search-input'))
    await press('ArrowDown')
    await press('Enter')
    expect(wrapper.emitted('update:modelValue')).toEqual([['beta']])
  })
})
