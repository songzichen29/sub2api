import { apiClient } from './client'

export interface CustomPageStatus {
  available: boolean
  reason?: string
  status_code?: number
}

export async function getCustomPageStatus(id: string): Promise<CustomPageStatus> {
  const { data } = await apiClient.get<CustomPageStatus>(
    `/settings/custom-pages/${encodeURIComponent(id)}/status`,
  )
  return data
}
