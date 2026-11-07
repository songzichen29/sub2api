import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import IntegrationDocsView from '../IntegrationDocsView.vue'
import { resolveDocsEndpoints } from '../integrationDocs'
import zhMisc from '@/i18n/locales/zh/misc'
import enMisc from '@/i18n/locales/en/misc'
import zhCommon from '@/i18n/locales/zh/common'
import enCommon from '@/i18n/locales/en/common'

const { appStore, copyToClipboard } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: { site_name: 'Test gateway', api_base_url: 'https://relay.example/gateway/v1/' },
    apiBaseUrl: '', docUrl: '', siteLogo: '', siteName: '',
    fetchPublicSettings: vi.fn().mockResolvedValue(null),
  },
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))
vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({ isAuthenticated: false }),
}))
vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: false, copyToClipboard }),
}))

const mounted: ReturnType<typeof mount>[] = []
async function mountDocs(locale = 'zh', path = '/docs') {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: '/:pathMatch(.*)*', component: { template: '<div />' } }],
  })
  await router.push(path)
  const wrapper = mount(IntegrationDocsView, {
    attachTo: document.body,
    global: {
      plugins: [
        router,
        createI18n({ legacy: false, locale, messages: { zh: { ...zhCommon, ...zhMisc }, en: { ...enCommon, ...enMisc } } }),
      ],
      stubs: { LocaleSwitcher: true },
    },
  })
  mounted.push(wrapper)
  await flushPromises()
  return { wrapper, router }
}
beforeEach(() => {
  copyToClipboard.mockClear()
  HTMLElement.prototype.scrollIntoView = vi.fn()
})
afterEach(() => {
  mounted.splice(0).forEach(wrapper => wrapper.unmount())
  vi.restoreAllMocks()
})

describe('gateway endpoint configuration', () => {
  it.each([
    ['', 'https://site.example'],
    ['https://relay.example/', 'https://relay.example'],
    [' https://relay.example/v1/// ', 'https://relay.example'],
    ['https://relay.example/gateway/v1', 'https://relay.example/gateway'],
    ['https://relay.example/gateway/v1beta/', 'https://relay.example/gateway'],
    ['/gateway/v1', 'https://site.example/gateway'],
    ['https://relay.example/v1?token=redacted#fragment', 'https://relay.example'],
    ['javascript:alert(1)', 'https://site.example'],
    ['https://user:password@relay.example', 'https://site.example'],
  ])('normalizes %s without duplicating protocol versions', (configured, root) => {
    expect(resolveDocsEndpoints(configured, 'https://site.example')).toEqual({
      root, openai: root + '/v1', gemini: root + '/v1beta',
    })
  })
})

describe('IntegrationDocsView', () => {
  it('opens a directly linked chapter after the page has mounted', async () => {
    const { wrapper } = await mountDocs('zh', '/docs#clients')
    expect(document.activeElement).toBe(wrapper.get('#clients').element)
    expect(wrapper.get('.docs-sidebar [aria-current="location"]').attributes('href')).toBe('#clients')
    expect(HTMLElement.prototype.scrollIntoView).toHaveBeenCalled()
  })

  it('renders public documentation and separates the three protocols', async () => {
    const { wrapper } = await mountDocs()
    expect(wrapper.get('h1').text()).toBe('接入文档')
    expect(wrapper.findAll('[data-doc-section]')).toHaveLength(10)
    expect(wrapper.get('#openai code').text()).toContain('https://relay.example/gateway/v1/chat/completions')
    expect(wrapper.get('#claude code').text()).toContain('/gateway/v1/messages')
    expect(wrapper.get('#gemini code').text()).toContain('/gateway/v1beta/models/YOUR_MODEL:generateContent')
    expect(wrapper.text()).not.toContain('/v1/v1')
    expect(wrapper.text()).not.toContain('只在创建成功时完整显示一次')
  })

  it('searches body and example content and exposes an empty state', async () => {
    const { wrapper } = await mountDocs()
    await wrapper.get('#docs-search').setValue('RETRY-AFTER')
    expect(wrapper.findAll('[data-doc-section]').map(section => section.attributes('id'))).toEqual(['troubleshooting'])
    expect(wrapper.get('#troubleshooting details').attributes()).toHaveProperty('open')
    await wrapper.get('#docs-search').setValue('cannot-find-this-section')
    expect(wrapper.findAll('[data-doc-section]')).toHaveLength(0)
    expect(wrapper.get('[role="status"]').text()).toContain('没有找到匹配的章节')
    await wrapper.get('[role="status"] button').trigger('click')
    expect(wrapper.findAll('[data-doc-section]')).toHaveLength(10)
  })

  it('copies the selected runnable example, including line breaks', async () => {
    const { wrapper } = await mountDocs()
    const pythonButton = wrapper.get('#openai').findAll('button').find(button => button.text() === 'Python')!
    await pythonButton.trigger('click')
    await wrapper.get('#openai [data-testid="copy-code"]').trigger('click')
    const copied = copyToClipboard.mock.calls[0][0]
    expect(copied).toBe(wrapper.get('#openai pre code').text())
    expect(copied).toContain('os.environ["SUB2API_API_KEY"]')
    expect(copied).toContain('base_url="https://relay.example/gateway/v1"')
    expect(copied).toContain('\n')
    const overviewCode = wrapper.get('#overview pre code').text()
    expect(overviewCode).toContain(' \\\n  -H ')
    expect(overviewCode).not.toContain('\n+')
  })

  it('clears a search before navigating to a linked hidden section', async () => {
    const { wrapper, router } = await mountDocs()
    await wrapper.get('#docs-search').setValue('quick start')
    expect(wrapper.find('#models').exists()).toBe(false)
    await wrapper.get('#overview a[href="#models"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('#models').exists()).toBe(true)
    expect(router.currentRoute.value.hash).toBe('#models')
    expect(document.activeElement?.id).toBe('models')
  })

  it('renders translated English content', async () => {
    const { wrapper } = await mountDocs('en')
    expect(wrapper.get('h1').text()).toBe('Integration guide')
    expect(wrapper.get('#keys').text()).toContain('Choose a platform and available group')
    expect(wrapper.text()).not.toMatch(/[\u4e00-\u9fff]/)
  })
})
