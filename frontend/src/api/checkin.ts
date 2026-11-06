import apiClient from './client'
import type { QQGrowthReward } from './qqcommunity'

export interface CheckinDay { date: string; amount: number; source: 'website' | 'qq' }
export interface CheckinBonus { reward: QQGrowthReward; status: string }
export interface UserCheckin {
  today: string; month: string; server_time: string; next_reset_at: string
  enabled: boolean; eligible: boolean; checked_in: boolean; can_checkin: boolean
  today_amount: number; today_source: string; min_amount: number; max_amount: number
  total_days: number; streak_days: number; total_amount: number
  streak_interval: number; streak_reward: QQGrowthReward | null; today_bonus: CheckinBonus | null
  min_recharge_total: number; min_recharge_30d: number; recharge_window_days: number
  days: CheckinDay[]
}
export interface CheckinResult extends CheckinDay { already_checked_in: boolean; bonus: CheckinBonus | null }

export const checkinAPI = {
  overview: async (month = '') => (await apiClient.get<UserCheckin>('/user/checkin', { params: month ? { month } : undefined })).data,
  claim: async (day: string) => (await apiClient.post<CheckinResult>('/user/checkin', { day }, { timeout: 20000 })).data,
}
