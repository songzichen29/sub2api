import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import IntegrationDocsPageView from '../IntegrationDocsPageView.vue'

const setLocale = vi.hoisted(() => vi.fn())
vi.mock('@/i18n', () => ({ setLocale }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div data-testid="app-layout"><slot /></div>' } }))
vi.mock('../IntegrationDocsView.vue', () => ({ default: { props: ['embedded'], template: '<article data-testid="docs-content" />' } }))

const wrappers: ReturnType<typeof mount>[] = []
async function mountPage(path = '/docs') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/docs', component: { template: '<div />' } }],
  })
  await router.push(path)
  const wrapper = mount(IntegrationDocsPageView, {
    global: { plugins: [router, createI18n({ legacy: false, locale: 'zh', messages: { zh: { docs: { title: '接入文档' } } } })] },
  })
  wrappers.push(wrapper)
  await flushPromises()
  return wrapper
}

afterEach(() => {
  wrappers.splice(0).forEach(wrapper => wrapper.unmount())
  document.documentElement.classList.remove('dark')
  setLocale.mockClear()
})

describe('documentation iframe container', () => {
  it('keeps the site layout and embeds the requested chapter in one iframe', async () => {
    const wrapper = await mountPage('/docs#clients')
    expect(wrapper.find('[data-testid="app-layout"]').exists()).toBe(true)
    const frame = wrapper.get('iframe')
    const url = new URL(frame.attributes('src'))
    expect(url.origin).toBe(window.location.origin)
    expect(url.pathname).toBe('/docs')
    expect(url.searchParams.get('ui_mode')).toBe('embedded')
    expect(url.searchParams.get('lang')).toBe('zh')
    expect(url.searchParams.has('token')).toBe(false)
    expect(url.hash).toBe('#clients')
    expect(frame.attributes('title')).toBe('接入文档')
    expect(wrapper.find('[data-testid="docs-content"]').exists()).toBe(false)
  })

  it('renders only document content in the custom-menu iframe without recursion', async () => {
    const wrapper = await mountPage('/docs?ui_mode=embedded&theme=dark&lang=en')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(wrapper.find('[data-testid="app-layout"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="docs-content"]').exists()).toBe(true)
    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(setLocale).toHaveBeenCalledWith('en')
  })

  it('passes changes in the site theme into the iframe URL', async () => {
    const wrapper = await mountPage()
    document.documentElement.classList.add('dark')
    await flushPromises()
    expect(new URL(wrapper.get('iframe').attributes('src')).searchParams.get('theme')).toBe('dark')
  })
})
