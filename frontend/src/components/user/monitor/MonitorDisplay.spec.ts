import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import MonitorCardGrid from './MonitorCardGrid.vue'
import MonitorCard from './MonitorCard.vue'
import type { UserMonitorView } from '@/api/channelMonitor'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string, args?: { rate?: number }) => args?.rate != null ? `${key} ${args.rate}×` : key }),
}))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorQuotaVisible: () => false }))

const item = (id: number, provider: UserMonitorView['provider']): UserMonitorView => ({
  id, name: `Channel ${id}`, provider, group_name: '', primary_model: 'model', primary_status: 'operational',
  primary_latency_ms: 200, primary_ping_latency_ms: 100, availability_7d: 100, extra_models: [], timeline: [],
})

describe('V1 configured monitor display', () => {
  it('preserves server category and channel order after refresh', async () => {
    const wrapper = shallowMount(MonitorCardGrid, { props: {
      items: [item(3, 'grok'), item(2, 'openai'), item(1, 'openai')], window: '7d', countdownSeconds: 30, loading: false, detailCache: {},
    } })
    expect(wrapper.findAll('h2').map(row => row.text())).toEqual(['monitorCommon.providers.grok', 'monitorCommon.providers.openai'])
    expect(wrapper.findAllComponents(MonitorCard).map(card => card.props('item').id)).toEqual([3, 2, 1])
    await wrapper.setProps({ items: [item(1, 'openai'), item(2, 'openai'), item(3, 'grok')] })
    expect(wrapper.findAll('h2').map(row => row.text())).toEqual(['monitorCommon.providers.openai', 'monitorCommon.providers.grok'])
    expect(wrapper.findAllComponents(MonitorCard).map(card => card.props('item').id)).toEqual([1, 2, 3])
    wrapper.unmount()
  })

  it('shows zero multipliers and hides rates absent from the authenticated response', async () => {
    const wrapper = shallowMount(MonitorCard, { props: {
      item: { ...item(1, 'openai'), billing_group_name: 'My group', rate_multiplier: 0 }, window: '7d', availabilityValue: 100, countdownSeconds: 30,
    } })
    expect(wrapper.get('[data-testid="monitor-rate"]').text()).toContain('My group · monitorDisplay.currentRate 0×')
    await wrapper.setProps({ item: item(1, 'openai') })
    expect(wrapper.find('[data-testid="monitor-rate"]').exists()).toBe(false)
    wrapper.unmount()
  })
})
