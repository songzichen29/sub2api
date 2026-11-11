import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DOMWrapper, flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'
import AccountActionMenu from '@/components/admin/account/AccountActionMenu.vue'
import AccountMetricRecoveryCell from '@/components/account/AccountMetricRecoveryCell.vue'
import AccountMetricRecoveryPanel from '@/components/account/AccountMetricRecoveryPanel.vue'

const {
  listAccounts,
  listWithEtag,
  getById,
  getBatchTodayStats,
  getBatchMetricRecovery,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  refreshCredentials,
  showError,
  showWarning
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getById: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getBatchMetricRecovery: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  refreshCredentials: vi.fn(),
  showError: vi.fn(),
  showWarning: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      getById,
      listWithEtag,
      getBatchTodayStats,
      getBatchMetricRecovery,
      listTags: vi.fn().mockResolvedValue([]),
      getUpstreamBillingProbeSettings,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn(),
      refreshCredentials
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showWarning, showSuccess: vi.fn(), showInfo: vi.fn() })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ token: 'test-token', isSimpleMode: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

const DataTableStub = defineComponent({
  props: { data: { type: Array, default: () => [] }, loading: Boolean },
  template: `
    <div>
      <div v-for="row in data" :key="row.id" :data-account-name="row.name">
        <slot name="cell-groups" :row="row" />
        <slot name="cell-last_used_at" :row="row" :value="row.last_used_at" />
        <slot name="cell-actions" :row="row" />
      </div>
    </div>
  `
})

const AccountGroupsCellStub = defineComponent({
  props: { groups: { type: Array, default: () => [] } },
  template: '<span data-test="account-groups">{{ groups.map(group => group.name).join(",") }}</span>'
})

const EditAccountModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="edit-account">{{ show ? account?.name : "" }}</div>'
})

const AccountTestModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="test-account">{{ show ? account?.name : "" }}</div>'
})

const AccountStatsModalStub = defineComponent({
  props: { show: Boolean, account: { type: Object, default: null } },
  template: '<div data-test="stats-account">{{ show ? account?.name : "" }}</div>'
})

function mountView(stubActionMenu = true) {
  return mount(AccountsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
        DataTable: DataTableStub,
        AccountTableActions: { template: '<div><button data-test="manual-refresh" @click="$emit(\'refresh\')">refresh</button><slot name="after" /></div>' },
        AccountTableFilters: true,
        AccountBulkActionsBar: true,
        Pagination: true,
        ConfirmDialog: true,
        AccountActionMenu: stubActionMenu,
        ImportDataModal: true,
        ReAuthAccountModal: true,
        AccountTestModal: AccountTestModalStub,
        AccountStatsModal: AccountStatsModalStub,
        ScheduledTestsPanel: true,
        SyncFromCrsModal: true,
        TempUnschedStatusModal: true,
        ErrorPassthroughRulesModal: true,
        TLSFingerprintProfilesModal: true,
        CreateAccountModal: true,
        EditAccountModal: EditAccountModalStub,
        AccountMetricRecoveryPanel: true,
        BulkEditAccountModal: true,
        PlatformTypeBadge: true,
        AccountCapacityCell: true,
        AccountStatusIndicator: true,
        AccountTodayStatsCell: true,
        AccountGroupsCell: AccountGroupsCellStub,
        AccountUsageCell: true,
        UpstreamBillingRateCell: true,
        HelpTooltip: true,
        Icon: true,
        Teleport: stubActionMenu
      }
    }
  })
}

const listRow = {
  id: 42,
  name: 'compact row',
  platform: 'openai',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  concurrency: 2,
  priority: 1,
  group_ids: [7],
  extra: {},
  credentials: {}
}

const fullAccount = {
  ...listRow,
  groups: [{ id: 7, name: 'codex', platform: 'openai' }],
  account_groups: [{ account_id: 42, group_id: 7 }],
  credentials: { api_key: 'redacted' },
  extra: { detail_only: true }
}

describe('admin AccountsView lite account list', () => {
  beforeEach(() => {
    localStorage.clear()
    listAccounts.mockReset().mockResolvedValue({ items: [listRow], total: 1, page: 1, page_size: 20, pages: 1 })
    listWithEtag.mockReset().mockResolvedValue({ notModified: true, etag: 'compact-etag', data: null })
    getById.mockReset().mockResolvedValue(fullAccount)
    getBatchTodayStats.mockReset().mockResolvedValue({ stats: {} })
    getBatchMetricRecovery.mockReset().mockResolvedValue({ accounts: { '42': { monitor: { account_id: 42, config: { enabled: true }, paused: true, reason: 'test pause' }, metrics_available: true } } })
    getUpstreamBillingProbeSettings.mockReset().mockResolvedValue({ enabled: true })
    getAllProxies.mockReset().mockResolvedValue([])
    getAllGroups.mockReset().mockResolvedValue([{ id: 7, name: 'codex', platform: 'openai' }])
    refreshCredentials.mockReset()
    showError.mockReset()
    showWarning.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps lite=1 on the initial list request', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) })
    )
    wrapper.unmount()
  })

  it('maps group_ids through the group catalog for the table cell', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="account-groups"]').text()).toBe('codex')
    wrapper.unmount()
  })

  it('opens metric configuration from the list and refreshes after changes', async () => {
    const wrapper = mountView()
    await flushPromises()
    expect(getBatchMetricRecovery).toHaveBeenCalledWith([42], expect.any(AbortSignal))
    const cell = wrapper.getComponent(AccountMetricRecoveryCell)
    expect(cell.text()).toContain('admin.accounts.metricRecovery.paused')
    await cell.get('button').trigger('click'); await flushPromises()
    const panel = wrapper.getComponent(AccountMetricRecoveryPanel)
    expect(panel.props('accountId')).toBe(42)
    getBatchMetricRecovery.mockClear()
    panel.vm.$emit('updated', 42); await flushPromises()
    expect(getBatchMetricRecovery).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('keeps the action menu open during internal scrolling but closes it on table scrolling', async () => {
    const wrapper = mountView(false)
    await flushPromises()

    const trigger = wrapper.findAll('button').find(button => button.text() === 'common.more')!
    await trigger.trigger('click')
    const menu = new DOMWrapper(document.body.querySelector('.action-menu-content')!)
    menu.element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    menu.get('button').element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(true)

    wrapper.getComponent(DataTableStub).element.dispatchEvent(new Event('scroll'))
    await flushPromises()
    expect(wrapper.findComponent(AccountActionMenu).props('show')).toBe(false)
    wrapper.unmount()
  })

  it('keeps lite=1 on automatic ETag refreshes', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const wrapper = mountView()
    await flushPromises()

    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(listWithEtag).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({ lite: '1' }),
      expect.objectContaining({ etag: null })
    )
    wrapper.unmount()
  })

  it('updates runtime fields without reordering or replacing the visible rows', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const second = { ...listRow, id: 43, name: 'second row' }
    listAccounts.mockResolvedValue({ items: [listRow, second], total: 2, pages: 1 })
    const updated = { ...listRow, last_used_at: '2026-11-11T08:00:00Z', current_concurrency: 1, status: 'error' }
    listWithEtag.mockResolvedValue({ notModified: false, etag: 'new', data: { items: [second, updated], total: 2, pages: 1 } })
    const wrapper = mountView()
    await flushPromises()
    const firstElement = wrapper.get('[data-account-name="compact row"]').element

    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()

    expect(wrapper.findAll('[data-account-name]').map(row => row.attributes('data-account-name'))).toEqual(['compact row', 'second row'])
    expect(wrapper.get('[data-account-name="compact row"]').element).toBe(firstElement)
    expect(wrapper.getComponent(DataTableStub).props('data')[0]).toMatchObject(updated)
    expect(wrapper.getComponent(DataTableStub).props('loading')).toBe(false)
    expect(getBatchMetricRecovery).toHaveBeenCalledTimes(2)
    expect(listAccounts).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it.each([true, false])('refreshes metrics once even when visible account data is unchanged (304=%s)', async (notModified) => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    listWithEtag.mockResolvedValue({ notModified, etag: 'new', data: { items: [listRow], total: 1, pages: 1 } })
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()
    expect(getBatchMetricRecovery).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('defers page membership changes until manual synchronization', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    const second = { ...listRow, id: 43, name: 'second row' }
    listWithEtag.mockResolvedValue({ notModified: false, etag: 'new', data: { items: [second], total: 1, pages: 1 } })
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('compact row')
    expect(wrapper.text()).toContain('admin.accounts.listPendingSyncHint')

    listAccounts.mockResolvedValue({ items: [second], total: 1, pages: 1 })
    const syncButton = wrapper.findAll('button').find(button => button.text() === 'admin.accounts.listPendingSyncAction')!
    await syncButton.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('second row')
    expect(wrapper.text()).not.toContain('admin.accounts.listPendingSyncHint')
    wrapper.unmount()
  })

  it('ignores an automatic response started before a manual refresh', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    let finish!: (value: unknown) => void
    listWithEtag.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    listAccounts.mockResolvedValue({ items: [{ ...listRow, name: 'manually refreshed', updated_at: 'new' }], total: 1, pages: 1 })
    await wrapper.get('[data-test="manual-refresh"]').trigger('click')
    await flushPromises()
    finish({ notModified: false, etag: 'stale', data: { items: [{ ...listRow, updated_at: 'old' }], total: 1, pages: 1 } })
    await flushPromises()
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('manually refreshed')
    wrapper.unmount()
  })

  it.each(['sort', 'page', 'pageSize'])('fully synchronizes on %s and rejects the previous automatic response', async (action) => {
    vi.useFakeTimers()
    vi.spyOn(document, 'hidden', 'get').mockReturnValue(false)
    localStorage.setItem('account-auto-refresh', JSON.stringify({ enabled: true, interval_seconds: 5 }))
    listAccounts.mockResolvedValueOnce({ items: [listRow], total: 30, pages: 2 })
    let finish!: (value: unknown) => void
    listWithEtag.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await vi.advanceTimersByTimeAsync(6000)
    const second = { ...listRow, id: 43, name: 'new first row' }
    const refreshed = { ...listRow, updated_at: 'new' }
    listAccounts.mockResolvedValue({ items: [second, refreshed], total: 30, pages: 2 })
    if (action === 'sort') wrapper.getComponent(DataTableStub).vm.$emit('sort', 'last_used_at', 'desc')
    if (action === 'page') wrapper.getComponent({ name: 'Pagination' }).vm.$emit('update:page', 2)
    if (action === 'pageSize') wrapper.getComponent({ name: 'Pagination' }).vm.$emit('update:pageSize', 50)
    await flushPromises()
    finish({ notModified: false, etag: 'stale', data: { items: [{ ...listRow, name: 'stale row', updated_at: 'old' }], total: 1, pages: 1 } })
    await flushPromises()
    expect(wrapper.findAll('[data-account-name]').map(row => row.attributes('data-account-name'))).toEqual(['new first row', 'compact row'])
    expect(wrapper.text()).not.toContain('admin.accounts.listPendingSyncHint')
    expect(listAccounts).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('loads the full account by id before opening edit, test, and stats actions', async () => {
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    expect(editButton).toBeTruthy()
    await editButton!.trigger('click')
    await flushPromises()
    expect(getById).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('compact row')

    const menu = wrapper.findComponent(AccountActionMenu)
    menu.vm.$emit('test', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(2)
    expect(wrapper.get('[data-test="test-account"]').text()).toBe('compact row')

    menu.vm.$emit('stats', listRow)
    await flushPromises()
    expect(getById).toHaveBeenCalledTimes(3)
    expect(wrapper.get('[data-test="stats-account"]').text()).toBe('compact row')
    wrapper.unmount()
  })

  it('shows the warning and patches the account after a partial Antigravity refresh', async () => {
    refreshCredentials.mockResolvedValue({
      account: { ...fullAccount, name: 'refreshed account' },
      message: 'Token refreshed, but project_id is temporarily unavailable',
      warning: 'missing_project_id_temporary'
    })
    const wrapper = mountView(false)
    await flushPromises()

    wrapper.findComponent(AccountActionMenu).vm.$emit('refresh-token', listRow)
    await flushPromises()

    expect(refreshCredentials).toHaveBeenCalledWith(42)
    expect(wrapper.get('[data-account-name]').attributes('data-account-name')).toBe('refreshed account')
    expect(showWarning).toHaveBeenCalledWith('Token refreshed, but project_id is temporarily unavailable')
    wrapper.unmount()
  })

  it('shows an error and keeps the modal closed when detail loading fails', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getById.mockRejectedValueOnce(new Error('detail failed'))
    const wrapper = mountView()
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.text().includes('common.edit'))
    await editButton!.trigger('click')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('detail failed')
    expect(wrapper.get('[data-test="edit-account"]').text()).toBe('')
    consoleError.mockRestore()
    wrapper.unmount()
  })
})
