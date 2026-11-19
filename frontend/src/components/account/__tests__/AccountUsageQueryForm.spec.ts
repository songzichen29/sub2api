import { afterEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import AccountUsageQueryForm from '../AccountUsageQueryForm.vue'
import AccountAPIKeyEditor from '../AccountAPIKeyEditor.vue'
import { accountApiKeyDraftError, serializeAccountApiKeyDrafts } from '../accountApiKeys'
import { buildUsageQueryConfig } from '@/utils/accountUsageQuery'

const { previewUsageQuery } = vi.hoisted(() => ({ previewUsageQuery: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { previewUsageQuery } } }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key })
}))
enableAutoUnmount(afterEach)
afterEach(() => vi.clearAllMocks())

const props = {
  enabled: true, provider: 'sub2api' as const, baseUrl: '', accessToken: '', userId: '',
  accountId: 42, accountBaseUrl: 'https://upstream.example/v1',
  apiKeys: [{ id: 'first', masked: '••••1111', value: '' }, { id: 'second', masked: '••••2222', value: '' }]
}

describe('usage query configuration', () => {
  it('previews the selected saved key without submitting credential text', async () => {
    previewUsageQuery.mockResolvedValue({ third_party_quota: { remaining: 12, unit: 'CNY', total_known: false } })
    const wrapper = mount(AccountUsageQueryForm, { props })
    await wrapper.get('[data-testid="usage-query-preview-key"]').setValue('1')
    await wrapper.get('[data-testid="usage-query-preview"]').trigger('click')
    await flushPromises()
    expect(previewUsageQuery).toHaveBeenCalledWith({
      account_id: 42, usage_query: { enabled: true, provider: 'sub2api' },
      account_base_url: 'https://upstream.example/v1', key_id: 'second'
    })
    expect(wrapper.get('[data-testid="third-party-remaining"]').text()).toContain('¥12.00')
  })

  it('uses a newly entered replacement for the preview', async () => {
    previewUsageQuery.mockResolvedValue({ error: 'upstream returned 401' })
    const wrapper = mount(AccountUsageQueryForm, { props: { ...props, apiKeys: [{ id: 'first', value: 'new-key' }] } })
    await wrapper.get('[data-testid="usage-query-preview"]').trigger('click')
    await flushPromises()
    expect(previewUsageQuery.mock.calls[0][0]).toMatchObject({ api_key: 'new-key' })
    expect(previewUsageQuery.mock.calls[0][0]).not.toHaveProperty('key_id')
    expect(wrapper.get('[data-testid="usage-query-error"]').text()).toContain('401')
  })

  it('keeps the saved NewAPI token when blank and uses the same result presentation', async () => {
    previewUsageQuery.mockResolvedValue({ third_party_quota: { remaining: 8, unit: 'USD', total_known: false } })
    const wrapper = mount(AccountUsageQueryForm, { props: { ...props, provider: 'newapi', baseUrl: 'https://panel.example', userId: '12', hasStoredToken: true } })
    await wrapper.get('[data-testid="usage-query-preview"]').trigger('click')
    await flushPromises()
    expect(previewUsageQuery.mock.calls[0][0]).toEqual({ account_id: 42, usage_query: {
      enabled: true, provider: 'newapi', base_url: 'https://panel.example', access_token: '', user_id: '12'
    } })
    expect(wrapper.get('[data-testid="usage-query-result"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="third-party-remaining"]').text()).toContain('$8.00')
    expect(buildUsageQueryConfig('newapi', { baseUrl: 'https://panel.example', accessToken: '', userId: '12' })).toBeNull()
  })

  it('discards a response for a configuration changed while querying', async () => {
    let complete: (value: object) => void = () => undefined
    previewUsageQuery.mockReturnValue(new Promise(resolve => { complete = resolve }))
    const wrapper = mount(AccountUsageQueryForm, { props })
    await wrapper.get('[data-testid="usage-query-preview"]').trigger('click')
    await wrapper.setProps({ accountBaseUrl: 'https://different.example/v1' })
    complete({ third_party_quota: { remaining: 8, unit: 'USD', total_known: false } })
    await flushPromises()
    expect(wrapper.find('[data-testid="third-party-remaining"]').exists()).toBe(false)
  })
})

describe('saved key editor', () => {
  it('keeps blank saved keys, replaces one entry and supports reorder/remove/append', async () => {
    const wrapper = mount(AccountAPIKeyEditor, { props: { modelValue: props.apiKeys } })
    await wrapper.findAll('input')[1].setValue('new-second')
    const changed = wrapper.emitted('update:modelValue')![0][0] as typeof props.apiKeys
    expect(serializeAccountApiKeyDrafts(changed)).toEqual([{ id: 'first' }, { value: 'new-second' }])
    await wrapper.setProps({ modelValue: changed })
    await wrapper.findAll('[aria-label="admin.accounts.apiKeyPool.moveUp"]')[1].trigger('click')
    const reordered = wrapper.emitted('update:modelValue')![1][0] as typeof props.apiKeys
    expect(serializeAccountApiKeyDrafts(reordered)).toEqual([{ value: 'new-second' }, { id: 'first' }])
    await wrapper.setProps({ modelValue: reordered })
    await wrapper.findAll('[aria-label="admin.accounts.apiKeyPool.remove"]')[0].trigger('click')
    expect(wrapper.emitted('update:modelValue')![2][0]).toEqual([props.apiKeys[0]])
    await wrapper.setProps({ modelValue: [props.apiKeys[0]] })
    await wrapper.get('[data-testid="add-account-api-key"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')![3][0]).toEqual([props.apiKeys[0], { value: '' }])
    expect(accountApiKeyDraftError([{ value: 'same' }, { value: 'same' }])).toBe('duplicate')
    expect(accountApiKeyDraftError(props.apiKeys)).toBeNull()
    expect(accountApiKeyDraftError([...props.apiKeys, { value: '' }])).toBe('emptyEntry')
    expect(wrapper.findAll('button').every(button => button.attributes('type') === 'button')).toBe(true)
  })
})
