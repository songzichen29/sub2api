import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import BaseDialog from '../BaseDialog.vue'
import Select from '../Select.vue'
import HelpTooltip from '../HelpTooltip.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

enableAutoUnmount(afterEach)
afterEach(() => vi.restoreAllMocks())
const pressEscape = () => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))

describe('stacked dialog Escape handling', () => {
  it('closes only the most recently opened dialog, then allows the parent to close', async () => {
    const parent = mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    const child = mount(BaseDialog, { props: { show: true, title: 'Child' } })
    pressEscape()
    expect(child.emitted('close')).toHaveLength(1)
    expect(parent.emitted('close')).toBeUndefined()
    await child.setProps({ show: false })
    pressEscape()
    expect(parent.emitted('close')).toHaveLength(1)
  })

  it('does not dismiss a parent behind a child that disallows Escape', () => {
    const parent = mount(BaseDialog, { props: { show: true, title: 'Parent' } })
    const child = mount(BaseDialog, { props: { show: true, title: 'Child', closeOnEscape: false } })
    pressEscape()
    expect(child.emitted('close')).toBeUndefined()
    expect(parent.emitted('close')).toBeUndefined()
    child.unmount()
    pressEscape()
    expect(parent.emitted('close')).toHaveLength(1)
  })

  it('Tab 在最上层弹窗内循环并在卸载时返回原焦点', async () => {
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue({ length: 1 } as DOMRectList)
    const opener = document.createElement('button')
    document.body.append(opener)
    opener.focus()
    const wrapper = mount(BaseDialog, {
      props: { show: true, title: '编辑', showCloseButton: false },
      slots: { default: '<input aria-label="名称" /><button type="button">保存</button>' },
    })
    await wrapper.vm.$nextTick()
    const first = document.body.querySelector<HTMLInputElement>('input')!
    const last = document.body.querySelector<HTMLButtonElement>('.modal-body button')!
    last.focus()
    last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(first)
    first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }))
    expect(document.activeElement).toBe(last)
    wrapper.unmount()
    expect(document.activeElement).toBe(opener)
    opener.remove()
  })

  it('支持弹窗内 Select 的 Teleport 菜单和 Escape', async () => {
    vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue({ length: 1 } as DOMRectList)
    const wrapper = mount({
      components: { BaseDialog, Select },
      template: `<BaseDialog :show="true" title="编辑" :show-close-button="false">
        <Select model-value="one" :options="[{ value: 'one', label: '选项一' }]" :searchable="false" />
        <button type="button" id="after-select">保存</button>
      </BaseDialog>`,
    })
    await wrapper.vm.$nextTick()
    const trigger = document.body.querySelector<HTMLButtonElement>('.select-trigger')!
    trigger.click()
    await flushPromises()
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.getComponent(BaseDialog).emitted('close')).toBeUndefined()
    trigger.click()
    await flushPromises()
    document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }))
    await wrapper.vm.$nextTick()
    expect(document.activeElement?.id).toBe('after-select')
  })

  it('帮助提示先处理 Escape，保留其所在弹窗', async () => {
    const wrapper = mount({
      components: { BaseDialog, HelpTooltip },
      data: () => ({ show: false }),
      template: `<button @click="show = true">打开</button>
        <BaseDialog :show="show" title="编辑" @close="show = false">
          <HelpTooltip content="帮助内容" />
        </BaseDialog>`,
    })
    await wrapper.get('button').trigger('click')
    await flushPromises()
    await wrapper.getComponent(HelpTooltip).get('[role="button"]').trigger('keydown', { key: 'Enter' })
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.getComponent(BaseDialog).props('show')).toBe(true)
    expect(wrapper.getComponent(BaseDialog).emitted('close')).toBeUndefined()
    expect(wrapper.getComponent(HelpTooltip).get('[role="button"]').attributes('aria-expanded')).toBe('false')
  })
})
