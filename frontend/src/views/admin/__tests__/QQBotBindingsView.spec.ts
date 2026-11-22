import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import { createI18n } from 'vue-i18n'
import QQBotBindingsView from '../QQBotBindingsView.vue'
import type { QQTransferHistory } from '@/api/qqcommunity'
import zh from '@/i18n/locales/zh'

const transferHistory = vi.hoisted(() => vi.fn())
vi.mock('@/api/qqcommunity', () => ({ qqbotAPI: { transferHistory } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/admin/QQBotBindings.vue', () => ({ default: { template: '<section>成员列表</section>' } }))
vi.mock('@/components/admin/qqbot/QQBotTransferSettings.vue', () => ({ default: { template: '<section>转账设置</section>' } }))
enableAutoUnmount(afterEach)
beforeEach(() => vi.resetAllMocks())

function pendingHistory() {
  let resolve!: (value: QQTransferHistory) => void
  let reject!: (error: Error) => void
  const promise = new Promise<QQTransferHistory>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

function historyPage(email: string): QQTransferHistory {
  return {
    items: [{
      id: 1,
      from_user_id: 1,
      to_user_id: 2,
      from_email: email,
      to_email: 'recipient@example.com',
      amount: '10',
      fee: '1',
      net_amount: '9',
      created_at: '2026-11-22T00:00:00Z',
    }],
    total: 1,
    page: 1,
    page_size: 20,
  }
}

async function setup() {
  const router = createRouter({ history: createMemoryHistory(), routes: [
    { path: '/admin/qqbot/bindings', component: QQBotBindingsView },
  ] })
  await router.push('/admin/qqbot/bindings?tab=history')
  const wrapper = mount(QQBotBindingsView, {
    global: { plugins: [router, createI18n({ legacy: false, locale: 'zh', messages: { zh } })] },
  })
  await flushPromises()
  return { wrapper, router }
}

describe('QQ 转账记录的并发加载', () => {
  it('旧响应晚于新响应返回时保留当前搜索结果', async () => {
    const older = pendingHistory()
    const newer = pendingHistory()
    transferHistory.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const { wrapper } = await setup()
    await wrapper.get('input').setValue('newer')
    await wrapper.get('form').trigger('submit')
    expect(transferHistory).toHaveBeenNthCalledWith(2, 'newer', 1)
    newer.resolve(historyPage('newer@example.com'))
    await flushPromises()
    expect(wrapper.get('article').text()).toContain('newer@example.com')
    older.resolve(historyPage('older@example.com'))
    await flushPromises()
    expect(wrapper.get('article').text()).toContain('newer@example.com')
    expect(wrapper.text()).not.toContain('older@example.com')
  })

  it('旧请求失败时保留当前请求的加载状态', async () => {
    const older = pendingHistory()
    const newer = pendingHistory()
    transferHistory.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const { wrapper } = await setup()
    await wrapper.get('form').trigger('submit')
    older.reject(new Error('旧请求失败'))
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toContain('加载中')
    newer.resolve(historyPage('newer@example.com'))
    await flushPromises()
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(wrapper.get('article').text()).toContain('newer@example.com')
  })

  it('当前请求失败后，旧请求成功不会替换错误状态和记录', async () => {
    const older = pendingHistory()
    const newer = pendingHistory()
    transferHistory.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const { wrapper } = await setup()
    await wrapper.get('form').trigger('submit')
    newer.reject(new Error('当前请求失败'))
    await flushPromises()
    older.resolve(historyPage('older@example.com'))
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('当前请求失败')
    expect(wrapper.find('article').exists()).toBe(false)
  })

  it('离开记录页签时作废请求，重新进入时使用新结果', async () => {
    const older = pendingHistory()
    const newer = pendingHistory()
    transferHistory.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise)
    const { wrapper, router } = await setup()
    await router.push('/admin/qqbot/bindings?tab=members')
    older.resolve(historyPage('older@example.com'))
    await flushPromises()
    await router.push('/admin/qqbot/bindings?tab=history')
    await flushPromises()
    expect(wrapper.find('article').exists()).toBe(false)
    expect(wrapper.get('[role="status"]').text()).toContain('加载中')
    newer.resolve(historyPage('newer@example.com'))
    await flushPromises()
    expect(wrapper.get('article').text()).toContain('newer@example.com')
  })
})
