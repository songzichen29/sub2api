import type { AccountAPIKeySummary, AccountAPIKeyUpdate } from '@/types'

export const MAX_ACCOUNT_API_KEYS = 20

export interface AccountAPIKeyDraft {
  id?: string
  masked?: string
  last_used_at?: string
  value: string
}

export function accountApiKeyDrafts(keys: AccountAPIKeySummary[]): AccountAPIKeyDraft[] {
  return keys.map(key => ({ ...key, value: '' }))
}

export function accountApiKeyDraftError(entries: AccountAPIKeyDraft[]): string | null {
  if (!entries.length) return 'required'
  if (entries.length > MAX_ACCOUNT_API_KEYS) return 'tooMany'
  if (entries.some(entry => !entry.id && !entry.value.trim())) return 'emptyEntry'
  const newValues = entries.filter(entry => entry.value.trim() || !entry.id).map(entry => entry.value)
  if (!newValues.length) return null
  return accountApiKeyInputError(newValues.join('\n'))
}

export function serializeAccountApiKeyDrafts(entries: AccountAPIKeyDraft[]): AccountAPIKeyUpdate['entries'] {
  return entries.map(entry => entry.value.trim() || !entry.id
    ? { value: entry.value.trim() }
    : { id: entry.id })
}

export function accountApiKeyInputError(value: string): string | null {
  if (!value.trim()) return 'required'
  const keys = value.split(/\r?\n/).map(key => key.trim())
  if (keys.length > MAX_ACCOUNT_API_KEYS) return 'tooMany'
  const encoder = new TextEncoder()
  if (keys.some(key => !key || encoder.encode(key).length > 8192 || /[\s\p{Cc}]/u.test(key))) return 'invalid'
  if (new Set(keys).size !== keys.length) return 'duplicate'
  return null
}

export function applyAccountApiKeys(credentials: Record<string, unknown>, value: string): void {
  const keys = value.split(/\r?\n/).map(key => key.trim())
  credentials.api_key = keys[0]
  if (keys.length > 1) credentials.api_keys = keys
  else delete credentials.api_keys
}
