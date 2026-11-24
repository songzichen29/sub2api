<template>
  <div class="space-y-2">
    <div v-if="displayUsage?.third_party_key" class="text-xs text-gray-500 dark:text-gray-400" data-testid="third-party-key" :title="displayUsage.third_party_key.last_used_at ? formatDateTime(displayUsage.third_party_key.last_used_at) : undefined">
      {{ keyIndex > 0
        ? t('admin.accounts.apiKeyPool.key', { index: displayUsage.third_party_key.index })
        : t(displayUsage.third_party_key.last_used_at ? 'admin.accounts.usageQuery.recentKey' : 'admin.accounts.usageQuery.primaryKey', { index: displayUsage.third_party_key.index }) }}
    </div>
    <AccountUsageQueryResult
      :quota="displayUsage?.third_party_quota"
      :error="(keyIndex > 0 ? selectedError : error) || displayUsage?.error"
      :loading="keyIndex > 0 ? querying : loading"
      :scope="isSub2API ? undefined : t('admin.accounts.usageQuery.panelAccountBalance')"
    >
      <div class="flex flex-wrap items-center gap-2">
        <select v-if="isSub2API && (account.api_key_count ?? 0) > 1" v-model.number="keyIndex" class="max-w-40 rounded border border-gray-300 bg-transparent px-1.5 py-1 text-xs dark:border-dark-600" :aria-label="t('admin.accounts.usageQuery.queryKey')" data-testid="usage-query-key" @change="query">
          <option :value="0">{{ t('admin.accounts.usageQuery.autoKey') }}</option>
          <option v-for="index in account.api_key_count" :key="index" :value="index">{{ t('admin.accounts.apiKeyPool.key', { index }) }}</option>
        </select>
        <button type="button" class="inline-flex min-h-7 items-center gap-1 rounded px-1.5 py-0.5 text-xs font-medium text-blue-600 hover:bg-blue-50 disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30" :disabled="keyIndex > 0 ? querying : loading" :aria-label="t('admin.accounts.usageWindow.activeQuery')" @click="query">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': keyIndex > 0 ? querying : loading }" />
          {{ t('admin.accounts.usageWindow.activeQuery') }}
        </button>
      </div>
    </AccountUsageQueryResult>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { Icon } from '@/components/icons'
import type { Account, AccountUsageInfo } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import AccountUsageQueryResult from './AccountUsageQueryResult.vue'

const props = defineProps<{ account: Account; usage: AccountUsageInfo | null; loading: boolean; error: string | null; manualRefreshToken: number }>()
const emit = defineEmits<{ refresh: [] }>()
const { t } = useI18n()
const keyIndex = ref(0)
const querying = ref(false)
const selectedUsage = ref<AccountUsageInfo | null>(null)
const selectedError = ref<string | null>(null)
const requestVersion = ref(0)
const isSub2API = computed(() => props.account.extra?.usage_query?.provider === 'sub2api')
const displayUsage = computed(() => keyIndex.value > 0 ? selectedUsage.value : props.usage)

watch(() => JSON.stringify([props.account.id, props.account.api_key_count, props.account.extra?.usage_query, props.account.credentials?.base_url]), () => {
  requestVersion.value++
  keyIndex.value = 0
  selectedUsage.value = null
  selectedError.value = null
  querying.value = false
})
watch(() => props.account.updated_at, () => { if (keyIndex.value > 0) void query() })
watch(() => props.manualRefreshToken, () => { if (keyIndex.value > 0) void query() })
onBeforeUnmount(() => { requestVersion.value++ })

async function query() {
  const version = ++requestVersion.value
  selectedUsage.value = null
  selectedError.value = null
  if (keyIndex.value === 0) {
    querying.value = false
    emit('refresh')
    return
  }
  querying.value = true
  try {
    const result = await adminAPI.accounts.getUsage(props.account.id, 'active', true, keyIndex.value)
    if (requestVersion.value === version) selectedUsage.value = result
  } catch (error) {
    if (requestVersion.value === version) selectedError.value = extractApiErrorMessage(error)
  } finally {
    if (requestVersion.value === version) querying.value = false
  }
}
</script>
