import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QQReportView from '../QQReportView.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/user/monitor/MonitorCardGrid.vue', () => ({ default: { props: ['items'], template: '<div>{{ items.map(i => i.name).join(",") }}</div>' } }))

const token = 'a'.repeat(64)
beforeEach(() => {
  history.replaceState(null, '', '/qq/report#' + token)
  Object.defineProperty(document, 'fonts', { configurable: true, value: { ready: Promise.resolve() } })
  vi.stubGlobal('requestAnimationFrame', (fn: FrameRequestCallback) => { fn(0); return 1 })
})
afterEach(() => { vi.unstubAllGlobals(); history.replaceState(null, '', '/') })

function mockReport(kind: string, data: object) {
  const fetcher = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ kind, title: '真实数据页面', source_path: '/key-usage', created_at: new Date().toISOString(), expires_at: new Date(Date.now() + 600000).toISOString(), data }) })
  vi.stubGlobal('fetch', fetcher)
  return fetcher
}

describe('QQ business report', () => {
  it('loads a capability in the body, uses no account session, and filters one complete price page', async () => {
    const fetcher = mockReport('prices', { model: 'model-a', filter: 'pro', blocks: ['pro 分组\nUSD 0.32', 'standard 分组\nUSD 0.50'] })
    const view = mount(QQReportView)
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/v1/qq-bot/report', expect.objectContaining({ credentials: 'omit', cache: 'no-store', body: JSON.stringify({ token }) }))
    expect(view.text()).toContain('USD 0.32')
    expect(view.text()).not.toContain('USD 0.50')
    expect(view.find('[data-report-ready="true"]').exists()).toBe(true)
    await view.get('input').setValue('')
    expect(view.text()).toContain('USD 0.50')
    expect(view.text()).not.toContain('下一页')
    view.unmount()
  })

  it('shows only the shared model usage table and treats model names as text', async () => {
    mockReport('usage', { models: [{ model: '<img src=x onerror=alert(1)>', requests: 2, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 30, actual_cost: .25 }] })
    const view = mount(QQReportView)
    await flushPromises()
    expect(view.findAll('table')).toHaveLength(1)
    expect(view.find('img').exists()).toBe(false)
    expect(view.text()).toContain('30')
    expect(view.text()).toContain('$0.2500')
    expect(view.find('canvas').exists()).toBe(false)
    view.unmount()
  })

  it('does not announce capture readiness when authorization has expired', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 410 }))
    const view = mount(QQReportView)
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('已失效')
    expect(view.find('[data-report-ready="true"]').exists()).toBe(false)
    view.unmount()
  })
})
