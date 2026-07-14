/**
 * Admin Subscriptions API endpoints
 * Handles user subscription management for administrators
 */

import { apiClient } from '../client'
import type {
  UserSubscription,
  SubscriptionProgress,
  AssignSubscriptionRequest,
  BulkAssignSubscriptionRequest,
  ExtendSubscriptionRequest,
  PaginatedResponse
} from '@/types'

/**
 * List all subscriptions with pagination
 * @param page - Page number (default: 1)
 * @param pageSize - Items per page (default: 20)
 * @param filters - Optional filters (status, user_id, group_id, sort_by, sort_order)
 * @returns Paginated list of subscriptions
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: 'active' | 'expired' | 'revoked' | 'quota_exhausted'
    user_id?: number
    group_id?: number
    platform?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: {
    signal?: AbortSignal
  }
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    '/admin/subscriptions',
    {
      params: {
        page,
        page_size: pageSize,
        ...filters
      },
      signal: options?.signal
    }
  )
  return data
}

/**
 * Get subscription by ID
 * @param id - Subscription ID
 * @returns Subscription details
 */
export async function getById(id: number): Promise<UserSubscription> {
  const { data } = await apiClient.get<UserSubscription>(`/admin/subscriptions/${id}`)
  return data
}

/**
 * Get subscription progress
 * @param id - Subscription ID
 * @returns Subscription progress with usage stats
 */
export async function getProgress(id: number): Promise<SubscriptionProgress> {
  const { data } = await apiClient.get<SubscriptionProgress>(`/admin/subscriptions/${id}/progress`)
  return data
}

/**
 * Assign subscription to user
 * @param request - Assignment request
 * @returns Created subscription
 */
export async function assign(request: AssignSubscriptionRequest): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>('/admin/subscriptions/assign', request)
  return data
}

/**
 * Bulk assign subscriptions to multiple users
 * @param request - Bulk assignment request
 * @returns Created subscriptions
 */
export async function bulkAssign(
  request: BulkAssignSubscriptionRequest
): Promise<UserSubscription[]> {
  const { data } = await apiClient.post<UserSubscription[]>(
    '/admin/subscriptions/bulk-assign',
    request
  )
  return data
}

/**
 * Adjust subscription validity
 * @param id - Subscription ID
 * @param request - Either days delta, or explicit starts_at/expires_at time range
 * @returns Updated subscription
 */
export async function extend(
  id: number,
  request: ExtendSubscriptionRequest
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/extend`,
    request
  )
  return data
}

/**
 * Revoke subscription
 * @param id - Subscription ID
 * @returns Success confirmation
 */
export async function revoke(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`/admin/subscriptions/${id}/revoke`)
  return data
}

/**
 * Reset daily, weekly, and/or monthly usage quota for a subscription
 * @param id - Subscription ID
 * @param options - Which windows to reset
 * @returns Updated subscription
 */
export async function resetQuota(
  id: number,
  options: { daily: boolean; weekly: boolean; monthly: boolean }
): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/reset-quota`,
    options
  )
  return data
}

export async function setWeekendSkip(
  id: number,
  enabled: boolean
): Promise<UserSubscription> {
  const { data } = await apiClient.put<UserSubscription>(
    `/admin/subscriptions/${id}/weekend-skip`,
    { enabled }
  )
  return data
}

export interface WeekendSkipPreview {
  subscription_id: number
  enabled: boolean
  current_expires_at: string
  preview_expires_at: string
  delta_seconds: number
  reason: string
}

export async function previewWeekendSkip(
  id: number,
  enabled: boolean
): Promise<WeekendSkipPreview> {
  const { data } = await apiClient.post<WeekendSkipPreview>(
    `/admin/subscriptions/${id}/weekend-skip/preview`,
    { enabled }
  )
  return data
}

export interface SubscriptionOrderUsageItem {
  order_id: number
  order_status: string
  order_type: string
  renewal_mode: string
  user_email: string
  plan_id?: number | null
  paid_at: string
  completed_at?: string | null
  window_start: string
  window_end: string
  subscription_days: number
  validity_unit?: string
  quota_usd?: number | null
  used_actual_cost_usd: number
  used_base_cost_usd: number
  allocated_used_usd: number
  window_subscription_used_usd: number
  window_balance_used_usd: number
  window_total_used_usd: number
  over_quota_usd?: number
  remaining_usd?: number | null
  request_count: number
  balance_request_count?: number
  input_tokens: number
  output_tokens: number
  first_usage_at?: string | null
  last_usage_at?: string | null
  exhausted_at?: string | null
  window_kind: string
  attribution: string
}

export interface SubscriptionOrderUsageResponse {
  subscription_id: number
  user_id: number
  group_id: number
  attribution: string
  orders: SubscriptionOrderUsageItem[]
  total_quota_usd?: number | null
  total_used_actual_cost: number
  total_allocated_used_usd: number
  total_window_subscription_used_usd: number
  total_window_balance_used_usd: number
  total_over_quota_usd?: number
  total_remaining_usd?: number | null
  generated_at: string
}

export async function getOrderUsage(id: number): Promise<SubscriptionOrderUsageResponse> {
  const { data } = await apiClient.get<SubscriptionOrderUsageResponse>(
    `/admin/subscriptions/${id}/order-usage`
  )
  return data
}

export async function resetWeekendSkipUserChange(id: number): Promise<UserSubscription> {
  const { data } = await apiClient.post<UserSubscription>(
    `/admin/subscriptions/${id}/weekend-skip/reset-user-change`
  )
  return data
}

/**
 * List subscriptions by group
 * @param groupId - Group ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of subscriptions in the group
 */
export async function listByGroup(
  groupId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/groups/${groupId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

/**
 * List subscriptions by user
 * @param userId - User ID
 * @param page - Page number
 * @param pageSize - Items per page
 * @returns Paginated list of user's subscriptions
 */
export async function listByUser(
  userId: number,
  page: number = 1,
  pageSize: number = 20
): Promise<PaginatedResponse<UserSubscription>> {
  const { data } = await apiClient.get<PaginatedResponse<UserSubscription>>(
    `/admin/users/${userId}/subscriptions`,
    {
      params: { page, page_size: pageSize }
    }
  )
  return data
}

export const subscriptionsAPI = {
  list,
  getById,
  getProgress,
  assign,
  bulkAssign,
  extend,
  revoke,
  resetQuota,
  getOrderUsage,
  previewWeekendSkip,
  setWeekendSkip,
  resetWeekendSkipUserChange,
  listByGroup,
  listByUser
}

export default subscriptionsAPI
