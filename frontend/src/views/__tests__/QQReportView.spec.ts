import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import QQReportView from '../QQReportView.vue'
import { i18n, loadLocaleMessages } from '@/i18n'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import ModelQualityDetails from '@/features/model-quality/ModelQualityDetails.vue'

vi.mock('@/components/user/monitor/MonitorCardGrid.vue', () => ({ default: { props: ['items'], template: '<div>{{ items.map(i => i.name).join(",") }}</div>' } }))

const token = 'a'.repeat(64)
beforeEach(async () => {
  await loadLocaleMessages('en')
  await loadLocaleMessages('zh')
  i18n.global.locale.value = 'en'
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
  it('opens the clicked quality batch using the report capability and hides history enumeration', async () => {
    const fetcher = mockReport('monitor', { mode: 'v1', items: [], quality_plans: [] })
    const view = mount(QQReportView, { global: { plugins: [i18n] } })
    await flushPromises()
    fetcher.mockResolvedValue({ ok: true, json: async () => ({ items: [], models: ['model'], model: 'model', benchmarks: ['pelican'], benchmark: 'pelican', has_more: false }) })
    view.findComponent(MonitorCardGrid).vm.$emit('qualitySelect', { planId: 'plan', batchId: 'batch', planName: '检测' })
    await flushPromises()
    expect(fetcher).toHaveBeenLastCalledWith('/api/v1/qq-bot/report/detail', expect.objectContaining({ credentials: 'omit', body: JSON.stringify({ token, plan_id: 'plan', batch_id: 'batch', model: '', benchmark: '', page: 1 }) }))
    expect(view.findComponent(ModelQualityDetails).props('snapshotOnly')).toBe(true)
    expect(document.querySelector('[data-scope="history"]')).toBeNull()
    view.unmount()
  })
  it('loads a capability in the body, uses no account session, and filters the shared model usage page', async () => {
    const fetcher = mockReport('usage', { filter: 'pro', models: [{ model: 'pro-model', actual_cost: .32 }, { model: 'standard-model', actual_cost: .5 }] })
    const view = mount(QQReportView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/v1/qq-bot/report', expect.objectContaining({ credentials: 'omit', cache: 'no-store', body: JSON.stringify({ token }) }))
    expect(view.text()).toContain('$0.3200')
    expect(view.text()).not.toContain('$0.5000')
    expect(view.find('[data-report-ready="true"]').exists()).toBe(true)
    await view.get('input').setValue('')
    expect(view.text()).toContain('$0.5000')
    expect(view.text()).not.toContain('下一页')
    view.unmount()
  })

  it('shows only the shared model usage table and treats model names as text', async () => {
    mockReport('usage', { models: [{ model: '<img src=x onerror=alert(1)>', requests: 2, input_tokens: 10, output_tokens: 20, cache_creation_tokens: 0, cache_read_tokens: 0, total_tokens: 30, actual_cost: .25 }] })
    const view = mount(QQReportView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(view.findAll('table')).toHaveLength(1)
    expect(view.find('img').exists()).toBe(false)
    expect(view.text()).toContain('30')
    expect(view.text()).toContain('$0.2500')
    expect(view.find('canvas').exists()).toBe(false)
    expect(view.text()).toContain('模型用量统计')
    expect(view.text()).not.toContain('MODEL USAGE STATISTICS')
    view.unmount()
    expect(i18n.global.locale.value).toBe('en')
  })

  it('does not announce capture readiness when authorization has expired', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 410 }))
    const view = mount(QQReportView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('已失效')
    expect(view.find('[data-report-ready="true"]').exists()).toBe(false)
    view.unmount()
  })

  it('reloads the report and clears the previous result when opening another link in the same tab', async () => {
    mockReport('balance', { balance: 12.34 })
    const view = mount(QQReportView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(view.text()).toContain('$12.34')

    const fetcher = mockReport('usage', { models: [{ model: 'model-b', requests: 1, total_tokens: 30 }] })
    const nextToken = 'b'.repeat(64)
    history.replaceState(null, '', '/qq/report#' + nextToken)
    window.dispatchEvent(new HashChangeEvent('hashchange'))
    expect(view.find('table').exists()).toBe(false)
    await flushPromises()
    expect(fetcher).toHaveBeenCalledWith('/api/v1/qq-bot/report', expect.objectContaining({ body: JSON.stringify({ token: nextToken }) }))
    expect(view.text()).toContain('model-b')
    expect(view.text()).not.toContain('$12.34')
    expect((view.get('input').element as HTMLInputElement).value).toBe('')
    view.unmount()
  })
})
