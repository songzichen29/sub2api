import { apiClient } from './client'

export interface QualityConfig {
  id?: string
  monitor_id?: number
  base_url?: string
  api_key_id?: number
  benchmark: 'candy' | 'pelican' | 'creative'
  model: string
  reasoning_effort: string
  service_tier: string
}
export interface QualityTarget {
  monitor_id?: number
  monitor_name?: string
  group_id: number
  group_name: string
  channel_id: number
  channel_name: string
  benchmark: string
  model: string
  reasoning_effort: string
  service_tier: string
}
export interface QualityRun {
  id: string
  plan_id: string
  batch_id?: string
  config_id?: string
  target: QualityTarget
  status: string
  verdict: string
  phase: string
  reason: string
  started_at: string
  finished_at: string | null
  cancel_requested: boolean
  stats_24h?: QualityStats
  result?: {
    answer?: string
    html?: string
    source?: string
    duration_ms: number
    input_tokens?: number
    output_tokens?: number
    reasoning_tokens?: number
    service_tier?: string
  }
}
export interface QualityInput {
  name: string
  group_id: number
  configs: QualityConfig[]
  config?: QualityConfig
  enabled: boolean
  public_visible: boolean
  interval_minutes: number
}
export interface QualityStats {
  total: number
  passed: number
  failed: number
  errors: number
  unknown: number
}
export interface QualityPlan extends Omit<QualityInput, 'config' | 'configs'> {
  id: string
  config?: QualityConfig
  configs?: QualityConfig[]
  batches?: QualityBatch[]
  active_run_id?: string
  can_edit: boolean
  next_run_at: string
  created_at: string
  target: QualityTarget
  stats: QualityStats
  stats_24h?: QualityStats
  recent: QualityRun[]
  monitor_ids?: number[] | null
}
export interface QualityBatch {
  id: string
  plan_id: string
  started_at: string
  finished_at: string
  verdict: string
  count: number
  stats?: QualityStats
}
export interface QualityOptions {
  groups: { id: number; name: string; channel_name: string }[]
  keys: { id: number; name: string; group_id: number }[]
  monitors: { id: number; name: string; group_id: number; model: string }[]
}

const root = (admin: boolean) => admin ? '/admin/model-quality' : '/model-quality'
export const list = async (admin: boolean) => (await apiClient.get<QualityPlan[]>(root(admin))).data
export const options = async () => (await apiClient.get<QualityOptions>('/admin/model-quality/options')).data
export const save = async (id: string | null, input: QualityInput) => id
  ? (await apiClient.put(`/admin/model-quality/${id}`, input)).data
  : (await apiClient.post('/admin/model-quality', input)).data
export const run = async (id: string) => (await apiClient.post<QualityRun[]>(`/admin/model-quality/${id}/run`)).data
export const remove = async (id: string) => (await apiClient.delete(`/admin/model-quality/${id}`)).data
export const cancel = async (id: string) => (await apiClient.post(`/admin/model-quality/runs/${id}/cancel`)).data
export const history = async (admin: boolean, id: string, page: number) =>
  (await apiClient.get<QualityRun[]>(`${root(admin)}/${id}/history`, { params: { page } })).data
export const detail = async (admin: boolean, id: string) =>
  (await apiClient.get<QualityRun>(`${root(admin)}/runs/${id}`)).data
export const groupHistory = async (admin: boolean, groupId: number, page: number) =>
  (await apiClient.get<QualityRun[]>(`${root(admin)}/groups/${groupId}/history`, { params: { page } })).data

export type QualityBenchmark = string
export interface QualityResults {
  batch_id?: string
  model: string
  models: string[]
  benchmark: QualityBenchmark
  benchmarks: QualityBenchmark[]
  items: QualityRun[]
  has_more: boolean
}
export interface QualityResultFilter {
  batch_id?: string
  exclude_batch_id?: string
  model?: string
  benchmark?: string
  page: number
}
export const planResults = async (admin: boolean, id: string, params: QualityResultFilter) =>
  (await apiClient.get<QualityResults>(`${root(admin)}/${id}/results`, { params })).data
export const groupResults = async (admin: boolean, groupId: number, benchmark: QualityBenchmark | '', page: number) =>
  (await apiClient.get<QualityResults>(`${root(admin)}/groups/${groupId}/results`, { params: { benchmark, page } })).data
