import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import DashboardView from '../DashboardView.vue'

const {
  refreshUser,
  getDashboardStats,
  getDashboardTrend,
  getDashboardModels,
  getByDateRange,
  getMyPlatformQuotas,
} = vi.hoisted(() => ({
  refreshUser: vi.fn(),
  getDashboardStats: vi.fn(),
  getDashboardTrend: vi.fn(),
  getDashboardModels: vi.fn(),
  getByDateRange: vi.fn(),
  getMyPlatformQuotas: vi.fn(),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { balance: 0 },
    isSimpleMode: false,
    refreshUser,
  }),
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats,
    getDashboardTrend,
    getDashboardModels,
    getByDateRange,
  },
}))

vi.mock('@/api/user', () => ({ getMyPlatformQuotas }))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('user DashboardView', () => {
  beforeEach(() => {
    refreshUser.mockReset().mockResolvedValue(undefined)
    getDashboardStats.mockReset().mockResolvedValue({})
    getDashboardTrend.mockReset().mockResolvedValue({ trend: [] })
    getDashboardModels.mockReset().mockResolvedValue({ models: [] })
    getByDateRange.mockReset().mockResolvedValue({ items: [] })
    getMyPlatformQuotas.mockReset().mockResolvedValue({ platform_quotas: [] })
  })

  it('loads today with hourly granularity by default', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          UserDashboardStats: true,
          UserDashboardCharts: true,
          UserDashboardRecentUsage: true,
          UserDashboardQuickActions: true,
        },
      },
    })

    await flushPromises()
    const today = formatLocalDate(new Date())

    expect(getDashboardTrend).toHaveBeenCalledWith({
      start_date: today,
      end_date: today,
      granularity: 'hour',
    })
    expect(getDashboardModels).toHaveBeenCalledWith({ start_date: today, end_date: today })
    expect(getByDateRange).toHaveBeenCalledWith(today, today)
  })
})
