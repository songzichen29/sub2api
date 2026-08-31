import { beforeEach, describe, expect, it, vi } from 'vitest'

const { post } = vi.hoisted(() => ({
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { post }
}))

import { importData } from '@/api/admin/accounts'
import type { AdminDataPayload } from '@/types'

describe('admin account import API', () => {
  beforeEach(() => {
    post.mockReset()
    post.mockResolvedValue({
      data: {
        account_created: 1,
        account_failed: 0,
        proxy_created: 0,
        proxy_reused: 0,
        proxy_failed: 0,
        errors: []
      }
    })
  })

  it('forwards template apply values to the account import endpoint', async () => {
    const data = { type: 'sub2api-data', version: 1, accounts: [], proxies: [] } as AdminDataPayload
    const apply = {
      tags: ['vip'],
      group_ids: [7],
      concurrency: 8
    }

    await importData({ data, skip_default_group_bind: true, apply })

    expect(post).toHaveBeenCalledWith('/admin/accounts/data', {
      data,
      skip_default_group_bind: true,
      apply
    })
  })
})
