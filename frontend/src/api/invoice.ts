import { apiClient } from './client'
import type { BasePaginationResponse } from '@/types'

export type InvoiceApplicationStatus = 'PENDING' | 'PROCESSING' | 'INVOICED' | 'REJECTED'
export type InvoiceHeaderType = 'personal' | 'company'

export interface InvoiceHeader {
  id: number
  title_type: InvoiceHeaderType
  title: string
  tax_number: string
  email: string
  phone: string
  address?: string
  is_default: boolean
  created_at: string
  updated_at: string
}

export interface InvoiceHeaderInput {
  title_type: InvoiceHeaderType
  title: string
  tax_number: string
  email: string
  phone: string
  address: string
  is_default: boolean
}

export interface InvoiceEligibleOrder {
  id: number
  order_no: string
  order_type: string
  amount: number
  paid_at?: string
  completed_at?: string
}

export interface InvoiceApplicationOrder {
  order_id: number
  order_no: string
  order_type: string
  amount: number
  paid_at?: string
  created_at: string
}

export interface InvoiceApplication {
  id: number
  application_no: string
  user_id: number
  user_email?: string
  user_name?: string
  status: InvoiceApplicationStatus
  invoice_type: 'ordinary'
  header_type: InvoiceHeaderType
  header_title: string
  header_tax_number: string
  header_email: string
  header_phone: string
  header_address?: string
  total_amount: number
  handled_by?: number
  rejection_reason?: string
  admin_note?: string
  invoice_number: string
  processed_at?: string
  invoiced_at?: string
  created_at: string
  updated_at: string
  orders: InvoiceApplicationOrder[]
}

export const invoiceAPI = {
  getApplicationData() {
    return apiClient.get<{ min_amount: number; orders: InvoiceEligibleOrder[] }>('/payment/invoices/eligible-orders')
  },

  listHeaders() {
    return apiClient.get<InvoiceHeader[]>('/payment/invoice-headers')
  },

  createHeader(data: InvoiceHeaderInput) {
    return apiClient.post<InvoiceHeader>('/payment/invoice-headers', data)
  },

  updateHeader(id: number, data: InvoiceHeaderInput) {
    return apiClient.put<InvoiceHeader>(`/payment/invoice-headers/${id}`, data)
  },

  deleteHeader(id: number) {
    return apiClient.delete(`/payment/invoice-headers/${id}`)
  },

  createApplication(data: { order_ids: number[]; header_id: number }) {
    return apiClient.post<InvoiceApplication>('/payment/invoices', data)
  },

  listApplications(params?: { page?: number; page_size?: number; status?: InvoiceApplicationStatus }) {
    return apiClient.get<BasePaginationResponse<InvoiceApplication>>('/payment/invoices', { params })
  },

  getApplication(id: number) {
    return apiClient.get<InvoiceApplication>(`/payment/invoices/${id}`)
  },
}
