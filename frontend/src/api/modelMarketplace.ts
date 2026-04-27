import { apiClient } from './client'
import type { MarketplaceGroupFacet, MarketplaceModelItem, MarketplaceProviderFacet } from '@/utils/modelMarketplace'

export interface ModelMarketplaceListResponse {
  items: MarketplaceModelItem[]
  provider_facets: MarketplaceProviderFacet[]
  group_facets: MarketplaceGroupFacet[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface ModelMarketplaceListParams {
  page?: number
  page_size?: number
  search?: string
  provider?: string
  group_id?: number | null
}

export async function list(
  params: ModelMarketplaceListParams,
  options?: { signal?: AbortSignal },
): Promise<ModelMarketplaceListResponse> {
  const { data } = await apiClient.get<ModelMarketplaceListResponse>('/model-marketplace', {
    params,
    signal: options?.signal,
  })
  return data
}

const modelMarketplaceAPI = { list }

export default modelMarketplaceAPI
