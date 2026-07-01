import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import { affiliatesAPI } from '@/api/admin/affiliates'

describe('admin affiliates api', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({ data: { items: [], total: 0 } })
  })

  it('uses shared apiClient for invite records so auth interceptors run', async () => {
    await affiliatesAPI.listInviteRecords({
      page: 1,
      page_size: 20,
      search: '1410134718@qq.com',
      sort_by: 'created_at',
      sort_order: 'desc',
      timezone: 'America/Los_Angeles',
    })

    expect(get).toHaveBeenCalledWith('/admin/affiliates/invites', {
      params: {
        page: 1,
        page_size: 20,
        search: '1410134718@qq.com',
        start_at: undefined,
        end_at: undefined,
        sort_by: 'created_at',
        sort_order: 'desc',
        timezone: 'America/Los_Angeles',
      },
    })
  })

  it('uses shared apiClient for rebate records so auth interceptors run', async () => {
    await affiliatesAPI.listRebateRecords({
      page: 1,
      page_size: 20,
      search: '1410134718@qq.com',
      sort_by: 'created_at',
      sort_order: 'desc',
      timezone: 'America/Los_Angeles',
    })

    expect(get).toHaveBeenCalledWith('/admin/affiliates/rebates', {
      params: {
        page: 1,
        page_size: 20,
        search: '1410134718@qq.com',
        start_at: undefined,
        end_at: undefined,
        sort_by: 'created_at',
        sort_order: 'desc',
        timezone: 'America/Los_Angeles',
      },
    })
  })
})
