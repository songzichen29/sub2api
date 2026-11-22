import { describe, expect, it, vi } from 'vitest'

const authStore = vi.hoisted(() => ({
  checkAuth: vi.fn(),
  isAuthenticated: false,
  isAdmin: false,
  isSimpleMode: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'Sub2API',
  backendModeEnabled: false,
  cachedPublicSettings: null as null | Record<string, unknown>,
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    customMenuItems: [],
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

describe('router WeChat OAuth route', () => {
  it('registers the WeChat callback route as a public route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'WeChatOAuthCallback')

    expect(route?.path).toBe('/auth/wechat/callback')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.title).toBe('WeChat OAuth Callback')
  })

  it('registers the WeChat payment callback route as a public route', async () => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === 'WeChatPaymentOAuthCallback')

    expect(route?.path).toBe('/auth/wechat/payment/callback')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.title).toBe('WeChat Payment Callback')
  })
})

describe('router invoice routes', () => {
  it.each([
    ['InvoiceList', '/invoices'],
    ['AdminInvoices', '/admin/orders/invoices'],
  ])('keeps %s accessible independently of the payment switch', async (name, path) => {
    const { default: router } = await import('@/router')
    const route = router.getRoutes().find((record) => record.name === name)

    expect(route?.path).toBe(path)
    expect(route?.meta.requiresPayment).not.toBe(true)
    expect(route?.meta.requiresUserInvoice === true).toBe(name === 'InvoiceList')
  })
})

describe('补齐已有页面入口', () => {
  it('为未知地址注册公开的 404 页', async () => {
    const { default: router } = await import('@/router')
    const route = router.resolve('/missing-page')
    expect(route.name).toBe('NotFound')
    expect(route.meta.requiresAuth).toBe(false)
  })

  it.each([
    ['/auth/oauth/callback', 'OAuthCallback'],
    ['/auth/callback', 'OAuthCallback'],
    ['/auth/dingtalk/callback', 'DingTalkOAuthCallback'],
    ['/auth/dingtalk/email-completion', 'DingTalkEmailCompletion'],
    ['/payment/airwallex', 'AirwallexPayment'],
  ])('%s 解析到对应的公开页面', async (path, name) => {
    const { default: router } = await import('@/router')
    const route = router.resolve(path)
    expect(route.name).toBe(name)
    expect(route.meta.requiresAuth).toBe(false)
  })

  it('通过协议命名路由生成条款地址', async () => {
    const { default: router } = await import('@/router')
    const route = router.resolve({ name: 'LegalDocument', params: { documentId: 'terms' } })
    expect(route.path).toBe('/legal/terms')
    expect(route.meta.requiresAuth).toBe(false)
  })
})
