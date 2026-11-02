import { apiClient } from './client'

export interface QQBotConfig {
  version: number
  enabled: boolean
  bot_qq: string
  allowed_groups: string[]
  access_token: string
  access_token_set: boolean
  clear_access_token?: boolean
  checkin_reward: number
  checkin_reward_max: number
  chat_enabled: boolean
  api_key: string
  api_key_set: boolean
  clear_api_key?: boolean
  chat_model: string
  system_prompt: string
  max_output_tokens: number
  history_messages: number
  daily_chat_limit: number
  total_daily_chat_limit: number
  cooldown_seconds: number
}
export interface QQBinding { user_id: number; qq: string; group_id: string; created_at: string }
export interface QQBindingStatus {
  enabled: boolean
  bot_qq: string
  binding: QQBinding | null
  pending_qq: string
  expires_at?: string
}
export interface QQConnection { connected: boolean; connections: number; connected_since: string | null; scope: string }
export interface QQReward { id: string; user_id: number; qq: string; group_id: string; day: string; amount: number; balance_after: number; created_at: string }
export interface QQEvent { id: string; qq: string; group_id: string; command: string; status: string; result: string; created_at: string }
export type QQRecord = QQBinding | QQReward | QQEvent
export type QQRecordKind = 'bindings' | 'rewards' | 'events'
export interface QQPage { items: QQRecord[]; total: number; page: number; page_size: number }

const admin = '/admin/qq-bot'
const user = '/user/qq-bot'
export const qqbotApi = {
  config: async () => (await apiClient.get<QQBotConfig>(`${admin}/config`)).data,
  save: async (config: QQBotConfig) => (await apiClient.put<QQBotConfig>(`${admin}/config`, config)).data,
  connection: async () => (await apiClient.get<QQConnection>(`${admin}/connection`)).data,
  test: async (config: QQBotConfig) => (await apiClient.post<{ reply: string }>(`${admin}/test-model`, config, { timeout: 120000 })).data,
  records: async (kind: QQRecordKind, page: number) => (await apiClient.get<QQPage>(`${admin}/records/${kind}`, { params: { page, page_size: 20 } })).data,
  adminUnbind: (id: number) => apiClient.delete(`${admin}/bindings/${id}`),
  binding: async () => (await apiClient.get<QQBindingStatus>(user)).data,
  code: async () => (await apiClient.post<{ code: string; expires_at: string }>(`${user}/code`)).data,
  confirm: (qq: string) => apiClient.post(`${user}/confirm`, { qq }),
  unbind: () => apiClient.delete(user),
}
