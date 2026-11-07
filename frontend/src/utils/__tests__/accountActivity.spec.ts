import { describe, expect, it } from 'vitest'
import { parseQQCampaignNote, parseQQRewardNote } from '../accountActivity'

describe('QQ reward receipt presentation', () => {
  it('identifies known machine-written reward notes', () => {
    expect(parseQQRewardNote('QQ growth reward #3')).toEqual({ kind: 'growth', number: '3', detail: '' })
    expect(parseQQRewardNote('QQ lottery reward #7: 保底奖励')).toEqual({ kind: 'lottery', number: '7', detail: '保底奖励' })
    expect(parseQQRewardNote('Website checkin reward #8')).toEqual({ kind: 'website_checkin', number: '8', detail: '' })
    expect(parseQQCampaignNote('QQ campaign 1c712861ba2a48048bde15d9281fd793')).toBe('1c712861ba2a48048bde15d9281fd793')
  })

  it('leaves operator and malformed notes untouched', () => {
    expect(parseQQRewardNote('管理员补款 QQ growth reward #3')).toBeNull()
    expect(parseQQRewardNote('QQ growth reward #3 私人备注')).toBeNull()
    expect(parseQQRewardNote('自定义备注')).toBeNull()
    expect(parseQQCampaignNote('管理员备注 QQ campaign 1c712861ba2a48048bde15d9281fd793')).toBeNull()
  })
})
