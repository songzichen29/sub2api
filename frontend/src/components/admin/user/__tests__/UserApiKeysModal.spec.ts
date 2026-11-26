import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import UserApiKeysModal from '../UserApiKeysModal.vue'
import type { AdminUser } from '@/types'

const { getKeys, getGroups, getSubscriptions, updateGroup } = vi.hoisted(() => ({ getKeys: vi.fn(), getGroups: vi.fn(), getSubscriptions: vi.fn(), updateGroup: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: {
  users: { getUserApiKeys: getKeys }, groups: { getAll: getGroups },
  subscriptions: { listByUser: getSubscriptions }, apiKeys: { updateApiKeyGroup: updateGroup },
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
enableAutoUnmount(afterEach)
beforeEach(() => { getGroups.mockResolvedValue([]); getSubscriptions.mockResolvedValue({ items: [], pages: 1 }); updateGroup.mockReset(); getKeys.mockReset(); vi.spyOn(console, 'error').mockImplementation(() => {}) })
afterEach(() => vi.restoreAllMocks())
function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (error: Error) => void
  const promise = new Promise((res, rej) => { resolve = res; reject = rej })
  return { promise, resolve, reject }
}
const user = (id: number) => ({ id, email: `user${id}@example.com`, username: `user${id}` }) as AdminUser
const keys = (id: number, name: string) => ({ items: [{ id, name, key: 'sk-example-key-value-for-tests', status: 'active', created_at: '2026-09-20', group_id: null }] })
async function open() {
  const wrapper = mount(UserApiKeysModal, {
    props: { show: false, user: user(1) },
    global: { stubs: {
      BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
      GroupBadge: true, GroupOptionItem: true, Teleport: true,
    } },
  })
  await wrapper.setProps({ show: true })
  return wrapper
}
async function switchUser(wrapper: Awaited<ReturnType<typeof open>>) {
  await wrapper.setProps({ show: false })
  await wrapper.setProps({ show: true, user: user(2) })
}

describe('user API key loading', () => {
  it('uses the managed user subscriptions and switches between same-group instances', async () => {
    const group = { id: 7, name: 'Monthly', subscription_type: 'subscription', platform: 'openai' }
    const key = { ...keys(1, 'managed-key').items[0], user_id: 1, group_id: 7, subscription_id: 101, group }
    getKeys.mockResolvedValue({ items: [key] })
    getGroups.mockResolvedValue([group])
    getSubscriptions.mockResolvedValue({ items: [101, 102].map((id) => ({ id, group_id: 7, user_id: 1, status: 'active', expires_at: '2099-01-01T00:00:00Z', quota_limit_usd: 2000, quota_used_usd: id === 101 ? 1500 : 200, group })), pages: 1 })
    updateGroup.mockResolvedValue({ api_key: { ...key, subscription_id: 102 } })
    const wrapper = await open()
    await flushPromises()
    expect(getSubscriptions).toHaveBeenCalledWith(1)
    await wrapper.get('button').trigger('click')
    expect(wrapper.findAll('[data-subscription-option]')).toHaveLength(2)
    await wrapper.get('[data-subscription-option="102"]').trigger('click')
    await flushPromises()
    expect(updateGroup).toHaveBeenCalledWith(1, 7, 102)
    expect(wrapper.text()).toContain('#102')
  })

  it('does not display the previous user keys when the next load fails', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = await open(); await flushPromises()
    expect(wrapper.text()).toContain('first-user-key')
    await switchUser(wrapper); await flushPromises()
    expect(wrapper.text()).toContain('user2@example.com')
    expect(wrapper.text()).not.toContain('first-user-key')
  })

  it('does not replace current keys with a late previous response', async () => {
    const old = deferred()
    getKeys.mockReturnValueOnce(old.promise).mockResolvedValueOnce(keys(2, 'current-user-key'))
    const wrapper = await open()
    await switchUser(wrapper); await flushPromises()
    old.resolve(keys(1, 'old-user-key')); await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.text()).not.toContain('old-user-key')
  })

  it('keeps the current request loading when an obsolete request fails', async () => {
    const old = deferred(); const current = deferred()
    getKeys.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise)
    const wrapper = await open()
    await switchUser(wrapper)
    old.reject(new Error('obsolete')); await flushPromises()
    expect(wrapper.find('.animate-spin').exists()).toBe(true)
    current.resolve(keys(2, 'current-user-key')); await flushPromises()
    expect(wrapper.text()).toContain('current-user-key')
    expect(wrapper.find('.animate-spin').exists()).toBe(false)
  })

  it('loads keys when the selected user changes while the dialog is open', async () => {
    getKeys.mockResolvedValueOnce(keys(1, 'first-user-key')).mockResolvedValueOnce(keys(2, 'second-user-key'))
    const wrapper = await open(); await flushPromises()
    await wrapper.setProps({ user: user(2) }); await flushPromises()
    expect(getKeys).toHaveBeenLastCalledWith(2)
    expect(wrapper.text()).toContain('second-user-key')
    expect(wrapper.text()).not.toContain('first-user-key')
  })
})
