import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountMetricRecoveryPanel from '../AccountMetricRecoveryPanel.vue'

const { getMetricRecovery, saveMetricRecovery, releaseMetricRecovery } = vi.hoisted(() => ({
  getMetricRecovery: vi.fn(),
  saveMetricRecovery: vi.fn(),
  releaseMetricRecovery: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getMetricRecovery, saveMetricRecovery, releaseMetricRecovery }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, values?: Record<string, unknown>) => values ? `${key}:${values.revision}` : key })
}))

const monitor = {
  account_id: 7,
  config: { enabled: false },
  revision: 0,
  paused: false,
  pause_generation: 0,
  reason: '',
  trial_requests: 0
}

describe('AccountMetricRecoveryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getMetricRecovery.mockResolvedValue({ monitor, metrics_available: false })
    saveMetricRecovery.mockResolvedValue({ ...monitor, revision: 1 })
    releaseMetricRecovery.mockResolvedValue({ ...monitor, revision: 1 })
  })

  it('loads disabled by default and saves an explicit configuration revision', async () => {
    const wrapper = mount(AccountMetricRecoveryPanel, {
      props: { accountId: 7 },
      global: { stubs: { Toggle: true, Icon: true } }
    })
    await flushPromises()

    expect(getMetricRecovery).toHaveBeenCalledWith(7)
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.title')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(saveMetricRecovery).toHaveBeenCalledWith(7, 0, expect.objectContaining({ enabled: false }))
  })

  it('shows a paused state and releases with the current pause generation', async () => {
    getMetricRecovery.mockResolvedValueOnce({
      monitor: { ...monitor, config: { enabled: true }, revision: 4, paused: true, pause_generation: 2, reason: 'success_rate below' },
      metrics_available: false
    })
    const wrapper = mount(AccountMetricRecoveryPanel, {
      props: { accountId: 7 },
      global: { stubs: { Toggle: true, Icon: true } }
    })
    await flushPromises()

    expect(wrapper.text()).toContain('success_rate below')
    await wrapper.get('button.btn-secondary').trigger('click')
    expect(releaseMetricRecovery).toHaveBeenCalledWith(7, 4, 2)
  })
})
