import apiClient from './client'
import type { QQLotteryPrize, QQLotteryState, QQLotteryPublicGroup } from './qqcommunity'

export interface UserLottery {
  enabled: boolean
  title: string
  period_start: string
  recharge_step: number
  chances_per_step: number
  prizes: QQLotteryPrize[]
  guarantee_prizes: QQLotteryPrize[]
  state: QQLotteryState
  bound_qq: string
  public_groups: QQLotteryPublicGroup[]
  pending_recharge: boolean
}
export type LotteryMembershipStatus = 'unbound' | 'member' | 'not_member' | 'unknown' | 'disabled'
export interface LotteryMembership { status: LotteryMembershipStatus; checked_at: string }
export interface UserLotteryDraw { id: number; prize_name: string; amount: number; is_guaranteed: boolean; created_at: string }
export interface UserLotteryDraws { items: UserLotteryDraw[]; total: number; page: number; page_size: number }
export const lotteryAPI = {
  overview: async () => (await apiClient.get<UserLottery>('/user/qqbot/lottery')).data,
  membership: async () => (await apiClient.post<LotteryMembership>('/user/qqbot/lottery/membership', {}, { timeout: 12000 })).data,
  draws: async (page = 1) => (await apiClient.get<UserLotteryDraws>('/user/qqbot/lottery/draws', { params: { page, page_size: 20 } })).data,
}
