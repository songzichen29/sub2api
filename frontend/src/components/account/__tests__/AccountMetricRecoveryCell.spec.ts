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

  it('keeps metrics visible during refresh and marks retained data after failure', async () => {
    const wrapper = mount(AccountMetricRecoveryCell, { props: { status: { monitor: { account_id: 7, config: { enabled: false }, revision: 1, paused: false, pause_generation: 0, reason: '', trial_requests: 0 }, metrics_available: true }, loading: true } })
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.successShort')
    expect(wrapper.text()).not.toContain('common.loading')
    await wrapper.setProps({ loading: false, error: true })
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.successShort')
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.loadFailed')
    expect(wrapper.text()).not.toContain('admin.accounts.metricRecovery.lastHour')
    wrapper.unmount()
  })
})
