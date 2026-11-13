import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelQualityPreview from '../ModelQualityPreview.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: ReturnType<typeof mount>
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(960)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(640)
})
afterEach(() => {
  wrapper?.unmount()
  vi.restoreAllMocks()
})
async function render() {
  wrapper = mount(ModelQualityPreview, { props: { html: '<canvas></canvas>', title: 'Artwork' }, attachTo: document.body })
  await flushPromises()
  return wrapper.find('iframe').element.contentWindow!
}
function report(source: Window, documentVersion: number, type = 'sub2api-quality-preview-error') {
  window.dispatchEvent(new MessageEvent('message', { source, data: { type, documentVersion, message: '<script>private-detail</script>' } }))
}

describe('artwork preview script errors', () => {
  it('reports only an error from the current iframe document and keeps the sandbox', async () => {
    const source = await render()
    expect(wrapper.find('iframe').attributes('sandbox')).toBe('allow-scripts')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    report(window, 1)
    report(source, 1, 'unrelated-message')
    report(source, 0)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    report(source, 1)
    report(source, 1)
    await flushPromises()
    expect(wrapper.findAll('[role="alert"]')).toHaveLength(1)
    expect(wrapper.find('[role="alert"]').text()).toBe('modelQuality.previewScriptFailed')
    expect(wrapper.text()).not.toContain('private-detail')
  })
  it('clears errors on artwork changes and ignores late messages from the previous document', async () => {
    const oldSource = await render()
    report(oldSource, 1)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.setProps({ html: '<svg><circle r="5" /></svg>' })
    report(oldSource, 1)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    report(wrapper.find('iframe').element.contentWindow!, 2)
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  })
  it('removes its message listener when the preview is destroyed', async () => {
    const add = vi.spyOn(window, 'addEventListener')
    const remove = vi.spyOn(window, 'removeEventListener')
    await render()
    const registered = add.mock.calls.find(([type]) => type === 'message')!
    expect(registered).toBeDefined()
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('message', registered[1])
  })
})
