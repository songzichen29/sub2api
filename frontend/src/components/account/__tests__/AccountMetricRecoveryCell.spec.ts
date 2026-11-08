import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import AccountMetricRecoveryCell from '../AccountMetricRecoveryCell.vue'
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
describe('AccountMetricRecoveryCell', () => {
  it('keeps the configuration entry available with no requests or unavailable monitoring', async () => {
    const wrapper = mount(AccountMetricRecoveryCell, { props: { status: { monitor: { account_id: 7, config: { enabled: true }, revision: 1, paused: true, pause_generation: 1, reason: 'test', trial_requests: 2, trial_started_at: '2026-11-08T00:00:00Z' }, metrics_available: false } } })
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.trial')
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.unavailable')
    await wrapper.get('button').trigger('click'); expect(wrapper.emitted('configure')).toHaveLength(1)
  })
})
