import { describe, expect, it } from 'vitest'
import { createI18n } from 'vue-i18n'
import zh from '@/i18n/locales/zh/dashboard'
import en from '@/i18n/locales/en/dashboard'
import { formatAccountActivityNote, parseQQCampaignNote, parseQQRewardNote } from '../accountActivity'

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

  it.each(['zh', 'en'] as const)('formats historical reward receipts using the actual %s messages', locale => {
    const { t } = createI18n({ legacy: false, locale, messages: { zh, en } }).global
    for (const kind of ['growth', 'lottery', 'invite', 'checkin', 'streak', 'ranking', 'redemption']) {
      expect(formatAccountActivityNote(`QQ ${kind} reward #76`, t)).toBe(
        locale === 'zh' ? '奖励记录 #76' : 'Reward record #76'
      )
    }
    expect(formatAccountActivityNote('Website checkin reward #88', t)).toBe(
      locale === 'zh' ? '奖励记录 #88' : 'Reward record #88'
    )
    expect(formatAccountActivityNote('QQ lottery reward #7: 保底奖励', t)).toBe(
      locale === 'zh' ? '奖励记录 #7：保底奖励' : 'Reward record #7: 保底奖励'
    )
    expect(formatAccountActivityNote('QQ campaign 1c712861ba2a48048bde15d9281fd793', t)).toBe(
      locale === 'zh' ? 'QQ 群领码活动兑换' : 'QQ group campaign redemption'
    )
    expect(formatAccountActivityNote('Manual credit: QQ growth reward #76', t)).toBe('Manual credit: QQ growth reward #76')
    expect(formatAccountActivityNote('管理员补款', t)).toBe('管理员补款')
  })
})
