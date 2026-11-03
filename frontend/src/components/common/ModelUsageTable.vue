<template>
          <!-- Model Stats Table -->
          <div
            v-if="items.length > 0"
            class="fade-up fade-up-delay-4 rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm overflow-hidden dark:border-dark-700 dark:bg-dark-900/90"
          >
            <div class="px-8 py-5 border-b border-gray-200 dark:border-dark-700">
              <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.modelStats') }}</h3>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-950">
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.model') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.requests') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.inputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.outputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheCreationTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheReadTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.totalTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cost') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(m, i) in items"
                    :key="i"
                    class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
                  >
                    <td class="px-4 py-3 text-sm font-medium whitespace-nowrap text-gray-900 dark:text-white">{{ m.model || '-' }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.requests) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.input_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.output_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.cache_creation_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.cache_read_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200">{{ fmtNum(m.total_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right font-medium text-gray-900 dark:text-white">{{ usd(m.actual_cost != null ? m.actual_cost : m.cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
export interface ModelUsageRow {
 model: string; requests: number; input_tokens: number; output_tokens: number
 cache_creation_tokens: number; cache_read_tokens: number; total_tokens: number
 actual_cost?: number; cost?: number
}
defineProps<{ items: ModelUsageRow[] }>()
const { t } = useI18n()
const fmtNum = (value: number) => Number(value || 0).toLocaleString()
const usd = (value: number | undefined) => '$' + Number(value || 0).toFixed(4)
</script>
