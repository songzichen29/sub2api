/**
 * Redeem code API endpoints
 * Handles redeem code redemption for users
 */

import { apiClient } from './client'
import type { PaginatedResponse, RedeemCodeRequest } from '@/types'

export interface RedeemHistoryItem {
  id: number
  code: string
  type: string
  value: number
  status: string
  used_at: string
  created_at: string
  // Notes from admin for admin_balance/admin_concurrency types
  notes?: string
  // Subscription-specific fields
  group_id?: number
  validity_days?: number
  group?: {
    id: number
    name: string
  }
}

export interface AccountActivityItem extends RedeemHistoryItem {
  group_name?: string
  counterparty?: string
  amount?: string
  fee?: string
  net_amount?: string
}

/**
 * Redeem a code
 * @param code - Redeem code string
 * @returns Redemption result with updated balance or concurrency
 */
export async function redeem(code: string): Promise<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
}> {
  const payload: RedeemCodeRequest = { code }

  const { data } = await apiClient.post<{
    message: string
    type: string
    value: number
    new_balance?: number
    new_concurrency?: number
  }>('/redeem', payload)

  return data
}

/**
 * Get user's redemption history
 * @returns The requested page of redeemed codes and the total count
 */
export async function getHistory(page = 1, pageSize = 20): Promise<PaginatedResponse<RedeemHistoryItem>> {
  const { data } = await apiClient.get<PaginatedResponse<RedeemHistoryItem>>('/redeem/history', {
    params: { page, page_size: pageSize }
  })
  return data
}

/** Get the authenticated user's redemptions and completed QQ transfers. */
export async function getAccountActivity(page = 1, pageSize = 20): Promise<PaginatedResponse<AccountActivityItem>> {
  const { data } = await apiClient.get<PaginatedResponse<AccountActivityItem>>('/user/account/activity', {
    params: { page, page_size: pageSize }
  })
  return data
}

export const redeemAPI = {
  redeem,
  getHistory,
  getAccountActivity
}

export default redeemAPI
