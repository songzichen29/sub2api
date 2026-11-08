import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { useAccountMetricRecoveryList } from '../useAccountMetricRecoveryList'

const { getBatchMetricRecovery } = vi.hoisted(() => ({ getBatchMetricRecovery: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getBatchMetricRecovery } } }))
describe('useAccountMetricRecoveryList', () => {
  beforeEach(() => { vi.resetAllMocks() })
  it('batches at most 100 accounts and does not query per row', async () => {
    const accounts = ref<{ id: number }[]>([])
    getBatchMetricRecovery.mockResolvedValue({ accounts: {} })
    const wrapper = mount(defineComponent({ setup() { return useAccountMetricRecoveryList(accounts) }, template: '<div />' }))
    accounts.value = Array.from({ length: 105 }, (_, i) => ({ id: i+1 }))
    await flushPromises()
    expect(getBatchMetricRecovery).toHaveBeenCalledTimes(2)
    expect(getBatchMetricRecovery.mock.calls.map(call => call[0].length)).toEqual([100, 5])
    wrapper.unmount()
  })
  it('discards stale pages and represents errors as unknown', async () => {
    const accounts = ref<{ id: number }[]>([])
    let resolveOld!: (v: unknown) => void
    getBatchMetricRecovery.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    getBatchMetricRecovery.mockResolvedValueOnce({ accounts: { '8': { metrics_available: false } } })
    const wrapper = mount(defineComponent({ setup() { return useAccountMetricRecoveryList(accounts) }, template: '<div />' }))
    accounts.value = [{ id: 7 }]; await flushPromises()
    accounts.value = [{ id: 8 }]; await flushPromises()
    resolveOld({ accounts: { '7': {} } }); await flushPromises()
    expect(wrapper.vm.statuses).toEqual({ '8': { metrics_available: false } })
    getBatchMetricRecovery.mockRejectedValueOnce({ response: { status: 403 } })
    await wrapper.vm.refresh()
    expect(wrapper.vm.error).toBe(true); expect(wrapper.vm.statuses).toEqual({})
    wrapper.unmount()
  })
})
