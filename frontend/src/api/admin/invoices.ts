import { apiClient } from '../client'
import type { BasePaginationResponse } from '@/types'
import type { InvoiceApplication, InvoiceApplicationStatus } from '@/api/invoice'

export interface InvoiceSettings {
  min_amount: number
}

export interface InvoiceApplicationFilters {
  page?: number
  page_size?: number
  user_id?: number
  status?: InvoiceApplicationStatus
  keyword?: string
  start_date?: string
  end_date?: string
}

export interface UpdateInvoiceApplicationInput {
  status: Exclude<InvoiceApplicationStatus, 'PENDING'>
  rejection_reason?: string
  admin_note?: string
  invoice_number?: string
}

export const adminInvoiceAPI = {
  getSettings() {
    return apiClient.get<InvoiceSettings>('/admin/payment/invoices/config')
  },

  updateSettings(minAmount: number) {
    return apiClient.put<InvoiceSettings>('/admin/payment/invoices/config', { min_amount: minAmount })
  },

  listApplications(params?: InvoiceApplicationFilters) {
    return apiClient.get<BasePaginationResponse<InvoiceApplication>>('/admin/payment/invoices', { params })
  },

  getApplication(id: number) {
    return apiClient.get<InvoiceApplication>(`/admin/payment/invoices/${id}`)
  },

  updateApplication(id: number, data: UpdateInvoiceApplicationInput) {
    return apiClient.put<InvoiceApplication>(`/admin/payment/invoices/${id}`, data)
  },

  exportApplications(params?: Omit<InvoiceApplicationFilters, 'page' | 'page_size'>) {
    return apiClient.get('/admin/payment/invoices/export', { params, responseType: 'blob' })
  },
}
