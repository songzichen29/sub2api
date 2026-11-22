<template>
  <main class="flex min-h-screen items-center justify-center bg-gray-50 px-6 dark:bg-dark-950">
    <div class="w-full max-w-md space-y-6">
      <p class="font-mono text-5xl font-medium text-gray-400 dark:text-dark-500">404</p>
      <div class="space-y-3">
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('errors.pageNotFound') }}</h1>
        <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">{{ t('common.pageNotFoundDescription') }}</p>
      </div>
      <div class="flex flex-wrap gap-3">
        <button v-if="canGoBack" type="button" class="btn btn-secondary" @click="router.back()">
          <Icon name="arrowLeft" size="sm" />
          {{ t('common.back') }}
        </button>
        <RouterLink :to="homePath" class="btn btn-primary">
          <Icon name="home" size="sm" />
          {{ t('common.goHome') }}
        </RouterLink>
      </div>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const appStore = useAppStore()
const canGoBack = !!window.history.state.back
const homePath = computed(() => {
  if (authStore.isAdmin) return '/admin/dashboard'
  if (appStore.backendModeEnabled) return '/login'
  if (authStore.isAuthenticated) return '/dashboard'
  return '/home'
})
</script>
