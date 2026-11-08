import { describe, expect, it } from 'vitest'
import { applyAccountCreatePolicies, defaultAccountCreatePolicies } from '../accountCreatePolicies'

describe('account create policies', () => {
  it('preserves provider-detected defaults and does not mutate the original payload', () => {
    const payload = { platform: 'openai' as const, type: 'oauth' as const, credentials: { plan_type: 'pro_5x', access_token: 'secret' }, extra: { other_policy: true } }
    expect(applyAccountCreatePolicies(payload, defaultAccountCreatePolicies())).toEqual(payload)
  })

  it('serializes OAuth policy controls with the same units as edit', () => {
    const policy = { ...defaultAccountCreatePolicies(), schedulingEnabled: true, schedulingThreshold: 85, pause5hThreshold: 75, pause7dDisabled: true, resetCreditEnabled: true, resetCredit5hThreshold: 90, resetCredit7dThreshold: 95, planType: 'plus', imageToolMode: 'block' as const }
    const result = applyAccountCreatePolicies({ platform: 'openai', type: 'oauth', credentials: { plan_type: 'free' }, extra: {} }, policy)
    expect(result.credentials).toEqual({ plan_type: 'plus', account_scheduling_threshold: 85 })
    expect(result.extra).toEqual({ auto_pause_5h_threshold: 0.75, auto_pause_7d_disabled: true, auto_reset_credit_enabled: true, auto_reset_credit_5h_threshold: 0.9, auto_reset_credit_7d_threshold: 0.95, codex_image_generation_explicit_tool_policy: 'strip' })
  })

  it('restricts OAuth policies to their platform and account type', () => {
    const policy = { ...defaultAccountCreatePolicies(), resetCreditEnabled: true, planType: 'pro', grokMediaMode: 'enabled' as const, grokClientToolCache: false, imageToolMode: 'disabled' as const }
    const apiKey = applyAccountCreatePolicies({ platform: 'openai', type: 'apikey', credentials: {}, extra: {} }, policy)
    expect(apiKey.extra).toEqual({ codex_image_generation_bridge: false })
    expect(apiKey.credentials).toEqual({})
    const grok = applyAccountCreatePolicies({ platform: 'grok', type: 'oauth', credentials: {}, extra: {} }, policy)
    expect(grok.extra).toEqual({ grok_client_tool_cache_enabled: false, grok_media_eligible: true })
    expect(applyAccountCreatePolicies({ platform: 'gemini', type: 'apikey', credentials: {}, extra: {} }, policy).extra).toEqual({})
  })

  it('enables probing with rate sync and rejects invalid credit thresholds', () => {
    const result = applyAccountCreatePolicies({ platform: 'anthropic', type: 'apikey', credentials: {} }, { ...defaultAccountCreatePolicies(), rateSyncEnabled: true })
    expect(result).toMatchObject({ upstream_billing_probe_enabled: true, upstream_billing_rate_sync_enabled: true })
    for (const threshold of [0, -1, 101, NaN]) {
      expect(() => applyAccountCreatePolicies({ platform: 'openai', type: 'oauth', credentials: {} }, { ...defaultAccountCreatePolicies(), resetCreditEnabled: true, resetCredit5hThreshold: threshold })).toThrow('admin.accounts.autoResetCredit.thresholdInvalid')
    }
  })
})
