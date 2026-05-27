<template>
  <AppLayout>
    <div class="custom-page-layout">
      <div class="card flex-1 min-h-0 overflow-hidden">
        <div v-if="loading" class="flex h-full items-center justify-center py-12">
          <div
            class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
          ></div>
        </div>

        <div
          v-else-if="!menuItem"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="link" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('customPage.notFoundTitle') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('customPage.notFoundDesc') }}
            </p>
          </div>
        </div>

        <div v-else-if="!isValidUrl" class="flex h-full items-center justify-center p-10 text-center">
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="link" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('customPage.notConfiguredTitle') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('customPage.notConfiguredDesc') }}
            </p>
          </div>
        </div>

        <div
          v-else-if="availability === 'checking'"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <div
                class="h-5 w-5 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
              ></div>
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('customPage.checkingTitle') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('customPage.checkingDesc') }}
            </p>
          </div>
        </div>

        <div
          v-else-if="availability === 'unavailable'"
          class="flex h-full items-center justify-center p-10 text-center"
        >
          <div class="max-w-md">
            <div
              class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
            >
              <Icon name="exclamationTriangle" size="lg" class="text-gray-400" />
            </div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('customPage.unavailableTitle') }}
            </h3>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('customPage.unavailableDesc') }}
            </p>
            <p
              v-if="unavailableStatusCode"
              class="mt-2 text-xs text-gray-400 dark:text-dark-500"
            >
              {{ t('customPage.unavailableStatus', { status: unavailableStatusCode }) }}
            </p>
            <div class="mt-5 flex flex-wrap justify-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="retryAvailabilityCheck">
                {{ t('common.retry') }}
              </button>
              <a
                :href="embeddedUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="btn btn-secondary btn-sm"
              >
                {{ t('customPage.openInNewTab') }}
              </a>
            </div>
          </div>
        </div>

        <div v-else class="custom-embed-shell">
          <a
            :href="embeddedUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="btn btn-secondary btn-sm custom-open-fab"
          >
            <Icon name="externalLink" size="sm" class="mr-1.5" :stroke-width="2" />
            {{ t('customPage.openInNewTab') }}
          </a>
          <iframe
            :src="embeddedUrl"
            class="custom-embed-frame"
            allowfullscreen
          ></iframe>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { useAuthStore } from '@/stores/auth'
import { useAdminSettingsStore } from '@/stores/adminSettings'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { getCustomPageStatus } from '@/api/customPage'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'

const { t, locale } = useI18n()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const adminSettingsStore = useAdminSettingsStore()

const loading = ref(false)
const pageTheme = ref<'light' | 'dark'>('light')
const availability = ref<'idle' | 'checking' | 'available' | 'unavailable'>('idle')
const unavailableStatusCode = ref<number | null>(null)
let themeObserver: MutationObserver | null = null
let availabilityCheckSeq = 0

const menuItemId = computed(() => route.params.id as string)

const menuItem = computed(() => {
  const id = menuItemId.value
  // Try public settings first (contains user-visible items)
  const publicItems = appStore.cachedPublicSettings?.custom_menu_items ?? []
  const found = publicItems.find((item) => item.id === id) ?? null
  if (found) return found
  // For admin users, also check admin settings (contains admin-only items)
  if (authStore.isAdmin) {
    return adminSettingsStore.customMenuItems.find((item) => item.id === id) ?? null
  }
  return null
})

const embeddedUrl = computed(() => {
  if (!menuItem.value) return ''
  return buildEmbeddedUrl(
    menuItem.value.url,
    authStore.user?.id,
    authStore.token,
    pageTheme.value,
    locale.value,
  )
})

const isValidUrl = computed(() => {
  const url = embeddedUrl.value
  return url.startsWith('http://') || url.startsWith('https://')
})

async function checkPageAvailability() {
  const seq = ++availabilityCheckSeq
  unavailableStatusCode.value = null

  if (!menuItem.value || !isValidUrl.value) {
    availability.value = 'idle'
    return
  }

  availability.value = 'checking'
  try {
    const status = await getCustomPageStatus(menuItemId.value)
    if (seq !== availabilityCheckSeq) return
    availability.value = status.available ? 'available' : 'unavailable'
    unavailableStatusCode.value = status.status_code ?? null
  } catch {
    if (seq !== availabilityCheckSeq) return
    // 后端状态探测失败时不阻断自定义页面，避免因为探测接口异常影响原有可用入口。
    availability.value = 'available'
  }
}

function retryAvailabilityCheck() {
  void checkPageAvailability()
}

watch(
  () => [menuItemId.value, embeddedUrl.value, isValidUrl.value] as const,
  () => {
    void checkPageAvailability()
  },
  { immediate: true },
)

onMounted(async () => {
  pageTheme.value = detectTheme()

  if (typeof document !== 'undefined') {
    themeObserver = new MutationObserver(() => {
      pageTheme.value = detectTheme()
    })
    themeObserver.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }

  if (appStore.publicSettingsLoaded && (!authStore.isAdmin || adminSettingsStore.loaded)) return

  loading.value = true
  try {
    const tasks: Array<Promise<unknown>> = []
    if (!appStore.publicSettingsLoaded) {
      tasks.push(appStore.fetchPublicSettings())
    }
    if (authStore.isAdmin && !adminSettingsStore.loaded) {
      tasks.push(adminSettingsStore.fetch())
    }
    if (tasks.length > 0) {
      await Promise.all(tasks)
    }
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  }
})
</script>

<style scoped>
.custom-page-layout {
  @apply flex flex-col;
  height: calc(100vh - 64px - 4rem);
}

.custom-embed-shell {
  @apply relative;
  @apply h-full w-full overflow-hidden rounded-2xl;
  @apply bg-gradient-to-b from-gray-50 to-white dark:from-dark-900 dark:to-dark-950;
  @apply p-0;
}

.custom-open-fab {
  @apply absolute right-3 top-3 z-10;
  @apply shadow-sm backdrop-blur supports-[backdrop-filter]:bg-white/80 dark:supports-[backdrop-filter]:bg-dark-800/80;
}

.custom-embed-frame {
  display: block;
  margin: 0;
  width: 100%;
  height: 100%;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  background: transparent;
}
</style>
