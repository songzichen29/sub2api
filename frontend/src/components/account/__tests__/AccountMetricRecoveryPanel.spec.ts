import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountMetricRecoveryPanel from '../AccountMetricRecoveryPanel.vue'

const { getMetricRecovery, saveMetricRecovery, releaseMetricRecovery, listMetricRecoveryEvents } = vi.hoisted(() => ({
  getMetricRecovery: vi.fn(),
  saveMetricRecovery: vi.fn(),
  releaseMetricRecovery: vi.fn(),
  listMetricRecoveryEvents: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { getMetricRecovery, saveMetricRecovery, releaseMetricRecovery, listMetricRecoveryEvents }
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
    listMetricRecoveryEvents.mockResolvedValue({ events: [] })
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
    await wrapper.get('[data-testid="metric-release"]').trigger('click')
    expect(releaseMetricRecovery).toHaveBeenCalledWith(7, 4, 2)
  })

  it('preserves legacy thresholds and sample counts when upgrading the form', async () => {
    getMetricRecovery.mockResolvedValue({ monitor: { ...monitor, config: { enabled: true, cooldown_seconds: 600, success_rate: { pause_threshold: 0.8, recovery_threshold: 0.9, min_samples: 10 } } }, metrics_available: true })
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    expect(saveMetricRecovery).toHaveBeenCalledWith(7, 0, {
      enabled: true, auto_recover: true, cooldown_seconds: 600, recovery_samples: 3,
      success_rate: { pause: { min: 0.8, max: undefined }, recovery: { min: 0.9, max: undefined }, min_samples: 10 }
    })
    expect(wrapper.emitted('updated')).toEqual([[7]])
  })

  it('supports both bounds and automatic recovery off', async () => {
    getMetricRecovery.mockResolvedValue({ monitor: { ...monitor, config: { enabled: true, auto_recover: false, ttft_ms: { pause: { min: 0, max: 1000 }, recovery: { min: 10, max: 800 }, min_samples: 20 } } }, metrics_available: true })
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    expect(saveMetricRecovery).toHaveBeenCalledWith(7, 0, expect.objectContaining({ auto_recover: false, cooldown_seconds: 300, ttft_ms: { pause: { min: 0, max: 1000 }, recovery: { min: 10, max: 800 }, min_samples: 20 } }))
  })

  it('ignores a late load from another account', async () => {
    let resolveFirst!: (value: unknown) => void
    getMetricRecovery.mockImplementationOnce(() => new Promise(resolve => { resolveFirst = resolve }))
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await wrapper.setProps({ accountId: 8 })
    await flushPromises()
    resolveFirst({ monitor: { ...monitor, revision: 99 }, metrics_available: true })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    expect(saveMetricRecovery).toHaveBeenCalledWith(8, 0, expect.anything())
  })

  it('keeps the new account form after a previous save completes', async () => {
    let resolveSave!: (value: unknown) => void
    saveMetricRecovery.mockImplementationOnce(() => new Promise(resolve => { resolveSave = resolve }))
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises()
    await wrapper.get('button.btn-primary').trigger('click')
    await wrapper.setProps({ accountId: 8 }); await flushPromises()
    resolveSave({ ...monitor, revision: 99 }); await flushPromises()
    expect(wrapper.text()).not.toContain('admin.accounts.metricRecovery.revision:99')
    expect(wrapper.emitted('updated')).toEqual([[7]])
  })

  it('requires refresh after a revision conflict', async () => {
    saveMetricRecovery.mockRejectedValueOnce({ status: 409, message: 'Configuration changed' })
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises(); await wrapper.get('button.btn-primary').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.conflict')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
  })

  it('shows unavailable metrics and still permits manual release', async () => {
    getMetricRecovery.mockResolvedValue({ monitor: { ...monitor, config: { enabled: true }, paused: true }, metrics_available: false })
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises()
    expect(wrapper.text()).toContain('admin.accounts.metricRecovery.unavailableHint')
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="metric-release"]').attributes('disabled')).toBeUndefined()
  })

  it('loads audit history with a cursor', async () => {
    listMetricRecoveryEvents.mockResolvedValueOnce({ events: Array.from({ length: 20 }, (_, i) => ({ id: 40-i, action: 'pause', revision: 1, created_at: '2026-11-08T00:00:00Z', evidence: {} })) })
    const wrapper = mount(AccountMetricRecoveryPanel, { props: { accountId: 7 } })
    await flushPromises()
    const more = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.metricRecovery.moreHistory')!
    await more.trigger('click'); await flushPromises()
    expect(listMetricRecoveryEvents).toHaveBeenLastCalledWith(7, 21, 20)
  })
})
