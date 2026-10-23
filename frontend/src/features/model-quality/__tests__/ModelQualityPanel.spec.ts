import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelQualityPanel from '../ModelQualityPanel.vue'
import type { QualityPlan } from '@/api/modelQuality'

const api = vi.hoisted(() => ({ list: vi.fn(), planResults: vi.fn(), remove: vi.fn(), run: vi.fn(), cancel: vi.fn() }))
vi.mock('@/api/modelQuality', () => api)
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, args?: Record<string, unknown>) => args ? `${key}:${JSON.stringify(args)}` : key }) }))
vi.mock('../ModelQualityForm.vue', () => ({ default: { template: '<div />' } }))
let wrapper: ReturnType<typeof mount>
beforeEach(() => { vi.clearAllMocks() })
afterEach(() => { wrapper?.unmount() })

const plan: QualityPlan = {
  id: 'p1', name: '测试计划', group_id: 1, can_edit: false, enabled: false, public_visible: true, interval_minutes: 60,
  next_run_at: '2026-10-23T10:00:00Z', created_at: '2026-10-23T09:00:00Z',
  target: { group_id: 1, group_name: '测试分组', channel_id: 2, channel_name: '测试渠道', benchmark: 'candy', model: 'test-model', reasoning_effort: 'high', service_tier: '' },
  stats: { total: 102, passed: 1, failed: 1, errors: 80, unknown: 20 },
  recent: [{ id: 'r1', plan_id: 'p1', target: {} as QualityPlan['target'], status: 'succeeded', verdict: 'normal', phase: 'complete', reason: '', started_at: '2026-10-23T09:00:00Z', finished_at: '2026-10-23T09:01:00Z', cancel_requested: false }],
}
async function render() {
  api.list.mockResolvedValue([structuredClone(plan)])
  wrapper = mount(ModelQualityPanel, { global: { stubs: { teleport: true } } })
  await flushPromises()
}
describe('model quality results', () => {
  it('excludes errors and unknown results from pass rate and hides admin actions', async () => {
    await render()
    expect(api.list).toHaveBeenCalledWith(false)
    expect(wrapper.text()).toContain('50.0%')
    expect(wrapper.findAll('[title]').map(node => node.attributes('title'))).toContain('modelQuality.samples:{"count":2}')
    expect(wrapper.text()).not.toContain('modelQuality.caveat')
    expect(wrapper.text()).not.toContain('modelQuality.run')
  })
  it('keeps generated HTML out of the parent document and exposes its source', async () => {
    await render()
    const completed = { ...plan.recent[0], target: plan.target, result: { answer: '<img src=x onerror=alert(1)>', html: '<script>window.stolen=true</script>', duration_ms: 1000 } }
    api.planResults.mockResolvedValue({ items: [completed], model: 'test-model', models: ['test-model'], benchmark: 'candy', benchmarks: ['candy'], has_more: false })
    await wrapper.find('[data-run-id="r1"]').trigger('click'); await flushPromises()
    expect(api.planResults).toHaveBeenCalledWith(false, 'p1', expect.objectContaining({ batch_id: 'r1', page: 1 }))
    expect(wrapper.find('h3[id]').text()).toBe('测试计划 · modelQuality.testResult')
    const sourceButton = wrapper.findAll('button').find(button => button.text() === 'modelQuality.sourceView')
    expect(sourceButton).toBeDefined()
    expect(sourceButton!.element.closest('template')).toBeNull()
    await sourceButton!.trigger('click')
    expect(wrapper.text()).toContain('<script>window.stolen=true</script>')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('iframe').exists()).toBe(false)
  })
  it('opens history with the plan name and its configured mode', async () => {
    api.list.mockResolvedValue([{ ...plan, target: { ...plan.target, benchmark: 'creative' }, recent: [] }])
    api.planResults.mockResolvedValue({ items: [], model: 'test-model', models: ['test-model'], benchmark: 'creative', benchmarks: ['creative'], has_more: false })
    wrapper = mount(ModelQualityPanel, { global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'modelQuality.history')!.trigger('click')
    await flushPromises()
    expect(api.planResults).toHaveBeenCalledWith(false, 'p1', expect.objectContaining({ benchmark: 'creative', page: 1 }))
    expect(wrapper.find('h3[id]').text()).toBe('测试计划 · modelQuality.testResult')
    expect(wrapper.findAll('[data-benchmark-tab]').map(node => node.attributes('data-benchmark-tab'))).toEqual(['creative'])
  })
  it('confirms deletion, removes the plan and hides active-round edit actions', async () => {
    api.list.mockResolvedValue([{ ...plan, can_edit: true }])
    api.remove.mockResolvedValue({ deleted: true })
    wrapper = mount(ModelQualityPanel, { props: { admin: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'modelQuality.delete')!.trigger('click')
    expect(api.remove).not.toHaveBeenCalled()
    api.list.mockResolvedValue([])
    await wrapper.findAll('button').find(b => b.text() === 'common.confirm')!.trigger('click'); await flushPromises()
    expect(api.remove).toHaveBeenCalledWith('p1')
    expect(wrapper.find('[data-run-id]').exists()).toBe(false)
    wrapper.unmount()
    api.list.mockResolvedValue([{ ...plan, can_edit: true, active_run_id: 'active-child' }])
    wrapper = mount(ModelQualityPanel, { props: { admin: true }, global: { stubs: { teleport: true } } })
    await flushPromises()
    for (const action of ['delete', 'edit', 'run']) expect(wrapper.findAll('button').find(b => b.text() === `modelQuality.${action}`)!.attributes('disabled')).toBeDefined()
    expect(wrapper.text()).not.toContain('modelQuality.verdicts.pending')
  })
})
