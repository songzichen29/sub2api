import { mount, flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import { createMemoryHistory, createRouter } from 'vue-router'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import QQChatDiagnostics from '../QQChatDiagnostics.vue'
import QQBotRecordsView from '@/views/admin/QQBotRecordsView.vue'
import QQBotOverviewView from '@/views/admin/QQBotOverviewView.vue'
import type { QQChatRuntimeStatus } from '@/api/qqcommunity'
import { runtimeMetricsFresh } from '@/utils/qqRuntime'

const api = vi.hoisted(() => ({ config: vi.fn(), runtime: vi.fn(), overview: vi.fn() }))
vi.mock('@/api/qqcommunity', () => ({ qqbotAPI: api }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
function status(): QQChatRuntimeStatus {
  const now = Date.now()
  return { enabled: true, connected: true, revision: 'a'.repeat(64), expected_revision: 'a'.repeat(64), last_seen_at: new Date(now).toISOString(),
    sessions: 2, messages: 50, summaries: 1, metrics_available: true, day: new Date(now + 8 * 3600000).toISOString().slice(0, 10),
    daily_calls: 37, daily_sent: 30, daily_failed: 5, unconfirmed: 2, queued: 3, active: 1,
    recent: [{ id: 'record', scope: '123:group:456', sender: '789', message_id: '-42', status: 'partial', reason: 'qq_delivery_unconfirmed', created: now - 1000, updated: now }] }
}
const plugins = (locale = 'zh') => [createI18n({ legacy: false, locale, messages: { zh, en } })]
afterEach(() => { vi.useRealTimers(); vi.clearAllMocks() })

describe('actual DSH chat records', () => {
  it.each(['zh', 'en'])('shows actual metrics and partial delivery evidence in %s; stale counts become unknown', async locale => {
    vi.useFakeTimers()
    const w = mount(QQChatDiagnostics, { props: { status: status() }, global: { plugins: plugins(locale) } })
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('37')
    expect(w.text()).toContain('123:group:456')
    expect(w.text()).toContain('789 / -42')
    expect(w.text()).toContain(locale === 'zh' ? '部分送达' : 'Partially delivered')
    await vi.advanceTimersByTimeAsync(50000)
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('—')
    expect(w.get('[role="status"]').exists()).toBe(true)
    w.unmount()
  })

  it('does not show zero for legacy, stale or previous-day metrics', () => {
    const current = status()
    expect(runtimeMetricsFresh(current)).toBe(true)
    expect(runtimeMetricsFresh({ ...current, metrics_available: undefined })).toBe(false)
    expect(runtimeMetricsFresh({ ...current, day: '2020-01-01' })).toBe(false)
    expect(runtimeMetricsFresh({ ...current, connected: false })).toBe(false)
    expect(runtimeMetricsFresh({ ...current, last_seen_at: new Date(Date.now() - 46000).toISOString() })).toBe(false)
    const w = mount(QQChatDiagnostics, { global: { plugins: plugins() } })
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('—')
    w.unmount()
  })

  it('loads the actual runtime records on the records page chat tab', async () => {
    const current = status()
    api.config.mockResolvedValue({ app_id: '123' })
    api.runtime.mockResolvedValue({ chat_runtime: current })
    const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/admin/qqbot/records', component: QQBotRecordsView }, { path: '/admin/qqbot/activities', component: { template: '<div />' } }] })
    await router.push('/admin/qqbot/records?tab=chat')
    const w = mount(QQBotRecordsView, { global: { plugins: [...plugins(), router] } })
    await flushPromises()
    expect(api.runtime).toHaveBeenCalledOnce()
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('37')
    expect(w.text()).toContain('部分送达')
    w.unmount()
  })

  it('shows runtime usage on overview and marks unavailable totals unknown', async () => {
    const current = status()
    api.config.mockResolvedValue({ app_id: '123', enabled: true, runtime_chat: { enabled: true }, allowed_groups: ['456'], total_daily_chat_limit: 100, total_daily_image_limit: 20 })
    api.runtime.mockResolvedValue({ groups: [], chat_runtime: current })
    api.overview.mockResolvedValue({ day: current.day, chat_runtime: current, chat_used: 37, chat_remaining: 63, image_used: 0, image_remaining: 20, failed: 5, pending_rewards: 0, reward_amount: 0, queued: 3, running: 1, checks: [] })
    const w = mount(QQBotOverviewView, { global: { plugins: plugins(), stubs: { RouterLink: true, QQMemberPicker: true } } })
    await flushPromises()
    expect(w.text()).toContain('63 / 100')
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('37')
    api.overview.mockResolvedValue({ ...(await api.overview()), chat_used: 0, chat_remaining: 100, chat_runtime: { ...current, metrics_available: false } })
    await w.get('button').trigger('click'); await flushPromises()
    expect(w.text()).not.toContain('100 / 100')
    expect(w.get('[data-testid="chat-metric-daily_calls"]').text()).toBe('—')
    w.unmount()
  })
})
