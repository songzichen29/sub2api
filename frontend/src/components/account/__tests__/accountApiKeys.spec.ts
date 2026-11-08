import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { accountApiKeyInputError, applyAccountApiKeys } from '../accountApiKeys'
import AccountAPIKeysInput from '../AccountAPIKeysInput.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('ordered account API keys', () => {
  it('validates and serializes in priority order without retaining an old pool', () => {
    const credentials: Record<string, unknown> = { base_url: 'https://upstream.example', api_keys: ['old'] }
    expect(accountApiKeyInputError(' primary\r\nbackup ')).toBeNull()
    applyAccountApiKeys(credentials, ' primary\r\nbackup ')
    expect(credentials).toEqual({ base_url: 'https://upstream.example', api_key: 'primary', api_keys: ['primary', 'backup'] })
    applyAccountApiKeys(credentials, 'replacement')
    expect(credentials).not.toHaveProperty('api_keys')
    for (const [value, error] of [['', 'required'], ['a\na', 'duplicate'], ['a\n', 'invalid'], ['a b', 'invalid'], ['a\u0000b', 'invalid'], ['a'.repeat(8193), 'invalid'], [Array.from({ length: 21 }, (_, i) => `k${i}`).join('\n'), 'tooMany']]) {
      expect(accountApiKeyInputError(value)).toBe(error)
    }
  })

  it('adds, reorders, removes and pastes keys without submitting the outer form', async () => {
    const wrapper = mount(defineComponent({
      components: { AccountAPIKeysInput },
      data: () => ({ keys: 'primary' }),
      template: '<form><AccountAPIKeysInput v-model="keys" /></form>',
    }))
    await wrapper.get('[data-testid="add-account-api-key"]').trigger('click')
    await wrapper.findAll('input')[1].setValue('backup')
    expect(wrapper.vm.keys).toBe('primary\nbackup')
    await wrapper.findAll('[aria-label="admin.accounts.apiKeyPool.moveUp"]')[1].trigger('click')
    expect(wrapper.vm.keys).toBe('backup\nprimary')
    await wrapper.findAll('[aria-label="admin.accounts.apiKeyPool.remove"]')[1].trigger('click')
    expect(wrapper.vm.keys).toBe('backup')
    await wrapper.get('input').trigger('paste', { clipboardData: { getData: () => 'one\r\ntwo' } })
    expect(wrapper.vm.keys).toBe('one\ntwo')
    expect(wrapper.findAll('input').every(input => input.attributes('type') === 'password')).toBe(true)
    expect(wrapper.findAll('button').every(button => button.attributes('type') === 'button')).toBe(true)
  })
})
