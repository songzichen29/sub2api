import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

const routeState = vi.hoisted(() => ({
  params: { id: 'admin-page' },
}))

const fetchPublicSettings = vi.hoisted(() => vi.fn())
const getCustomPageStatus = vi.hoisted(() => vi.fn())
const adminFetch = vi.hoisted(() => vi.fn())

const appStoreState = vi.hoisted(() => ({
  publicSettingsLoaded: true,
  cachedPublicSettings: { custom_menu_items: [] as Array<any> },
  siteName: 'Sub2API',
}))

const authStoreState = vi.hoisted(() => ({
  isAdmin: true,
  token: 'jwt-token',
  user: { id: 1 },
}))

const adminSettingsState = vi.hoisted(() => ({
  loaded: false,
  customMenuItems: [{ id: 'admin-page', label: 'Admin Page', url: 'https://example.com/admin' }],
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => routeState,
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' },
    }),
  }
})

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    ...appStoreState,
    fetchPublicSettings,
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStoreState,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    ...adminSettingsState,
    fetch: adminFetch,
  }),
}))

vi.mock('@/api/customPage', () => ({
  getCustomPageStatus,
}))

import CustomPageView from '../CustomPageView.vue'

describe('CustomPageView', () => {
  beforeEach(() => {
    fetchPublicSettings.mockReset()
    adminFetch.mockReset()
    getCustomPageStatus.mockReset()

    appStoreState.publicSettingsLoaded = true
    appStoreState.cachedPublicSettings = { custom_menu_items: [] }

    authStoreState.isAdmin = true
    authStoreState.token = 'jwt-token'
    authStoreState.user = { id: 1 }

    adminSettingsState.loaded = false
    adminSettingsState.customMenuItems = [
      { id: 'admin-page', label: 'Admin Page', url: 'https://example.com/admin' },
    ]

    getCustomPageStatus.mockResolvedValue({ available: true })
  })

  it('fetches admin settings for admin-only custom pages even when public settings are already loaded', async () => {
    mount(CustomPageView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
        },
      },
    })

    await flushPromises()

    expect(fetchPublicSettings).not.toHaveBeenCalled()
    expect(adminFetch).toHaveBeenCalledTimes(1)
    expect(getCustomPageStatus).toHaveBeenCalledWith('admin-page')
  })
})
