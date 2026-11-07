<template>
  <section class="card space-y-4 p-5" :aria-busy="loading">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="text-lg font-semibold">{{ t('qqbot.activityStats.title') }}</h2>
      <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
    </div>
    <form class="flex flex-wrap items-end gap-3" @submit.prevent="applyFilters">
      <label class="text-sm">{{ t('qqbot.activityStats.date') }}<input v-model="date" type="date" class="input mt-1" required /></label>
      <label class="text-sm">{{ t('qqbot.activityStats.search') }}<input v-model="search" class="input mt-1" maxlength="254" /></label>
      <label class="text-sm">{{ t('qqbot.activityStats.filter') }}<select v-model="filter" class="input mt-1"><option v-for="key in ['', 'chances', 'checkin', 'draws']" :key="key" :value="key">{{ t(`qqbot.activityStats.filters.${key || 'all'}`) }}</option></select></label>
      <button class="btn btn-secondary" :disabled="loading">{{ t('qqbot.activityStats.query') }}</button>
      <button type="button" class="btn btn-secondary" :disabled="loading || exporting" @click="exportRows">{{ t('qqbot.activityStats.export') }}</button>
    </form>
    <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('qqbot.activityStats.scope') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <template v-if="data && !error">
      <div class="grid grid-cols-2 gap-x-6 gap-y-3 border-y py-4 dark:border-dark-700 md:grid-cols-4">
        <button v-for="metric in metrics" :key="metric.key" type="button" class="text-left" :disabled="loading" @click="selectMetric(metric.filter)">
          <span class="block text-sm text-gray-500 dark:text-gray-400">{{ t(`qqbot.activityStats.${metric.key}`) }}</span>
          <span class="text-xl font-semibold tabular-nums">{{ metric.value }}</span>
        </button>
      </div>
      <p class="text-xs text-gray-500">{{ t('qqbot.activityStats.refreshed', { time: formatTime(data.refreshed_at) }) }}</p>
      <div class="overflow-x-auto">
        <table class="w-full whitespace-nowrap text-left text-sm">
          <thead class="border-b dark:border-dark-700"><tr><th class="py-2 pr-4">{{ t('qqbot.activityStats.account') }}</th><th class="pr-4">QQ</th><th class="pr-4">{{ t('qqbot.activityStats.checkin') }}</th><th class="pr-4">{{ t('qqbot.activityStats.drawCount') }}</th><th class="pr-4">{{ t('qqbot.activityStats.principal') }}</th><th class="pr-4">{{ t('qqbot.activityStats.bonus') }}</th><th class="pr-4">{{ t('qqbot.activityStats.used') }}</th><th>{{ t('qqbot.activityStats.remaining') }}</th></tr></thead>
          <tbody><tr v-for="u in data.items" :key="u.user_id" class="border-b dark:border-dark-700"><td class="py-3 pr-4"><span>{{ u.email }}</span><span class="block text-xs text-gray-500">#{{ u.user_id }} · {{ t(`qqbot.activityStats.accountStates.${u.deleted ? 'deleted' : u.status === 'active' ? 'active' : 'inactive'}`) }}</span></td><td class="pr-4">{{ u.members.join(' / ') || '—' }}</td><td class="pr-4">{{ u.checked_in ? `$${u.checkin_amount.toFixed(4)}` : '—' }}</td><td class="pr-4">{{ u.day_draws }} · ${{ u.day_draw_amount.toFixed(4) }}</td><td class="pr-4">¥{{ u.recharge_total.toFixed(2) }}</td><td class="pr-4">{{ u.bonus_chances }}</td><td class="pr-4">{{ u.used }}</td><td class="font-medium">{{ u.remaining }}</td></tr></tbody>
        </table>
        <p v-if="!data.items.length" class="py-5 text-sm text-gray-500">{{ t('common.noData') }}</p>
      </div>
      <div class="flex items-center justify-between gap-3 text-sm"><span>{{ t('qqBot.page', { page: data.page, total: data.total }) }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading || data.page <= 1" @click="page--; load()">{{ t('qqBot.previous') }}</button><button class="btn btn-secondary" :disabled="loading || data.page * data.page_size >= data.total" @click="page++; load()">{{ t('qqBot.next') }}</button></div></div>
    </template>
  </section>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import { qqbotAPI, type QQActivityStatsPage } from '@/api/qqcommunity'
const { t } = useI18n()
const date = ref(new Date(Date.now() + 8 * 3600000).toISOString().slice(0, 10)), search = ref(''), filter = ref(''), page = ref(1)
const data = ref<QQActivityStatsPage>(), loading = ref(false), exporting = ref(false), error = ref('')
let request = 0
const params = () => ({ date: date.value, search: search.value, filter: filter.value, page: page.value, page_size: 20 })
const formatTime = (value: string) => new Date(value).toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false })
const metrics = computed(() => { const s = data.value?.summary; return s ? [
 { key: 'checkinUsers', value: s.checkin_users, filter: 'checkin' }, { key: 'chanceUsers', value: s.chance_users, filter: 'chances' },
 { key: 'remainingChances', value: s.remaining_chances, filter: 'chances' }, { key: 'drawUsers', value: s.draw_users, filter: 'draws' },
 { key: 'drawCount', value: s.draw_count, filter: 'draws' }, { key: 'checkinAmount', value: `$${s.checkin_amount.toFixed(4)}`, filter: 'checkin' },
 { key: 'drawAmount', value: `$${s.draw_amount.toFixed(4)}`, filter: 'draws' }
] : [] })
async function load() { const seq = ++request; loading.value = true; error.value = ''; try { const result = await qqbotAPI.activityStats(params()); if (seq === request) data.value = result } catch (e) { if (seq === request) error.value = (e as Error).message } finally { if (seq === request) loading.value = false } }
function applyFilters() { page.value = 1; void load() }
function selectMetric(value: string) { filter.value = value; applyFilters() }
async function exportRows() { exporting.value = true; error.value = ''; try { saveAs(await qqbotAPI.exportActivityStats(params()), `activity-stats-${date.value}.csv`) } catch (e) { error.value = (e as Error).message } finally { exporting.value = false } }
onMounted(load)
onBeforeUnmount(() => { ++request })
</script>
