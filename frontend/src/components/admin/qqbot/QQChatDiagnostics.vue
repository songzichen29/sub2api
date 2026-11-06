<template>
  <section class="space-y-3" :aria-label="t('qqbot.runtimeChat.diagnostics')" data-testid="qq-chat-diagnostics">
    <h2 class="text-lg font-semibold">{{ t('qqbot.runtimeChat.diagnostics') }}</h2>
    <p v-if="!fresh" role="status" class="text-sm text-amber-700 dark:text-amber-400">{{ t('qqbot.runtimeChat.metricsPending') }}</p>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">{{ t('qqbot.runtimeChat.syncedAt', { time: new Date(status!.last_seen_at!).toLocaleString() }) }}</p>
    <dl class="flex flex-wrap gap-x-6 gap-y-3 text-sm">
      <div v-for="key in metrics" :key="key"><dt class="text-gray-500 dark:text-gray-400">{{ t('qqbot.runtimeChat.metrics.' + key) }}</dt><dd class="mt-1 font-medium tabular-nums" :data-testid="'chat-metric-' + key">{{ fresh ? status?.[key] ?? '—' : '—' }}</dd></div>
    </dl>
    <template v-if="showRecords">
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('qqbot.runtimeChat.recentHelp') }}</p>
      <div v-if="status?.recent?.length" class="overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead><tr class="text-gray-500 dark:text-gray-400"><th class="pb-2 pr-4">{{ t('qqbot.time') }}</th><th class="pb-2 pr-4">{{ t('qqbot.runtimeChat.conversation') }}</th><th class="pb-2 pr-4">QQ / {{ t('qqbot.runtimeChat.messageId') }}</th><th class="pb-2 pr-4">{{ t('common.status') }}</th><th class="pb-2">{{ t('qqbot.runtimeChat.reason') }}</th></tr></thead>
          <tbody><tr v-for="record in status.recent" :key="record.id" class="border-t align-top dark:border-dark-700">
            <td class="whitespace-nowrap py-3 pr-4">{{ new Date(record.updated).toLocaleString() }}</td>
            <td class="break-all py-3 pr-4">{{ record.scope }}</td>
            <td class="break-all py-3 pr-4">{{ record.sender }} / {{ record.message_id }}</td>
            <td class="whitespace-nowrap py-3 pr-4">{{ label('states', record.status) }}</td>
            <td class="py-3">{{ record.reason ? label('reasons', record.reason) : '—' }}</td>
          </tr></tbody>
        </table>
      </div>
      <p v-else class="text-sm text-gray-500 dark:text-gray-400">{{ t(fresh ? 'common.noData' : 'qqbot.runtimeChat.metricsPending') }}</p>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQChatRuntimeStatus } from '@/api/qqcommunity'
import { runtimeMetricsFresh } from '@/utils/qqRuntime'
const props = withDefaults(defineProps<{ status?: QQChatRuntimeStatus; showRecords?: boolean }>(), { showRecords: true })
const { t, te } = useI18n()
const now = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => { timer = setInterval(() => { now.value = Date.now() }, 5000) })
onBeforeUnmount(() => { clearInterval(timer) })
const fresh = computed(() => runtimeMetricsFresh(props.status, now.value))
const metrics = ['daily_calls', 'daily_sent', 'daily_failed', 'unconfirmed', 'queued', 'active'] as const
function label(kind: string, code: string) { const key = `qqbot.runtimeChat.${kind}.${code}`; return te(key) ? t(key) : code }
</script>
