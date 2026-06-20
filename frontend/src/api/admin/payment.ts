/**
 * Admin Payment API endpoints
 * Handles payment management operations for administrators
 */

import { apiClient } from '../client'
import type {
  DashboardStats,
  DailyPaymentStat,
  PaymentOrder,
  PaymentChannel,
  SubscriptionPlan,
  ProviderInstance
} from '@/types/payment'
import type { DiscountRule } from '@/types/payment'
import type { BasePaginationResponse } from '@/types'
import type { PaymentPaidUserRateBackfillStatus } from './settings'

export type PaymentCouponType = 'fixed' | 'percent'
export type PaymentCouponScope = 'all' | 'balance' | 'subscription'
export type PaymentCouponStatus = 'active' | 'disabled' | 'archived'
export type PaymentCouponUsageStatus = 'used' | 'refunded'

export interface PaymentCoupon {
  id: number
  code: string
  type: PaymentCouponType
  value: number
  min_amount: number
  max_discount: number
  scope: PaymentCouponScope
  max_uses: number
  used_count: number
  per_user_limit: number
  status: PaymentCouponStatus
  starts_at?: string
  expires_at?: string
  notes: string
  created_at: string
  updated_at: string
}

export interface PaymentCouponUsage {
  id: number
  coupon_id: number
  user_id: number
  order_id: number
  discount_amount: number
  used_at: string
  status: PaymentCouponUsageStatus
}

export interface CreatePaymentCouponRequest {
  code?: string
  type: PaymentCouponType
  value: number
  min_amount?: number
  max_discount?: number
  scope?: PaymentCouponScope
  max_uses?: number
  per_user_limit?: number
  starts_at?: number
  expires_at?: number
  notes?: string
}

export interface UpdatePaymentCouponRequest {
  code?: string
  type?: PaymentCouponType
  value?: number
  min_amount?: number
  max_discount?: number
  scope?: PaymentCouponScope
  max_uses?: number
  per_user_limit?: number
  status?: PaymentCouponStatus
  starts_at?: number
  expires_at?: number
  notes?: string
}

/** Admin-facing payment config returned by GET /admin/payment/config */
export interface AdminPaymentConfig {
  enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  discount_rules?: DiscountRule[]
  paid_user_rate_enabled: boolean
  paid_user_rate_rules: Array<{ group_id: number; rate_multiplier: number; assigned_users?: number }>
  paid_user_rate_backfill: PaymentPaidUserRateBackfillStatus
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string
}

/** Fields accepted by PUT /admin/payment/config (all optional via pointer semantics) */
export interface UpdatePaymentConfigRequest {
  enabled?: boolean
  min_amount?: number
  max_amount?: number
  daily_limit?: number
  order_timeout_minutes?: number
  max_pending_orders?: number
  enabled_payment_types?: string[]
  balance_disabled?: boolean
  balance_recharge_multiplier?: number
  discount_rules?: DiscountRule[]
  paid_user_rate_enabled?: boolean
  paid_user_rate_rules?: Array<{ group_id: number; rate_multiplier: number }>
  load_balance_strategy?: string
  product_name_prefix?: string
  product_name_suffix?: string
  help_image_url?: string
  help_text?: string
}

export const adminPaymentAPI = {
  // ==================== Config ====================

  /** Get payment configuration (admin view) */
  getConfig() {
    return apiClient.get<AdminPaymentConfig>('/admin/payment/config')
  },

  /** Update payment configuration */
  updateConfig(data: UpdatePaymentConfigRequest) {
    return apiClient.put<AdminPaymentConfig>('/admin/payment/config', data)
  },

  // ==================== Coupons ====================

  /** Get payment coupons (paginated) */
  listCoupons(params?: {
    page?: number
    page_size?: number
    status?: string
    search?: string
  }) {
    return apiClient.get<BasePaginationResponse<PaymentCoupon>>('/admin/payment/coupons', { params })
  },

  /** Get one payment coupon */
  getCoupon(id: number) {
    return apiClient.get<PaymentCoupon>(`/admin/payment/coupons/${id}`)
  },

  /** Create payment coupon */
  createCoupon(data: CreatePaymentCouponRequest) {
    return apiClient.post<PaymentCoupon>('/admin/payment/coupons', data)
  },

  /** Update payment coupon */
  updateCoupon(id: number, data: UpdatePaymentCouponRequest) {
    return apiClient.put<PaymentCoupon>(`/admin/payment/coupons/${id}`, data)
  },

  /** Archive payment coupon */
  deleteCoupon(id: number) {
    return apiClient.delete(`/admin/payment/coupons/${id}`)
  },

  /** Get coupon usage records */
  listCouponUsages(id: number, params?: { page?: number; page_size?: number }) {
    return apiClient.get<BasePaginationResponse<PaymentCouponUsage>>(`/admin/payment/coupons/${id}/usages`, { params })
  },

  // ==================== Dashboard ====================

  /** Get payment dashboard statistics */
  getDashboard(days?: number) {
    return apiClient.get<DashboardStats>('/admin/payment/dashboard', {
      params: days ? { days } : undefined
    })
  },

  /** Get payment daily statistics for a fixed date range */
  getDailyStats(params: { start: string; end: string }) {
    return apiClient.get<DailyPaymentStat[]>('/admin/payment/daily-stats', { params })
  },

  // ==================== Orders ====================

  /** Get all orders (paginated, with filters) */
  getOrders(params?: {
    page?: number
    page_size?: number
    status?: string
    payment_type?: string
    user_id?: number
    keyword?: string
    start_date?: string
    end_date?: string
    order_type?: string
  }) {
    return apiClient.get<BasePaginationResponse<PaymentOrder>>('/admin/payment/orders', { params })
  },

  /** Get a specific order by ID */
  getOrder(id: number) {
    return apiClient.get<PaymentOrder>(`/admin/payment/orders/${id}`)
  },

  /** Cancel an order (admin) */
  cancelOrder(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/cancel`)
  },

  /** Retry recharge for a failed order */
  retryRecharge(id: number) {
    return apiClient.post(`/admin/payment/orders/${id}/retry`)
  },

  /** Preview refund amount */
  previewRefund(id: number, data: { amount?: number; refund_mode?: 'full' | 'proportional' }) {
    return apiClient.post<{ refund_amount: number; max_refundable_amount: number; calculated_automatically: boolean }>(`/admin/payment/orders/${id}/refund-preview`, data)
  },

  /** Process a refund */
  refundOrder(id: number, data: { amount: number; reason: string; deduct_balance?: boolean; force?: boolean; refund_mode?: 'full' | 'proportional' }) {
    return apiClient.post(`/admin/payment/orders/${id}/refund`, data)
  },

  // ==================== Channels ====================

  /** Get all payment channels */
  getChannels() {
    return apiClient.get<PaymentChannel[]>('/admin/payment/channels')
  },

  /** Create a payment channel */
  createChannel(data: Partial<PaymentChannel>) {
    return apiClient.post<PaymentChannel>('/admin/payment/channels', data)
  },

  /** Update a payment channel */
  updateChannel(id: number, data: Partial<PaymentChannel>) {
    return apiClient.put<PaymentChannel>(`/admin/payment/channels/${id}`, data)
  },

  /** Delete a payment channel */
  deleteChannel(id: number) {
    return apiClient.delete(`/admin/payment/channels/${id}`)
  },

  // ==================== Subscription Plans ====================

  /** Get all subscription plans */
  getPlans() {
    return apiClient.get<SubscriptionPlan[]>('/admin/payment/plans')
  },

  /** Create a subscription plan */
  createPlan(data: Record<string, unknown>) {
    return apiClient.post<SubscriptionPlan>('/admin/payment/plans', data)
  },

  /** Update a subscription plan */
  updatePlan(id: number, data: Record<string, unknown>) {
    return apiClient.put<SubscriptionPlan>(`/admin/payment/plans/${id}`, data)
  },

  /** Delete a subscription plan */
  deletePlan(id: number) {
    return apiClient.delete(`/admin/payment/plans/${id}`)
  },

  // ==================== Provider Instances ====================

  /** Get all provider instances */
  getProviders() {
    return apiClient.get<ProviderInstance[]>('/admin/payment/providers')
  },

  /** Create a provider instance */
  createProvider(data: Partial<ProviderInstance>) {
    return apiClient.post<ProviderInstance>('/admin/payment/providers', data)
  },

  /** Update a provider instance */
  updateProvider(id: number, data: Partial<ProviderInstance>) {
    return apiClient.put<ProviderInstance>(`/admin/payment/providers/${id}`, data)
  },

  /** Delete a provider instance */
  deleteProvider(id: number) {
    return apiClient.delete(`/admin/payment/providers/${id}`)
  }
}

export default adminPaymentAPI
