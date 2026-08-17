import type { AccountUsageQueryConfig } from '@/types'

export type AccountUsageQueryProvider = 'newapi' | 'sub2api'

export interface NewAPIUsageQueryFields {
  baseUrl: string
  accessToken: string
  userId: string
}

export function normalizeUsageQueryProvider(value: unknown): AccountUsageQueryProvider {
  return value === 'sub2api' ? 'sub2api' : 'newapi'
}

export function buildUsageQueryConfig(
  provider: AccountUsageQueryProvider,
  fields: NewAPIUsageQueryFields
): AccountUsageQueryConfig | null {
  if (provider === 'sub2api') {
    return {
      enabled: true,
      provider: 'sub2api'
    }
  }

  const baseUrl = fields.baseUrl.trim()
  const accessToken = fields.accessToken.trim()
  const userId = fields.userId.trim()
  if (!baseUrl || !accessToken || !userId) {
    return null
  }
  return {
    enabled: true,
    provider: 'newapi',
    base_url: baseUrl,
    access_token: accessToken,
    user_id: userId
  }
}
