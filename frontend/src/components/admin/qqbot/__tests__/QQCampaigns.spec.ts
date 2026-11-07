import { mount, flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import zh from '@/i18n/locales/zh'
import en from '@/i18n/locales/en'
import QQBotCodeCampaigns from '../QQBotCodeCampaigns.vue'
import QQActivityStats from '../QQActivityStats.vue'

const api = vi.hoisted(() => ({ codeCampaigns: vi.fn(), config: vi.fn(), codeClaims: vi.fn(), saveCodeCampaign: vi.fn(), resendCodeClaim: vi.fn(), disableCodeClaim: vi.fn(), activityStats: vi.fn() }))
vi.mock('@/api/qqcommunity', () => ({ qqbotAPI: api }))
vi.mock('@/api/admin/groups', () => ({ getAll: async () => [{ id: 7, name: 'Subscription', status: 'active', subscription_type: 'subscription' }] }))
const campaign = { id: '123456789012345678901234', name: 'Test campaign', type: 'lottery_chances', value: 3, subscription_group_id: null, validity_days: 30, enabled: true, groups: ['123'], claim_mode: 'once', stock_limit: 100, code_valid_hours: 24, starts_at: null, ends_at: null, version: 1, claimed: 2, sent: 1, redeemed: 0 }
const globalOptions = (locale = 'zh') => ({ plugins: [createI18n({ legacy: false, locale, messages: { zh, en } })] })
describe('QQ code campaigns and activity statistics', () => {
 beforeEach(() => { vi.resetAllMocks(); api.codeCampaigns.mockResolvedValue([campaign]); api.config.mockResolvedValue({ app_id: 'bot', allowed_groups: ['123'], group_policies: [] }); api.codeClaims.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }) })
 it.each(['zh', 'en'])('%s shows the fixed command, counts and locked reward settings', async locale => {
  const w = mount(QQBotCodeCampaigns, { global: globalOptions(locale) }); await flushPromises()
  expect(w.text()).toContain('/领码'); expect(w.text()).toContain('2 / 100'); expect(w.text()).not.toContain('qqbot.codeCampaigns.')
  await w.get('tbody button').trigger('click'); await flushPromises()
  const f = w.findAll('form')[0]!
  expect(f.findAll('select')[0]!.attributes('disabled')).toBeDefined()
  expect(f.findAll('select')[1]!.attributes('disabled')).toBeDefined()
  expect(f.get('input[type="number"]').attributes('disabled')).toBeDefined()
  w.unmount()
 })
 it('creates a paused campaign with a stable ID and converts Beijing time explicitly', async () => {
  const w = mount(QQBotCodeCampaigns, { global: globalOptions() }); await flushPromises()
  await w.findAll('button').find(b => b.text() === '新建活动')!.trigger('click')
  const f = w.findAll('form')[0]!
  await f.get('input[maxlength="60"]').setValue('Launch')
  await f.findAll('input[type="datetime-local"]')[0]!.setValue('2026-11-08T12:30')
  api.saveCodeCampaign.mockRejectedValueOnce(new Error('uncertain network result'))
  await f.trigger('submit'); await flushPromises()
  const first = api.saveCodeCampaign.mock.calls[0]![0]
  expect(first).toMatchObject({ name: 'Launch', enabled: false, version: 0, type: 'lottery_chances', starts_at: '2026-11-08T04:30:00.000Z' })
  expect(first.id).toMatch(/^[a-f0-9]{32}$/)
  await f.trigger('submit'); await flushPromises()
  expect(api.saveCodeCampaign.mock.calls[1]![0].id).toBe(first.id)
  expect(w.text()).toContain('/领码'); w.unmount()
 })
 it.each([
  [{ starts_at: '2099-01-01T00:00:00Z' }, '未开始'],
  [{ ends_at: '2000-01-01T00:00:00Z' }, '已结束'],
  [{ claimed: 100 }, '已领完'],
  [{ groups: [] }, '未配置开放群'],
  [{ enabled: false }, '暂停领取'],
 ])('shows actual claim availability for %j', async (changes, label) => {
  api.codeCampaigns.mockResolvedValue([{ ...campaign, ...changes }])
  const w = mount(QQBotCodeCampaigns, { global: globalOptions() }); await flushPromises()
  expect(w.get('tbody tr').text()).toContain(label)
  expect(w.get('tbody tr').text()).not.toContain('开放领取')
  w.unmount()
 })
 it('does not turn statistics failures into zero and filters by clicking a metric', async () => {
  api.activityStats.mockRejectedValueOnce(new Error('database unavailable'))
  const w = mount(QQActivityStats, { global: globalOptions() }); await flushPromises()
  expect(w.get('[role="alert"]').text()).toContain('database unavailable'); expect(w.find('table').exists()).toBe(false)
  api.activityStats.mockResolvedValue({ date: '2026-11-07', refreshed_at: '2026-11-07T00:00:00Z', items: [], total: 0, page: 1, page_size: 20, summary: { checkin_users: 12, chance_users: 6, remaining_chances: 30, draw_users: 3, draw_count: 8, checkin_amount: 1, draw_amount: 2 } })
  await w.findAll('button').find(b => b.text() === '刷新')!.trigger('click'); await flushPromises()
  const metric = w.findAll('button').find(b => b.text().includes('当前有次数人数'))!
  expect(metric.text()).toContain('6'); await metric.trigger('click'); await flushPromises()
  expect(api.activityStats).toHaveBeenLastCalledWith(expect.objectContaining({ filter: 'chances', page: 1 }))
  w.unmount()
 })
})
