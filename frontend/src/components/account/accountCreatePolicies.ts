import type { CreateAccountRequest } from '@/types'
import { applyPlanType } from './credentialsBuilder'

export interface AccountCreatePolicies {
  rateSyncEnabled: boolean
  schedulingEnabled: boolean
  schedulingThreshold: number
  pause5hDisabled: boolean
  pause7dDisabled: boolean
  pause5hThreshold: number | null
  pause7dThreshold: number | null
  resetCreditEnabled: boolean
  resetCredit5hThreshold: number
  resetCredit7dThreshold: number
  planType: string
  imageToolMode: 'inherit' | 'enabled' | 'disabled' | 'block'
  grokClientToolCache: boolean
  grokMediaMode: 'auto' | 'enabled' | 'disabled'
}

export function defaultAccountCreatePolicies(): AccountCreatePolicies {
  return {
    rateSyncEnabled: false,
    schedulingEnabled: false, schedulingThreshold: 100,
    pause5hDisabled: false, pause7dDisabled: false,
    pause5hThreshold: null, pause7dThreshold: null,
    resetCreditEnabled: false, resetCredit5hThreshold: 100, resetCredit7dThreshold: 100,
    planType: '', imageToolMode: 'inherit', grokClientToolCache: true, grokMediaMode: 'auto',
  }
}

export function supportsSchedulingThreshold(platform: string): boolean {
  return ['openai', 'anthropic', 'grok'].includes(platform)
}

// All create paths (including bulk OAuth and session/PAT imports) use the same
// serializer. An untouched control preserves provider-detected values/defaults.
export function applyAccountCreatePolicies<T extends Pick<CreateAccountRequest, 'platform' | 'type' | 'credentials' | 'extra'>>(
  payload: T, policy: AccountCreatePolicies,
): T {
  const credentials = { ...payload.credentials }
  const extra = { ...payload.extra }
  if (supportsSchedulingThreshold(payload.platform) && policy.schedulingEnabled) {
    credentials.account_scheduling_threshold = Math.min(100, Math.max(1, Math.trunc(Number(policy.schedulingThreshold) || 100)))
  }
  if (payload.platform === 'openai') {
    for (const window of ['5h', '7d'] as const) {
      const disabled = window === '5h' ? policy.pause5hDisabled : policy.pause7dDisabled
      const threshold = window === '5h' ? policy.pause5hThreshold : policy.pause7dThreshold
      if (disabled) extra[`auto_pause_${window}_disabled`] = true
      if (threshold != null && Number(threshold) > 0) {
        if (!Number.isFinite(Number(threshold)) || Number(threshold) > 100) throw new Error('admin.accounts.autoPauseThresholdHint')
        extra[`auto_pause_${window}_threshold`] = Number(threshold) / 100
      }
    }
    if (payload.type === 'oauth') {
      if (policy.planType.trim()) applyPlanType(credentials, policy.planType)
      if (policy.resetCreditEnabled) {
        const thresholds = [policy.resetCredit5hThreshold, policy.resetCredit7dThreshold].map(Number)
        if (thresholds.some(value => !Number.isFinite(value) || value <= 0 || value > 100)) {
          throw new Error('admin.accounts.autoResetCredit.thresholdInvalid')
        }
        extra.auto_reset_credit_enabled = true
        extra.auto_reset_credit_5h_threshold = thresholds[0] / 100
        extra.auto_reset_credit_7d_threshold = thresholds[1] / 100
      }
    }
    if (policy.imageToolMode === 'block') {
      extra.codex_image_generation_explicit_tool_policy = 'strip'
      delete extra.codex_image_generation_bridge
    } else if (policy.imageToolMode !== 'inherit') {
      extra.codex_image_generation_bridge = policy.imageToolMode === 'enabled'
      delete extra.codex_image_generation_explicit_tool_policy
    }
  }
  if (payload.platform === 'grok' && payload.type === 'oauth') {
    extra.grok_client_tool_cache_enabled = policy.grokClientToolCache
    if (policy.grokMediaMode !== 'auto') extra.grok_media_eligible = policy.grokMediaMode === 'enabled'
  }
  return {
    ...payload, credentials, ...(Object.keys(extra).length ? { extra } : {}),
    ...(payload.type === 'apikey' && policy.rateSyncEnabled
      ? { upstream_billing_probe_enabled: true, upstream_billing_rate_sync_enabled: true }
      : {}),
  }
}
