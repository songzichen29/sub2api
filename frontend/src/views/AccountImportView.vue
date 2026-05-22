<template>
  <HeaderOnlyLayout>
    <div class="mx-auto max-w-5xl space-y-4">
      <div class="flex items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('accountImport.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
            {{ t('accountImport.description') }}
          </p>
        </div>
      </div>

      <div v-if="loading" class="card p-5">
        <div class="text-sm text-gray-600 dark:text-dark-300">
          {{ t('common.loading') }}
        </div>
      </div>

      <div v-else-if="!status?.enabled" class="card p-5">
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('accountImport.disabled') }}
        </div>
      </div>

      <div v-else-if="!status.password_configured" class="card p-5">
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('accountImport.passwordNotConfigured') }}
        </div>
      </div>

      <form v-else-if="!token" class="card max-w-md space-y-4 p-5" @submit.prevent="verifyPassword">
        <div>
          <label class="input-label">{{ t('accountImport.password') }}</label>
          <input
            v-model="password"
            type="password"
            class="input"
            autocomplete="current-password"
            :placeholder="t('accountImport.passwordPlaceholder')"
          />
        </div>
        <button class="btn btn-primary" type="submit" :disabled="verifying || !password.trim()">
          {{ verifying ? t('accountImport.verifying') : t('accountImport.verify') }}
        </button>
      </form>

      <div v-else class="card p-5">
        <AccountDataImportForm
          show-actions
          form-id="standalone-account-import-form"
          :proxies="options.proxies"
          :groups="options.groups"
          :available-tags="options.tags"
          :templates-loader="loadTemplates"
          :importer="importData"
          :allow-proxy-test="false"
          :show-cancel-action="false"
        />
      </div>
    </div>
  </HeaderOnlyLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import HeaderOnlyLayout from '@/components/layout/HeaderOnlyLayout.vue'
import AccountDataImportForm from '@/components/admin/account/AccountDataImportForm.vue'
import { useAppStore } from '@/stores/app'
import type { AccountImportApplyTemplate } from '@/api/admin/settings'
import type { AdminDataImportResult, AdminGroup, Proxy as ProxyConfig } from '@/types'
import {
  getStandaloneAccountImportOptions,
  getStandaloneAccountImportStatus,
  getStandaloneAccountImportTemplates,
  importStandaloneAccountData,
  verifyStandaloneAccountImportPassword,
  type StandaloneAccountImportPayload,
  type StandaloneAccountImportStatus
} from '@/api/accountImport'

const TOKEN_STORAGE_KEY = 'standalone_account_import_token'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const verifying = ref(false)
const password = ref('')
const token = ref('')
const status = ref<StandaloneAccountImportStatus | null>(null)
const options = ref<{
  groups: AdminGroup[]
  proxies: ProxyConfig[]
  tags: string[]
}>({
  groups: [],
  proxies: [],
  tags: []
})

const loadOptions = async (nextToken: string) => {
  options.value = await getStandaloneAccountImportOptions(nextToken)
}

const loadStatus = async () => {
  loading.value = true
  try {
    status.value = await getStandaloneAccountImportStatus()
    if (status.value.enabled && status.value.password_configured) {
      const savedToken = sessionStorage.getItem(TOKEN_STORAGE_KEY)
      if (savedToken) {
        try {
          await loadOptions(savedToken)
          token.value = savedToken
        } catch {
          sessionStorage.removeItem(TOKEN_STORAGE_KEY)
        }
      }
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('accountImport.loadFailed'))
  } finally {
    loading.value = false
  }
}

const verifyPassword = async () => {
  const input = password.value.trim()
  if (!input) return

  verifying.value = true
  try {
    const result = await verifyStandaloneAccountImportPassword(input)
    token.value = result.token
    sessionStorage.setItem(TOKEN_STORAGE_KEY, result.token)
    password.value = ''
    await loadOptions(result.token)
    appStore.showSuccess(t('accountImport.verified'))
  } catch (error: any) {
    appStore.showError(error?.message || t('accountImport.invalidPassword'))
  } finally {
    verifying.value = false
  }
}

const loadTemplates = async (): Promise<AccountImportApplyTemplate[]> => {
  if (!token.value) return []
  return getStandaloneAccountImportTemplates(token.value)
}

const importData = async (payload: StandaloneAccountImportPayload): Promise<AdminDataImportResult> => {
  if (!token.value) {
    throw new Error(t('accountImport.invalidPassword'))
  }
  return importStandaloneAccountData(token.value, payload)
}

onMounted(loadStatus)
</script>
