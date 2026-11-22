import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import ConfirmDialog from '../ConfirmDialog.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)

describe('确认操作等待处理结果', () => {
  it('等待异步确认完成，期间阻止重复确认和关闭', async () => {
    let complete!: () => void
    const operation = new Promise<void>(resolve => { complete = resolve })
    const confirm = vi.fn(() => operation)
    const wrapper = mount(ConfirmDialog, {
      props: { show: true, title: '删除', message: '删除当前项目', onConfirm: confirm },
      global: { stubs: { teleport: true } },
    })
    const button = wrapper.get<HTMLButtonElement>('.btn-primary')
    await button.trigger('click')
    await button.trigger('click')
    expect(confirm).toHaveBeenCalledOnce()
    expect(wrapper.get<HTMLButtonElement>('.btn-primary').element.disabled).toBe(true)
    expect(wrapper.find('.btn-secondary').attributes('disabled')).toBeDefined()
    expect(wrapper.find('.modal-header button').exists()).toBe(false)
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    expect(wrapper.emitted('cancel')).toBeUndefined()
    complete()
    await flushPromises()
    expect(wrapper.get<HTMLButtonElement>('.btn-primary').element.disabled).toBe(false)
    await wrapper.get('.btn-primary').trigger('click')
    expect(confirm).toHaveBeenCalledTimes(2)
  })
})
