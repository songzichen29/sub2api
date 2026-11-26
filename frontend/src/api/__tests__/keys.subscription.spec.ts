import { beforeEach, describe, expect, it, vi } from 'vitest'
import { create } from '../keys'
import { updateApiKeyGroup } from '../admin/apiKeys'

const { post, put } = vi.hoisted(() => ({ post: vi.fn(), put: vi.fn() }))
vi.mock('../client', () => ({ apiClient: { post, put } }))

beforeEach(() => {
  vi.clearAllMocks()
  post.mockResolvedValue({ data: { id: 1 } })
  put.mockResolvedValue({ data: { api_key: { id: 1 } } })
})

describe('subscription binding transport', () => {
  it('includes the selected instance when creating a key', async () => {
    await create('New key', 7, undefined, undefined, undefined, undefined, undefined, undefined, 102)
    expect(post).toHaveBeenCalledWith('/keys', { name: 'New key', group_id: 7, subscription_id: 102 })
  })

  it('preserves the original create call without a subscription argument', async () => {
    await create('Balance key', 8)
    expect(post).toHaveBeenCalledWith('/keys', { name: 'Balance key', group_id: 8 })
  })

  it('passes the managed user instance and preserves unbinding for administrators', async () => {
    await updateApiKeyGroup(1, 7, 102)
    expect(put).toHaveBeenLastCalledWith('/admin/api-keys/1', { group_id: 7, subscription_id: 102 })
    await updateApiKeyGroup(1, null)
    expect(put).toHaveBeenLastCalledWith('/admin/api-keys/1', { group_id: 0 })
  })
})
