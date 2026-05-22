import { apiClient } from './client'
import type {
  AdminDataImportApply,
  AdminDataImportResult,
  AdminDataPayload,
  AdminGroup,
  Proxy as ProxyConfig
} from '@/types'
import type { AccountImportApplyTemplate } from '@/api/admin/settings'

export interface StandaloneAccountImportStatus {
  enabled: boolean
  password_configured: boolean
}

export interface StandaloneAccountImportVerifyResult {
  token: string
  expires_at: string
}

export interface StandaloneAccountImportOptions {
  groups: AdminGroup[]
  proxies: ProxyConfig[]
  tags: string[]
}

export interface StandaloneAccountImportPayload {
  data: AdminDataPayload
  skip_default_group_bind?: boolean
  apply?: AdminDataImportApply
}

const tokenHeaders = (token: string) => ({
  'X-Account-Import-Token': token
})

export async function getStandaloneAccountImportStatus(): Promise<StandaloneAccountImportStatus> {
  const { data } = await apiClient.get<StandaloneAccountImportStatus>('/account-import/status')
  return data
}

export async function verifyStandaloneAccountImportPassword(
  password: string
): Promise<StandaloneAccountImportVerifyResult> {
  const { data } = await apiClient.post<StandaloneAccountImportVerifyResult>('/account-import/verify', { password })
  return data
}

export async function getStandaloneAccountImportTemplates(token: string): Promise<AccountImportApplyTemplate[]> {
  const { data } = await apiClient.get<{ templates?: AccountImportApplyTemplate[] }>('/account-import/templates', {
    headers: tokenHeaders(token)
  })
  return Array.isArray(data?.templates) ? data.templates : []
}

export async function getStandaloneAccountImportOptions(token: string): Promise<StandaloneAccountImportOptions> {
  const { data } = await apiClient.get<StandaloneAccountImportOptions>('/account-import/options', {
    headers: tokenHeaders(token)
  })
  return {
    groups: Array.isArray(data?.groups) ? data.groups : [],
    proxies: Array.isArray(data?.proxies) ? data.proxies : [],
    tags: Array.isArray(data?.tags) ? data.tags : []
  }
}

export async function importStandaloneAccountData(
  token: string,
  payload: StandaloneAccountImportPayload
): Promise<AdminDataImportResult> {
  const body: Record<string, unknown> = {
    data: payload.data,
    skip_default_group_bind: payload.skip_default_group_bind
  }
  if (payload.apply !== undefined) {
    body.apply = payload.apply
  }
  const { data } = await apiClient.post<AdminDataImportResult>('/account-import/data', body, {
    headers: tokenHeaders(token)
  })
  return data
}
