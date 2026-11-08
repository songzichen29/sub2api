import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AccountUsageCell from '../AccountUsageCell.vue'
import type { Account, AccountUsageInfo } from '@/types'

const { getUsage } = vi.hoisted(() => ({ getUsage: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getUsage }, cnProviders: { queryQuota: vi.fn() } } }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)
const windowData = { utilization: 25, resets_at: '2099-01-01T00:00:00Z', remaining_seconds: 3600 }
const account = (fields: Partial<Account>) => ({
  id: 9601, platform: 'openai', type: 'apikey', credentials: {}, extra: {}, ...fields
}) as Account
const mountUsage = (fields: Partial<Account>, usage?: AccountUsageInfo) => mount(AccountUsageCell, {
  props: { account: account(fields), batchedUsage: usage, requestBatchedUsage: vi.fn() }
})

describe('account usage layout with real provider components', () => {
  it.each([
    { name: 'Anthropic', fields: { platform: 'anthropic', type: 'oauth' }, usage: { five_hour: windowData } },
    { name: 'OpenAI', fields: { platform: 'openai', type: 'oauth' }, usage: { five_hour: windowData } },
    { name: 'Antigravity', fields: { platform: 'antigravity', type: 'oauth' }, usage: { antigravity_quota: { 'gemini-3-pro-image': { utilization: 25, reset_time: windowData.resets_at } } } },
    { name: 'Grok', fields: { platform: 'grok', type: 'oauth' }, usage: { grok_billing: { period_type: 'weekly', usage_percent: 25 } } },
    { name: 'Gemini', fields: { platform: 'gemini', type: 'apikey', credentials: { base_url: 'https://generativelanguage.googleapis.com' } }, usage: { gemini_shared_daily: windowData } },
    { name: 'API key quota', fields: { quota_daily_limit: 100, quota_daily_used: 25 } },
    { name: 'CN provider', fields: { platform: 'zhipu', credentials: { account_mode: 'coding' }, extra: { zhipu_5h_used_percent: 25, zhipu_usage_updated_at: new Date().toISOString() } } },
    { name: 'Ollama', fields: { ollama_cloud_usage: { eligible: true, configured: true, snapshot: { status: 'ok', data: { five_hour: { used_percent: 25 } } } } } },
    { name: 'OpenCode', fields: { opencode_go_usage: { eligible: true, snapshot: { status: 'ok', data: { rolling: { percent: 25 } } } } } }
  ])('$name keeps its native usage progress bar', async ({ fields, usage }) => {
    const wrapper = mountUsage(fields as Partial<Account>, usage as AccountUsageInfo | undefined)
    await flushPromises()
    expect(wrapper.get('.account-usage-cell').exists()).toBe(true)
    const row = wrapper.get('.usage-progress-row')
    expect(row.get('.usage-progress-label').text()).not.toBe('')
    expect(row.get('.usage-progress-value').text()).toBe('25%')
    expect(row.get('.usage-progress-track > div').attributes('style')).toContain('width: 25%')
  })

  it('renders only used percentage when total is known and preserves active refresh', async () => {
    const quota = { plan_name: 'test', remaining: 75, used: 25, total: 100, unit: 'USD', utilization: 0.25 }
    getUsage.mockResolvedValue({ third_party_quota: { ...quota, remaining: 60, used: 40, utilization: 0.4 } })
    const wrapper = mountUsage({ extra: { usage_query: { enabled: true } } }, { third_party_quota: quota })
    expect(wrapper.get('[data-testid="third-party-usage-percent"] .usage-progress-value').text()).toBe('25%')
    expect(wrapper.find('.usage-progress-track').exists()).toBe(false)
    expect(wrapper.find('[data-testid="third-party-remaining"]').exists()).toBe(false)
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(9601, 'active', true)
    expect(wrapper.get('.usage-progress-value').text()).toBe('40%')
    expect(wrapper.find('[data-testid="third-party-remaining"]').exists()).toBe(false)
  })

  it('keeps unknown-total balances truthful without fabricating a bar or denominator', () => {
    const wrapper = mountUsage({ extra: { usage_query: { enabled: true } } }, {
      third_party_quota: { plan_name: 'balance', remaining: 75, used: 0, total: 0, unit: 'USD', utilization: 0, total_known: false }
    })
    expect(wrapper.find('.usage-progress-row').exists()).toBe(false)
    expect(wrapper.get('[data-testid="third-party-remaining"]').text()).toContain('$75.00')
    expect(wrapper.get('[data-testid="third-party-remaining"]').text()).not.toContain('/')
  })

  it.each(['openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'zhipu'] as const)(
    'automatically queues configured %s API-key usage', async platform => {
      const requestBatchedUsage = vi.fn()
      const wrapper = mount(AccountUsageCell, {
        props: {
          account: account({ platform, credentials: { base_url: 'https://upstream.example/v1' }, extra: { usage_query: { enabled: true, provider: 'sub2api' } } }),
          requestBatchedUsage,
        },
      })
      await flushPromises()
      expect(requestBatchedUsage).toHaveBeenCalledOnce()
      expect(requestBatchedUsage.mock.calls[0][0]).toEqual(wrapper.props('account'))
    }
  )

  it('queries a third-party Gemini key without the desktop batch loader', async () => {
    getUsage.mockReset().mockResolvedValue({ third_party_quota: { remaining: 75, used: 25, total: 100, utilization: 0.25 } })
    const wrapper = mount(AccountUsageCell, {
      props: { account: account({ id: 9701, platform: 'gemini', credentials: { base_url: 'https://upstream.example/v1' }, extra: { usage_query: { enabled: true, provider: 'sub2api' } } }) },
    })
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(9701)
    expect(wrapper.get('[data-testid="third-party-usage-percent"] .usage-progress-value').text()).toBe('25%')
    expect(wrapper.get('.usage-progress-value').text()).toBe('25%')
    expect(wrapper.find('.usage-progress-track').exists()).toBe(false)
  })

  it('starts a fresh query after enabling usage on an existing list row', async () => {
    const requestBatchedUsage = vi.fn()
    const wrapper = mount(AccountUsageCell, { props: { account: account({}), requestBatchedUsage } })
    await flushPromises()
    expect(requestBatchedUsage).not.toHaveBeenCalled()
    await wrapper.setProps({ account: account({ extra: { usage_query: { enabled: true, provider: 'sub2api' } } }) })
    await flushPromises()
    expect(requestBatchedUsage).toHaveBeenCalledWith(wrapper.props('account'), { force: true })
    requestBatchedUsage.mockClear()
    await wrapper.setProps({ account: account({ extra: { usage_query: { enabled: true, provider: 'sub2api' } } }) })
    await flushPromises()
    expect(requestBatchedUsage).not.toHaveBeenCalled()
  })

  it('keeps a retry action after a query error', async () => {
    getUsage.mockReset().mockResolvedValue({ third_party_quota: { remaining: 60, used: 40, total: 100, utilization: 0.4 } })
    const wrapper = mountUsage({ extra: { usage_query: { enabled: true } } }, { error: 'upstream unavailable' })
    expect(wrapper.text()).toContain('upstream unavailable')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(getUsage).toHaveBeenCalledWith(9601, 'active', true)
    expect(wrapper.get('.usage-progress-value').text()).toBe('40%')
  })
})
