import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelQualityStrip from '../ModelQualityStrip.vue'
import ModelQualityDetails from '../ModelQualityDetails.vue'
import ModelQualityForm from '../ModelQualityForm.vue'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import type { QualityPlan, QualityRun } from '@/api/modelQuality'
import type { UserMonitorView } from '@/api/channelMonitor'

const api = vi.hoisted(() => ({ detail: vi.fn(), groupResults: vi.fn(), options: vi.fn(), save: vi.fn() }))
vi.mock('@/api/modelQuality', () => api)
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => true }) }
})
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorQuotaVisible: () => false }))
let wrapper: ReturnType<typeof mount>
beforeEach(() => vi.resetAllMocks())
afterEach(() => wrapper?.unmount())

const target = { group_id: 1, group_name: 'Group', channel_id: 9, channel_name: 'Channel', model: 'model', benchmark: 'candy', reasoning_effort: 'high', service_tier: '' }
const run = (id: string, verdict = 'normal'): QualityRun => ({ id, plan_id: 'p1', target, verdict, status: 'succeeded', phase: 'complete', reason: id, started_at: id === 'older' ? '2026-10-23T09:00:00Z' : '2026-10-23T10:00:00Z', finished_at: '2026-10-23T10:01:00Z', cancel_requested: false })
const plan: QualityPlan = { id: 'p1', name: 'Plan', group_id: 1, monitor_ids: [2], enabled: false, public_visible: true, can_edit: true, interval_minutes: 60, created_at: '', next_run_at: '', target, recent: [run('newest'), run('older', 'degraded')], stats: { total: 2, passed: 1, failed: 1, errors: 0, unknown: 0 } }

describe('integrated model quality', () => {
  it('orders real results oldest to newest with green/yellow verdicts', async () => {
    wrapper = mount(ModelQualityStrip, { props: { plans: [plan] } })
    const bars = wrapper.findAll('[data-run-id]')
    expect(bars.map(b => b.attributes('data-run-id'))).toEqual(['older', 'newest'])
    expect(bars[0].classes()).toContain('bg-yellow-400')
    expect(bars[1].classes()).toContain('bg-emerald-500')
    await bars[0].trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([{ groupId: 1, groupName: 'Group', planName: 'Plan', runId: 'older' }])
    await wrapper.setProps({ plans: [{ ...plan, recent: [run('error', 'error')] }] })
    expect(wrapper.find('[data-run-id]').classes()).not.toContain('bg-yellow-400')
    expect(wrapper.find('[data-run-id]').classes()).not.toContain('bg-emerald-500')
  })
  it('places results only on the bound card and does not open availability details on block click', async () => {
    const item = { id: 1, name: 'Channel', group_name: 'Group', provider: 'openai', primary_model: 'model', primary_status: 'operational', primary_latency_ms: 10, primary_ping_latency_ms: 10, availability_7d: 100, timeline: [], extra_models: [] } as UserMonitorView
    wrapper = mount(MonitorCardGrid, { props: { items: [item, { ...item, id: 2 }], window: '7d', countdownSeconds: 60, loading: false, detailCache: {}, qualityPlans: [plan] } })
    const cards = wrapper.findAll('article')
    expect(cards[0].find('[data-testid="quality-strip"]').exists()).toBe(false)
    expect(cards[1].find('[data-testid="quality-strip"]').exists()).toBe(true)
    expect(wrapper.find('button button').exists()).toBe(false)
    await cards[1].find('[data-run-id="older"]').trigger('click')
    expect(wrapper.emitted('qualitySelect')?.[0]).toEqual([{ groupId: 1, groupName: 'Group', planName: 'Plan', runId: 'older' }])
    expect(wrapper.emitted('cardClick')).toBeUndefined()
  })
  it('merges test types into one timeline while keeping the clicked result group', async () => {
    const pelican: QualityPlan = {
      ...plan, id: 'p2', name: 'Drawing plan', group_id: 2, target: { ...target, group_id: 2, group_name: 'Second group', benchmark: 'pelican' },
      recent: [{ ...run('pelican', 'degraded'), plan_id: 'p2', started_at: '2026-10-23T09:30:00Z', target: { ...target, group_id: 2, group_name: 'Second group', benchmark: 'pelican' } }],
    }
    wrapper = mount(ModelQualityStrip, { props: { plans: [plan, pelican] } })
    expect(wrapper.text().split('modelQuality.statusTitle')).toHaveLength(2)
    expect(wrapper.findAll('[aria-label="modelQuality.history"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-run-id]').map(node => node.attributes('data-run-id'))).toEqual(['older', 'pelican', 'newest'])
    expect(wrapper.find('[data-run-id="pelican"]').attributes('title')).toContain(' · pelican · ')
    expect(wrapper.text()).toContain('modelQuality.verdicts.degraded')
    await wrapper.find('[data-run-id="pelican"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([{ groupId: 2, groupName: 'Second group', planName: 'Drawing plan', runId: 'pelican' }])
  })
  it('limits the combined timeline to the newest 60 unique runs', () => {
    const records = Array.from({ length: 70 }, (_, i) => ({ ...run(`r${i}`), started_at: new Date(Date.UTC(2026, 9, 23, 0, i)).toISOString() }))
    wrapper = mount(ModelQualityStrip, { props: { plans: [{ ...plan, recent: records.slice(0, 40) }, { ...plan, id: 'p2', recent: records.slice(30) }] } })
    const ids = wrapper.findAll('[data-run-id]').map(node => node.attributes('data-run-id'))
    expect(ids).toHaveLength(60)
    expect(ids[0]).toBe('r10')
    expect(ids[59]).toBe('r69')
    expect(new Set(ids).size).toBe(60)
  })
  it('keeps selected records and loads each benchmark page independently', async () => {
    const pelican = { ...run('pelican', 'pending'), target: { ...target, benchmark: 'pelican' }, status: 'running', finished_at: null }
    api.detail.mockResolvedValue(run('older', 'degraded'))
    const gallery = (items: QualityRun[], benchmark = 'candy', has_more = false) => ({ items, benchmark, benchmarks: ['candy', 'pelican'], has_more })
    api.groupResults.mockResolvedValue(gallery([run('newest')], 'candy', true))
    wrapper = mount(ModelQualityDetails, { props: { selection: { groupId: 1, groupName: 'Group', planName: 'Plan', runId: 'older' }, admin: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(api.groupResults).toHaveBeenCalledWith(true, 1, 'candy', 1)
    expect(wrapper.find('h3').text()).toBe('Plan · modelQuality.testResult')
    expect(wrapper.find('[data-benchmark-tab="candy"]').exists()).toBe(true)
    expect(wrapper.find('[data-benchmark-tab="pelican"]').exists()).toBe(true)
    expect(wrapper.find('[data-result-card="older"]').attributes('aria-current')).toBe('true')
    api.groupResults.mockResolvedValue(gallery([pelican], 'pelican', true))
    await wrapper.find('[data-benchmark-tab="pelican"]').trigger('click')
    await flushPromises()
    expect(api.groupResults).toHaveBeenLastCalledWith(true, 1, 'pelican', 1)
    expect(wrapper.find('[data-result-card="pelican"] .animate-spin').exists()).toBe(true)
    api.groupResults.mockResolvedValue(gallery([{ ...pelican, status: 'succeeded', finished_at: 'done', verdict: 'normal', result: { answer: 'Finished drawing', duration_ms: 1000 } }], 'pelican', true))
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.refresh')!.trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-benchmark-tab="pelican"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.find('[data-result-card="pelican"] .animate-spin').exists()).toBe(false)
    expect(wrapper.text()).toContain('Finished drawing')
    api.groupResults.mockResolvedValue(gallery([{ ...pelican, id: 'page2' }], 'pelican'))
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.nextPage')!.trigger('click')
    await flushPromises()
    expect(api.groupResults).toHaveBeenLastCalledWith(true, 1, 'pelican', 2)
    expect(wrapper.find('[data-result-card="page2"]').exists()).toBe(true)
    expect(wrapper.findAll('button').find(b => b.text() === 'modelQuality.nextPage')!.attributes('disabled')).toBeDefined()
  })
  it('ignores a late result after switching benchmark tabs', async () => {
    let finishCandy!: (value: unknown) => void
    api.groupResults.mockResolvedValue({ items: [], benchmark: 'candy', benchmarks: ['candy', 'pelican'], has_more: false })
    wrapper = mount(ModelQualityDetails, { props: { selection: { groupId: 1, groupName: 'Group', planName: 'Plan' } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    api.groupResults.mockImplementation((_admin, _group, mode) => mode === 'candy' ? new Promise(resolve => { finishCandy = resolve }) : Promise.resolve({ items: [{ ...run('pelican'), target: { ...target, benchmark: 'pelican' } }], benchmark: 'pelican', benchmarks: ['candy', 'pelican'], has_more: false }))
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.refresh')!.trigger('click')
    await wrapper.find('[data-benchmark-tab="pelican"]').trigger('click')
    await flushPromises()
    finishCandy({ items: [run('old')], benchmark: 'candy', benchmarks: ['candy', 'pelican'], has_more: false })
    await flushPromises()
    expect(wrapper.find('[data-result-card="pelican"]').exists()).toBe(true)
    expect(wrapper.find('[data-result-card="old"]').exists()).toBe(false)
  })
  it('uses only configured modes, including newly supported modes without frontend labels', async () => {
    api.groupResults.mockResolvedValue({ items: [], benchmark: 'creative', benchmarks: ['creative', 'new-mode'], has_more: false })
    wrapper = mount(ModelQualityDetails, { props: { selection: { groupId: 1, groupName: 'Key group', planName: 'Creative plan', benchmark: 'creative' } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.findAll('[data-benchmark-tab]').map(node => node.attributes('data-benchmark-tab'))).toEqual(['creative', 'new-mode'])
    expect(wrapper.find('[data-benchmark-tab="new-mode"]').text()).toBe('new-mode')
    expect(wrapper.find('h3').text()).toBe('Creative plan · modelQuality.testResult')
    api.groupResults.mockResolvedValue({ items: [], benchmark: 'new-mode', benchmarks: ['creative', 'new-mode'], has_more: false })
    await wrapper.find('[data-benchmark-tab="new-mode"]').trigger('click')
    await flushPromises()
    expect(api.groupResults).toHaveBeenLastCalledWith(false, 1, 'new-mode', 1)
  })
  it('binds a channel and submits no URL or API key fields', async () => {
    api.options.mockResolvedValue({ groups: [{ id: 1, name: 'Group', channel_name: 'Channel' }], keys: [], monitors: [{ id: 2, name: 'Monitor', group_id: 1, model: 'model' }] })
    api.save.mockResolvedValue({})
    wrapper = mount(ModelQualityForm, { props: { plan: null }, global: { stubs: { teleport: true } } })
    await flushPromises()
    const labels = wrapper.findAll('label')
    const monitor = labels.find(l => l.text().includes('modelQuality.monitor'))!
    await monitor.find('select').setValue('2')
    await labels.find(l => l.text().includes('modelQuality.name'))!.find('input').setValue('Test')
    expect(wrapper.find('input[type="url"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('modelQuality.key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(null, expect.objectContaining({ group_id: 1, config: expect.objectContaining({ monitor_id: 2 }) }))
    const saved = api.save.mock.calls[0][1].config
    expect(saved).not.toHaveProperty('api_key_id')
    expect(saved).not.toHaveProperty('base_url')
  })
})
