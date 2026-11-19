<template>
  <div class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex items-center justify-between gap-3">
      <h3 class="input-label mb-0">{{ t('admin.accounts.usageQuery.title') }}</h3>
      <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
        <input :checked="enabled" type="checkbox" data-testid="usage-query-enabled" class="h-4 w-4 rounded border-gray-300" @change="emit('update:enabled', ($event.target as HTMLInputElement).checked)" />
        {{ t('admin.accounts.usageQuery.enable') }}
      </label>
    </div>
    <div v-if="enabled" class="space-y-3">
      <div>
        <label class="input-label">{{ t('admin.accounts.usageQuery.provider') }}</label>
        <select :value="provider" data-testid="usage-query-provider" class="input" @change="emit('update:provider', ($event.target as HTMLSelectElement).value as AccountUsageQueryProvider)">
          <option value="newapi">NewAPI</option>
          <option value="sub2api">Sub2API</option>
        </select>
        <p class="input-hint">{{ t(provider === 'sub2api' ? 'admin.accounts.usageQuery.sub2apiHint' : 'admin.accounts.usageQuery.newapiHint') }}</p>
      </div>
      <template v-if="provider === 'newapi'">
        <div>
          <label class="input-label">{{ t('admin.accounts.usageQuery.baseUrl') }}</label>
          <input :value="baseUrl" type="text" required class="input" placeholder="https://newapi.example.com" data-testid="usage-query-base-url" @input="emit('update:baseUrl', ($event.target as HTMLInputElement).value)" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.usageQuery.accessToken') }}</label>
          <input :value="accessToken" type="password" :required="!hasStoredToken" class="input font-mono" autocomplete="new-password" :placeholder="t('admin.accounts.usageQuery.accessTokenPlaceholder')" data-testid="usage-query-access-token" @input="emit('update:accessToken', ($event.target as HTMLInputElement).value)" />
          <p v-if="hasStoredToken" class="input-hint">{{ t('admin.accounts.usageQuery.editTokenHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.accounts.usageQuery.userId') }}</label>
          <input :value="userId" type="text" required class="input" placeholder="12345" data-testid="usage-query-user-id" @input="emit('update:userId', ($event.target as HTMLInputElement).value)" />
        </div>
      </template>
      <template v-else>
        <div class="text-sm">
          <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.baseUrl') }}</span>
          <div class="break-all font-mono">{{ accountBaseUrl || t('admin.accounts.usageQuery.urlRequired') }}</div>
        </div>
        <div v-if="apiKeys.length > 1">
          <label class="input-label">{{ t('admin.accounts.usageQuery.queryKey') }}</label>
          <select v-model.number="selectedKey" class="input" data-testid="usage-query-preview-key">
            <option v-for="(key, index) in apiKeys" :key="key.id ?? index" :value="index">{{ t('admin.accounts.apiKeyPool.key', { index: index + 1 }) }}{{ key.masked ? ` · ${key.masked}` : '' }}</option>
          </select>
        </div>
      </template>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="testing" data-testid="usage-query-preview" @click="preview">{{ t(testing ? 'admin.accounts.usageQuery.testing' : 'admin.accounts.usageQuery.test') }}</button>
      <AccountUsageQueryResult
        v-if="testing || result || queryError"
        :quota="result?.third_party_quota"
        :loading="testing"
        :error="queryError || result?.error"
        :scope="provider === 'newapi' ? t('admin.accounts.usageQuery.panelAccountBalance') : t('admin.accounts.apiKeyPool.key', { index: selectedKey + 1 })"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountUsageInfo, AccountUsageQueryPreviewRequest } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { buildUsageQueryConfig, type AccountUsageQueryProvider } from '@/utils/accountUsageQuery'
import { accountApiKeyInputError, type AccountAPIKeyDraft } from './accountApiKeys'
import AccountUsageQueryResult from './AccountUsageQueryResult.vue'

const props = defineProps<{
  enabled: boolean
  provider: AccountUsageQueryProvider
  baseUrl: string
  accessToken: string
  userId: string
  accountId?: number
  accountBaseUrl: string
  apiKeys: AccountAPIKeyDraft[]
  hasStoredToken?: boolean
}>()
const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:provider': [value: AccountUsageQueryProvider]
  'update:baseUrl': [value: string]
  'update:accessToken': [value: string]
  'update:userId': [value: string]
}>()
const { t } = useI18n()
const selectedKey = ref(0)
const testing = ref(false)
const result = ref<AccountUsageInfo | null>(null)
const queryError = ref<string | null>(null)
const requestVersion = ref(0)
const queryIdentity = computed(() => JSON.stringify([props, selectedKey.value]))
watch(queryIdentity, () => {
  requestVersion.value++
  result.value = null
  queryError.value = null
  testing.value = false
  if (selectedKey.value >= props.apiKeys.length) selectedKey.value = 0
}, { flush: 'sync' })
onBeforeUnmount(() => { requestVersion.value++ })

async function preview() {
  const config = buildUsageQueryConfig(props.provider, {
    baseUrl: props.baseUrl,
    accessToken: props.accessToken,
    userId: props.userId
  }, props.hasStoredToken)
  if (!config) {
    queryError.value = t('admin.accounts.usageQuery.incompleteFields')
    return
  }
  const request: AccountUsageQueryPreviewRequest = { account_id: props.accountId, usage_query: config }
  if (props.provider === 'sub2api') {
    const key = props.apiKeys[selectedKey.value]
    if (!props.accountBaseUrl.trim() || !key) {
      queryError.value = t('admin.accounts.usageQuery.sub2apiMissingFields')
      return
    }
    request.account_base_url = props.accountBaseUrl.trim()
    if (key.value.trim() || !key.id) {
      const keyError = accountApiKeyInputError(key.value)
      if (keyError) {
        queryError.value = t(`admin.accounts.apiKeyPool.errors.${keyError}`)
        return
      }
      request.api_key = key.value.trim()
    }
    if (!key.value.trim() && key.id) request.key_id = key.id
  }
  const version = ++requestVersion.value
  testing.value = true
  queryError.value = null
  result.value = null
  try {
    const response = await adminAPI.accounts.previewUsageQuery(request)
    if (version === requestVersion.value) result.value = response
  } catch (error) {
    if (version === requestVersion.value) queryError.value = extractApiErrorMessage(error)
  } finally {
    if (version === requestVersion.value) testing.value = false
  }
}
</script>
