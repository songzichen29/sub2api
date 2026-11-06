import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MonitorDisplayDialog from '../MonitorDisplayDialog.vue'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), list: vi.fn(), groups: vi.fn(), error: vi.fn(), success: vi.fn() }))
vi.mock('@/api/admin/channelMonitor', () => ({ getDisplayConfig: mocks.get, updateDisplayConfig: mocks.update }))
vi.mock('@/api/admin', () => ({ adminAPI: { channelMonitor: { list: mocks.list }, groups: { getAllIncludingInactive: mocks.groups } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: mocks.success }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, args?: { name?: string }) => args?.name ? `${key} ${args.name}` : key }) }))

function mountDialog() {
  return mount(MonitorDisplayDialog, {
    props: { show: true },
    global: { stubs: { BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }, Icon: true } },
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.get.mockResolvedValue({ provider_order: ['grok', 'openai'], monitor_order: [3, 2, 1], group_mappings: [{ monitor_id: 2, group_id: 8 }] })
  mocks.list.mockImplementation(({ page }) => Promise.resolve(page === 1 ? {
    items: [{ id: 1, name: 'OpenAI A', provider: 'openai', enabled: true }, { id: 2, name: 'OpenAI B', provider: 'openai', enabled: false }], total: 3,
  } : { items: [{ id: 3, name: 'Grok A', provider: 'grok', enabled: true }], total: 3 }))
  mocks.groups.mockResolvedValue([{ id: 8, name: 'OpenAI Billing', platform: 'openai', rate_multiplier: 0.5, status: 'active' }])
  mocks.update.mockImplementation(async value => value)
})

describe('V1 display settings', () => {
  it('loads all pages and saves category order, channel order and explicit billing links', async () => {
    const wrapper = mountDialog()
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(wrapper.findAll('h4').slice(0, 2).map(row => row.text())).toEqual(['monitorCommon.providers.grok', 'monitorCommon.providers.openai'])
    const openai = wrapper.findAll('section')[1]!
    expect(openai.findAll('select').map(select => select.attributes('aria-label'))).toEqual(['OpenAI B · monitorDisplay.billingGroup', 'OpenAI A · monitorDisplay.billingGroup'])
    await wrapper.get('[aria-label="monitorDisplay.moveUp monitorCommon.providers.openai"]').trigger('click')
    await wrapper.get('[aria-label="monitorDisplay.moveUp OpenAI A"]').trigger('click')
    await wrapper.get('select[aria-label="OpenAI A · monitorDisplay.billingGroup"]').setValue('8')
    await wrapper.get('select[aria-label="OpenAI B · monitorDisplay.billingGroup"]').setValue('0')
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.update).toHaveBeenCalledWith(expect.objectContaining({
      provider_order: expect.arrayContaining(['openai', 'grok']), monitor_order: [1, 2, 3], group_mappings: [{ monitor_id: 1, group_id: 8 }],
    }))
    expect(mocks.update.mock.calls[0]![0].provider_order.slice(0, 2)).toEqual(['openai', 'grok'])
    expect(wrapper.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('does not overwrite settings after a load failure and preserves edits after save failure', async () => {
    mocks.get.mockRejectedValueOnce(new Error('read unavailable'))
    const failed = mountDialog()
    await flushPromises()
    expect(failed.get('button.btn-primary').attributes('disabled')).toBeDefined()
    failed.unmount()
    const wrapper = mountDialog()
    await flushPromises()
    await wrapper.get('[aria-label="monitorDisplay.moveUp monitorCommon.providers.openai"]').trigger('click')
    mocks.update.mockRejectedValueOnce(new Error('write unavailable'))
    await wrapper.get('button.btn-primary').trigger('click')
    await flushPromises()
    expect(wrapper.find('h4').text()).toBe('monitorCommon.providers.openai')
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(mocks.error).toHaveBeenCalled()
    wrapper.unmount()
  })
})
