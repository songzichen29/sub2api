import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelQualityStrip from '../ModelQualityStrip.vue'
import ModelQualityDetails from '../ModelQualityDetails.vue'
import ModelQualityForm from '../ModelQualityForm.vue'
import ModelQualityResultCard from '../ModelQualityResultCard.vue'
import MonitorCardGrid from '@/components/user/monitor/MonitorCardGrid.vue'
import type { QualityPlan, QualityRun } from '@/api/modelQuality'
import type { UserMonitorView } from '@/api/channelMonitor'

const api = vi.hoisted(() => ({ planResults: vi.fn(), options: vi.fn(), save: vi.fn() }))
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
    expect(wrapper.emitted('select')?.[0]).toEqual([{ planId: 'p1', batchId: 'older', groupId: 1, groupName: 'Group', planName: 'Plan' }])
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
    expect(wrapper.emitted('qualitySelect')?.[0]).toEqual([{ planId: 'p1', batchId: 'older', groupId: 1, groupName: 'Group', planName: 'Plan' }])
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
    expect(wrapper.find('[data-run-id="pelican"]').attributes('title')).toContain('Drawing plan')
    expect(wrapper.text()).toContain('modelQuality.verdicts.degraded')
    await wrapper.find('[data-run-id="pelican"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([{ planId: 'p2', batchId: 'pelican', groupId: 2, groupName: 'Second group', planName: 'Drawing plan' }])
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
  it('isolates the clicked round from history and switches model and mode independently', async () => {
    const gallery = (items: QualityRun[], model = 'model', benchmark = 'candy', has_more = false) => ({ items, model, models: ['model', 'other-model'], benchmark, benchmarks: ['candy', 'creative'], has_more })
    api.planResults.mockResolvedValue(gallery([run('older')]))
    wrapper = mount(ModelQualityDetails, { props: { selection: { planId: 'p1', batchId: 'older', groupId: 1, groupName: 'Group', planName: 'Plan' }, admin: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(api.planResults).toHaveBeenCalledWith(true, 'p1', { batch_id: 'older', model: '', benchmark: '', page: 1 })
    expect(wrapper.find('h3').text()).toBe('Plan · modelQuality.testResult')
    expect(wrapper.find('select').exists()).toBe(false)
    expect(wrapper.findAll('[data-model-button]').map(node => node.text())).toEqual(['model', 'other-model'])
    expect(wrapper.find('[data-model-button="model"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.findAll('[data-result-card]').map(n => n.attributes('data-result-card'))).toEqual(['older'])
    api.planResults.mockResolvedValue(gallery([run('newest')], 'model', 'candy', true))
    await wrapper.find('[data-scope="history"]').trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenLastCalledWith(true, 'p1', { exclude_batch_id: 'older', model: 'model', benchmark: 'candy', page: 1 })
    expect(wrapper.find('[data-result-card="older"]').exists()).toBe(false)
    expect(wrapper.find('[data-result-card="newest"]').exists()).toBe(true)
    api.planResults.mockResolvedValue(gallery([run('other')], 'other-model', 'creative', true))
    await wrapper.find('[data-model-button="other-model"]').trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenLastCalledWith(true, 'p1', { exclude_batch_id: 'older', model: 'other-model', benchmark: 'candy', page: 1 })
    expect(wrapper.find('[data-model-button="other-model"]').attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('[data-model-button="model"]').attributes('aria-pressed')).toBe('false')
    await wrapper.find('[data-benchmark-tab="candy"]').trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenLastCalledWith(true, 'p1', { exclude_batch_id: 'older', model: 'other-model', benchmark: 'candy', page: 1 })
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.nextPage')!.trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenLastCalledWith(true, 'p1', expect.objectContaining({ page: 2 }))
  })
  it('ignores late results after selecting a different block or closing details', async () => {
    let finishOld!: (value: unknown) => void
    api.planResults.mockImplementation((_admin, _plan, filter) => filter.batch_id === 'old' ? new Promise(resolve => { finishOld = resolve }) : Promise.resolve({ items: [run('newest')], model: 'model', models: ['model'], benchmark: 'candy', benchmarks: ['candy'], has_more: false }))
    const selection = { planId: 'p1', batchId: 'old', groupId: 1, groupName: 'Group', planName: 'Plan' }
    wrapper = mount(ModelQualityDetails, { props: { selection }, global: { stubs: { teleport: true } } })
    await wrapper.setProps({ selection: { ...selection, batchId: 'new' } }); await flushPromises()
    finishOld({ items: [run('old')], model: 'model', models: ['model'], benchmark: 'candy', benchmarks: ['candy'], has_more: false })
    await flushPromises()
    expect(wrapper.find('[data-result-card="newest"]').exists()).toBe(true)
    expect(wrapper.find('[data-result-card="old"]').exists()).toBe(false)
    await wrapper.setProps({ selection: null }); await flushPromises()
    expect(wrapper.find('[data-result-card]').exists()).toBe(false)
  })
  it('uses result modes including future labels and excludes unfinished cards', async () => {
    api.planResults.mockResolvedValue({ items: [{ ...run('pending'), finished_at: null }], model: 'model', models: ['model'], benchmark: 'creative', benchmarks: ['creative', 'new-mode'], has_more: false })
    wrapper = mount(ModelQualityDetails, { props: { selection: { planId: 'p1', groupId: 1, groupName: 'Key group', planName: 'Creative plan', benchmark: 'creative' } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.findAll('[data-benchmark-tab]').map(n => n.attributes('data-benchmark-tab'))).toEqual(['creative', 'new-mode'])
    expect(wrapper.find('[data-benchmark-tab="new-mode"]').text()).toBe('new-mode')
    expect(wrapper.find('[data-result-card]').exists()).toBe(false)
    expect(wrapper.find('h3').text()).toBe('Creative plan · modelQuality.testResult')
    await wrapper.find('[data-benchmark-tab="new-mode"]').trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenLastCalledWith(false, 'p1', expect.objectContaining({ benchmark: 'new-mode' }))
  })
  it('hides unfinished rounds and shows one block per completed round', () => {
    wrapper = mount(ModelQualityStrip, { props: { plans: [{ ...plan, recent: [{ ...run('pending'), finished_at: null }], batches: [] }] } })
    expect(wrapper.find('[data-testid="quality-strip"]').exists()).toBe(false)
    wrapper.unmount()
    wrapper = mount(ModelQualityStrip, { props: { plans: [{ ...plan, batches: [{ id: 'batch', plan_id: 'p1', verdict: 'degraded', count: 4, started_at: '2026-10-23T10:00:00Z', finished_at: '2026-10-23T10:05:00Z' }] }] } })
    expect(wrapper.findAll('[data-run-id]')).toHaveLength(1)
    expect(wrapper.find('[data-run-id="batch"]').classes()).toContain('bg-yellow-400')
  })
  it('shows weighted 24h success rates separately from latest-round verdicts', () => {
    const batch = { id: 'round-a', plan_id: 'p1', verdict: 'error', count: 2, started_at: '2026-10-23T10:00:00Z', finished_at: '2026-10-23T10:05:00Z', stats: { total: 2, passed: 1, failed: 0, errors: 1, unknown: 0 } }
    wrapper = mount(ModelQualityStrip, { props: { plans: [{ ...plan, stats_24h: batch.stats, batches: [batch] }] } })
    expect(wrapper.find('[data-quality-success-rate]').text()).toContain('50.0%')
    expect(wrapper.text()).toContain('modelQuality.verdicts.partialError')
    wrapper.unmount()
    wrapper = mount(ModelQualityStrip, { props: { plans: [
      { ...plan, stats_24h: batch.stats, batches: [batch, { ...batch, id: 'old-round', started_at: '2026-10-22T10:00:00Z', stats: { total: 50, passed: 50, failed: 0, errors: 0, unknown: 0 } }] },
      { ...plan, id: 'p2', stats_24h: { total: 8, passed: 8, failed: 0, errors: 0, unknown: 0 }, batches: [{ ...batch, id: 'round-b', plan_id: 'p2', verdict: 'normal', count: 8, stats: { total: 8, passed: 8, failed: 0, errors: 0, unknown: 0 } }] },
    ] } })
    expect(wrapper.find('[data-quality-success-rate]').text()).toContain('90.0%')
    expect(wrapper.find('[data-quality-success-rate]').text()).toContain('(9/10)')
  })
  it('shows 24h rates in every card, keeps them stable across pages and scopes, and separates configurations', async () => {
    const gallery = (model = 'model', benchmark = 'candy', passed = 10) => ({ items: [
      { ...run('visible'), stats_24h: { total: 30, passed, failed: 15 - passed, errors: 10, unknown: 5 } },
      { ...run('low'), target: { ...target, reasoning_effort: 'low' }, stats_24h: { total: 10, passed: 7, failed: 1, errors: 1, unknown: 1 } },
    ], model, models: ['model', 'other'], benchmark, benchmarks: ['candy', 'pelican'], has_more: true })
    api.planResults.mockResolvedValue(gallery())
    wrapper = mount(ModelQualityDetails, { props: { selection: { planId: 'p1', batchId: 'batch', groupId: 1, groupName: 'Group' } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    const rate = () => wrapper.find('[data-result-card="visible"] [data-quality-success-rate]')
    expect(wrapper.findAll('[data-result-card]')).toHaveLength(2)
    expect(wrapper.findAll('article header [data-quality-success-rate]')).toHaveLength(2)
    expect(wrapper.find('[role="tablist"] [data-quality-success-rate]').exists()).toBe(false)
    expect(rate().text()).toContain('33.3%')
    expect(wrapper.find('[data-result-card="low"] [data-quality-success-rate]').text()).toContain('70.0%')
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.nextPage')!.trigger('click'); await flushPromises()
    expect(rate().text()).toContain('33.3%')
    await wrapper.find('[data-scope="history"]').trigger('click'); await flushPromises()
    expect(rate().text()).toContain('33.3%')
    api.planResults.mockResolvedValue(gallery('model', 'pelican', 15))
    await wrapper.find('[data-benchmark-tab="pelican"]').trigger('click'); await flushPromises()
    expect(rate().text()).toContain('50.0%')
    api.planResults.mockResolvedValue(gallery('other', 'pelican', 0))
    await wrapper.find('[data-model-button="other"]').trigger('click'); await flushPromises()
    expect(rate().text()).toContain('0.0%')
    api.planResults.mockResolvedValue({ ...gallery(), items: [run('visible')] })
    await wrapper.find('[data-scope="current"]').trigger('click'); await flushPromises()
    expect(rate().text()).toContain('—')
    expect(rate().text()).not.toContain('NaN')
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
    expect(wrapper.find('input[type="number"]').attributes('min')).toBe('3')
    expect(wrapper.text()).not.toContain('modelQuality.key')
    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(api.save).toHaveBeenCalledWith(null, expect.objectContaining({ group_id: 1, configs: [expect.objectContaining({ monitor_id: 2 })] }))
    const saved = api.save.mock.calls[0][1].configs[0]
    expect(saved).not.toHaveProperty('api_key_id')
    expect(saved).not.toHaveProperty('base_url')
  })
  it('saves multiple models with independent mode selections in one plan', async () => {
    api.options.mockResolvedValue({ groups: [{ id: 1, name: 'Group', channel_name: '' }], keys: [], monitors: [{ id: 2, name: 'Monitor', group_id: 1, model: 'gpt-6-astra' }] })
    api.save.mockResolvedValue({})
    wrapper = mount(ModelQualityForm, { props: { plan: null }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.findAll('label').find(l => l.text().includes('modelQuality.monitor'))!.find('select').setValue('2')
    await wrapper.findAll('label').find(l => l.text().includes('modelQuality.name'))!.find('input').setValue('Multi')
    await wrapper.find('[data-model-config] input[value="pelican"]').setValue(true)
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.addModel')!.trigger('click')
    const customRow = wrapper.findAll('[data-model-config]')[1]
    await customRow.find('input').setValue('custom-model')
    await customRow.find('input[value="pelican"]').setValue(true)
    expect(customRow.find('input[value="creative"]').exists()).toBe(false)
    await wrapper.find('form').trigger('submit'); await flushPromises()
    const input = api.save.mock.calls[0][1]
    expect(input.configs.map((c: { model: string; benchmark: string }) => [c.model, c.benchmark])).toEqual([['gpt-6-astra', 'candy'], ['gpt-6-astra', 'pelican'], ['custom-model', 'candy'], ['custom-model', 'pelican']])
    expect(input.interval_minutes).toBe(3)
    expect(input.configs.every((c: { monitor_id: number }) => c.monitor_id === 2)).toBe(true)
  })
  it('restores the saved judge and accepts a custom model even when suggestions fail', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [], default_judge_model: 'server-default', judge_models: ['suggested-judge'], judge_models_warning: '列表暂不可用，可手动填写后测试' })
    api.save.mockResolvedValue({})
    const editable = { ...plan, judge_model: 'saved-judge', config: { monitor_id: 2, model: 'generator', benchmark: 'pelican' as const, reasoning_effort: 'high', service_tier: '' } }
    wrapper = mount(ModelQualityForm, { props: { plan: editable }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('button#quality-judge-model').text()).toContain('saved-judge')
    expect(wrapper.text()).toContain('列表暂不可用')
    await wrapper.find('button#quality-judge-model').trigger('click')
    await wrapper.find('.select-search-input').setValue('custom/model')
    await wrapper.find('[role="option"]').trigger('click')
    expect(wrapper.find('button#quality-judge-model').text()).toContain('custom/model')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.save).toHaveBeenCalledWith('p1', expect.objectContaining({ judge_model: 'custom/model' }))
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('uses the server default for new plans and saves a selected suggestion', async () => {
    api.options.mockResolvedValue({ groups: [{ id: 1, name: 'Group', channel_name: '' }], keys: [], monitors: [{ id: 2, name: 'Monitor', group_id: 1, model: 'generator' }], default_judge_model: 'gpt-6.1-sol', judge_models: ['other-judge'] })
    api.save.mockResolvedValue({})
    wrapper = mount(ModelQualityForm, { props: { plan: null }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.find('[data-model-config] input[value="pelican"]').setValue(true)
    expect(wrapper.find('button#quality-judge-model').text()).toContain('gpt-6.1-sol')
    await wrapper.find('button#quality-judge-model').trigger('click')
    await wrapper.findAll('[role="option"]').find(o => o.text() === 'other-judge')!.trigger('click')
    await wrapper.findAll('label').find(l => l.text().includes('modelQuality.monitor'))!.find('select').setValue('2')
    await wrapper.findAll('label').find(l => l.text().includes('modelQuality.name'))!.find('input').setValue('New plan')
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.save).toHaveBeenCalledWith(null, expect.objectContaining({ judge_model: 'other-judge' }))
  })
  it('blocks duplicate submits and closing during the test and retains edits after failure', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [] })
    let rejectSave!: (error: Error) => void
    api.save.mockImplementation(() => new Promise((_resolve, reject) => { rejectSave = reject }))
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, judge_model: 'saved-judge', config: { monitor_id: 2, model: 'generator', benchmark: 'pelican', reasoning_effort: 'high', service_tier: '' } } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.find('form').trigger('submit')
    expect(wrapper.find('[role="status"]').text()).toBe('modelQuality.testingJudge')
    expect(wrapper.find('form > fieldset').attributes('disabled')).toBeDefined()
    expect(wrapper.find('button#quality-judge-model').attributes('disabled')).toBeDefined()
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.find('form').trigger('submit')
    wrapper.findComponent({ name: 'BaseDialog' }).vm.$emit('close')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(api.save).toHaveBeenCalledTimes(1)
    rejectSave(new Error('评分模型测试失败；配置未保存'))
    await flushPromises()
    expect(wrapper.find('[role="alert"]').text()).toContain('配置未保存')
    expect(wrapper.find('button#quality-judge-model').text()).toContain('saved-judge')
    expect(wrapper.emitted('saved')).toBeUndefined()
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
  it('labels disabling an unchanged judge as save without a probe', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [] })
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, enabled: true, judge_model: 'saved-judge', config: { monitor_id: 2, model: 'generator', benchmark: 'pelican', reasoning_effort: 'high', service_tier: '' } } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.testAndSave')
    await wrapper.findAll('label').find(l => l.text() === 'modelQuality.enabled')!.find('input').setValue(false)
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.save')
    await wrapper.find('button#quality-judge-model').trigger('click')
    await wrapper.find('.select-search-input').setValue('changed-judge')
    await wrapper.find('[role="option"]').trigger('click')
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.testAndSave')
  })
  it('saves candy-only plans without judge validation or a scoring probe notice', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [], judge_models_warning: 'Judge unavailable' })
    let finishSave!: (value: unknown) => void
    api.save.mockImplementation(() => new Promise(resolve => { finishSave = resolve }))
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, judge_model: 'x'.repeat(201), config: { monitor_id: 2, model: 'generator', benchmark: 'candy', reasoning_effort: 'high', service_tier: '' } } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('button#quality-judge-model').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('Judge unavailable')
    expect(wrapper.text()).not.toContain('modelQuality.invalidJudgeModel')
    expect(wrapper.text()).not.toContain('modelQuality.judgeProbeNotice')
    expect(wrapper.text()).toContain('modelQuality.candyAnswerNotice')
    expect(wrapper.text()).toContain('modelQuality.candyCredentialNotice')
    expect(wrapper.text()).not.toContain('modelQuality.boundCredentialNotice')
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.save')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.find('form').trigger('submit')
    expect(wrapper.find('[role="status"]').text()).toBe('modelQuality.saving')
    expect(api.save).toHaveBeenCalledTimes(1)
    finishSave({})
    await flushPromises()
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('requires scoring settings only while pelican mode is selected', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [], judge_models_warning: 'Judge unavailable' })
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, judge_model: 'x'.repeat(201), config: { monitor_id: 2, model: 'generator', benchmark: 'candy', reasoning_effort: 'high', service_tier: '' } } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.find('[data-model-config] input[value="pelican"]').setValue(true)
    expect(wrapper.find('button#quality-judge-model').exists()).toBe(true)
    expect(wrapper.text()).toContain('Judge unavailable')
    expect(wrapper.text()).toContain('modelQuality.invalidJudgeModel')
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.testAndSave')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.find('[data-model-config] input[value="pelican"]').setValue(false)
    expect(wrapper.find('button#quality-judge-model').exists()).toBe(false)
    expect(wrapper.find('button[type="submit"]').text()).toBe('modelQuality.save')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
  })
  it('removes retired creative configurations from mixed plans and preserves valid modes', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [] })
    api.save.mockResolvedValue({})
    const base = { monitor_id: 2, model: 'generator', reasoning_effort: 'high', service_tier: '' }
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, configs: [
      { ...base, benchmark: 'candy' }, { ...base, benchmark: 'creative' },
      { ...base, model: 'retired-only', benchmark: 'creative' },
    ] } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.findAll('[data-model-config]')).toHaveLength(1)
    expect(wrapper.find('input[value="creative"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('modelQuality.retiredModeNotice')
    expect(wrapper.find('button#quality-judge-model').exists()).toBe(false)
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.save).toHaveBeenCalledWith('p1', expect.objectContaining({ configs: [expect.objectContaining({ model: 'generator', benchmark: 'candy' })] }))
  })
  it('requires an explicit supported mode when editing a creative-only plan', async () => {
    api.options.mockResolvedValue({ groups: [], keys: [], monitors: [] })
    api.save.mockResolvedValue({})
    wrapper = mount(ModelQualityForm, { props: { plan: { ...plan, config: { monitor_id: 2, model: 'legacy-model', benchmark: 'creative', reasoning_effort: 'high', service_tier: 'priority' } } }, global: { stubs: { teleport: true } } })
    await flushPromises()
    expect(wrapper.find('[data-model-config] input').element.value).toBe('legacy-model')
    expect(wrapper.find('input[value="creative"]').exists()).toBe(false)
    expect(wrapper.find('[data-model-config] input[type="checkbox"]:checked').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('modelQuality.candyCredentialNotice')
    expect(wrapper.text()).toContain('modelQuality.retiredModeNotice')
    expect(wrapper.text()).toContain('modelQuality.selectModelModes')
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.save).not.toHaveBeenCalled()
    await wrapper.find('input[value="pelican"]').setValue(true)
    expect(wrapper.find('button#quality-judge-model').exists()).toBe(true)
    expect(wrapper.find('button[type="submit"]').attributes('disabled')).toBeUndefined()
    await wrapper.find('form').trigger('submit'); await flushPromises()
    expect(api.save).toHaveBeenCalledWith('p1', expect.objectContaining({ configs: [expect.objectContaining({ model: 'legacy-model', benchmark: 'pelican', reasoning_effort: 'high', service_tier: 'priority' })] }))
  })
  it('shows local candy evidence without model confidence or judging usage', () => {
    const checked: QualityRun = {
      ...run('checked'),
      result: {
        answer: '答案是21。', source: 'native-v1:answer_exact', duration_ms: 1000,
        assessment: { version: 'native-v1:answer-exact-v1', model: '', basis: 'answer_exact', confidence: 1, checks: [{ id: 'answer', score: 2, evidence: '最终答案为21，与标准答案一致。' }] },
      },
    }
    wrapper = mount(ModelQualityResultCard, { props: { run: checked, bodyHeight: '240px' } })
    expect(wrapper.find('details').text()).toContain('modelQuality.localAnswerCheck')
    expect(wrapper.find('details').text()).toContain('最终答案为21，与标准答案一致。')
    expect(wrapper.find('details').text()).not.toContain('modelQuality.judgeModel')
    expect(wrapper.find('details').text()).not.toContain('modelQuality.evidenceConfidence')
    expect(wrapper.find('details').text()).not.toContain('modelQuality.judgeUsage')
    expect(wrapper.find('details').text()).not.toContain('100%')
  })
  it('renders native judge evidence as text and still supports legacy results', async () => {
    const assessed: QualityRun = {
      ...run('assessed'),
      result: {
        answer: '21', source: 'native-v1:gpt-6-luna:answer_reasoning', judge_input_tokens: 120, judge_output_tokens: 35,
        assessment: { version: 'native-v1', model: 'gpt-6-luna', basis: 'answer_reasoning', confidence: 0.92, checks: [{ id: 'answer', score: 2, evidence: '<img src=x onerror="alert(1)"> is untrusted text' }] },
      },
    }
    wrapper = mount(ModelQualityResultCard, { props: { run: assessed, bodyHeight: '240px' } })
    expect(wrapper.find('details').text()).toContain('gpt-6-luna')
    expect(wrapper.find('details').text()).toContain('92%')
    expect(wrapper.find('details').text()).toContain('120 / 35')
    expect(wrapper.find('details').text()).toContain('<img src=x')
    expect(wrapper.find('details img').exists()).toBe(false)
    await wrapper.setProps({ run: { ...run('legacy'), result: { answer: '21', source: 'source_classifier:source-migrated-20260917' } } })
    expect(wrapper.find('details').text()).toContain('source_classifier:source-migrated-20260917')
    expect(wrapper.find('details').text()).not.toContain('gpt-6-luna')
    expect(wrapper.find('details').text()).not.toContain('NaN')
  })
})
