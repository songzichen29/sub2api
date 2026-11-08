export const MAX_ACCOUNT_API_KEYS = 20

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
