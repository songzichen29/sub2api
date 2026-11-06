<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div class="flex items-center justify-between gap-4">
        <h1 class="text-xl font-semibold">{{ t('lotteryPortal.title') }}</h1>
        <button class="btn btn-secondary" :disabled="loading" @click="refresh">{{ t('lotteryPortal.refresh') }}</button>
      </div>
      <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
      <template v-if="data">
        <section class="card space-y-4 p-6">
          <h2 class="text-lg font-medium">{{ data.title }}</h2>
          <p v-if="!data.enabled" role="status">{{ t('lotteryPortal.disabled') }}</p>
          <p>{{ t('lotteryPortal.rule', { amount: data.recharge_step.toFixed(2), count: data.chances_per_step }) }}</p>
          <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm">
            <div><dt class="text-gray-500">{{ t('lotteryPortal.recharge') }}</dt><dd class="mt-1">USD {{ data.state.recharge_total.toFixed(2) }}</dd></div>
            <div><dt class="text-gray-500">{{ t('lotteryPortal.earned') }}</dt><dd class="mt-1">{{ data.state.earned }}</dd></div>
            <div><dt class="text-gray-500">{{ t('lotteryPortal.used') }}</dt><dd class="mt-1">{{ data.state.used }}</dd></div>
            <div><dt class="text-gray-500">{{ t('lotteryPortal.remaining') }}</dt><dd class="mt-1 font-semibold" data-testid="lottery-remaining">{{ data.state.remaining }}</dd></div>
          </dl>
          <p v-if="data.pending_recharge" role="status">{{ t(pollAttempts >= 12 ? 'lotteryPortal.pendingLater' : 'lotteryPortal.pending') }}</p>
          <p>{{ t('lotteryPortal.next', { amount: data.state.next_recharge.toFixed(2), count: data.chances_per_step }) }}</p>
          <p v-if="data.state.guarantee_every">{{ t('lotteryPortal.guarantee', { count: data.state.draws_until_guarantee, every: data.state.guarantee_every }) }}</p>
        </section>

        <section v-if="data.enabled" class="card space-y-4 p-6">
          <h2 class="text-lg font-medium">{{ t('lotteryPortal.groups') }}</h2>
          <p role="status" data-testid="lottery-membership">{{ membershipText }}</p>
          <div class="flex flex-wrap gap-3">
            <button v-if="data.bound_qq" class="btn btn-secondary" :disabled="checking" @click="verify">{{ t('lotteryPortal.verify') }}</button>
            <button v-if="membership === 'member'" class="btn btn-primary" @click="copy('抽奖')">{{ t('lotteryPortal.copyDraw') }}</button>
            <a v-if="!data.bound_qq" href="#lottery-binding" class="btn btn-secondary">{{ t('lotteryPortal.bind') }}</a>
            <RouterLink v-else to="/profile" class="btn btn-secondary">{{ t('lotteryPortal.manageBinding') }}</RouterLink>
          </div>
          <p v-if="copyNotice" role="status" class="text-sm">{{ copyNotice }}</p>
          <p v-if="!data.public_groups.length" class="text-sm text-gray-500">{{ t('lotteryPortal.noGroups') }}</p>
          <ul class="divide-y divide-gray-200 dark:divide-dark-700">
            <li v-for="group in data.public_groups" :key="group.group_id" class="space-y-3 py-4">
              <p>{{ group.name }} · <span class="select-all">{{ group.group_id }}</span></p>
              <div class="flex flex-wrap gap-3">
                <a v-if="safeJoinURL(group.join_url)" :href="group.join_url" class="btn btn-primary" target="_blank" rel="noopener noreferrer">{{ t('lotteryPortal.join') }}</a>
                <button class="btn btn-secondary" @click="copy(group.group_id)">{{ t('lotteryPortal.copyGroup') }}</button>
                <button v-if="safeJoinURL(group.join_url)" class="btn btn-secondary" @click="showQR(group)">{{ t('lotteryPortal.showQR') }}</button>
              </div>
              <img v-if="qr?.id === group.group_id" :src="qr.url" :alt="t('lotteryPortal.qrAlt', { name: group.name })" width="192" height="192" />
            </li>
          </ul>
        </section>
        <div v-if="data.enabled && !data.bound_qq" id="lottery-binding" class="space-y-3">
          <p class="text-sm">{{ t('lotteryPortal.bindingHelp') }}</p>
          <ProfileQQBotCard @changed="refresh" />
        </div>

        <section class="card space-y-3 p-6">
          <h2 class="text-lg font-medium">{{ t('lotteryPortal.rules') }}</h2>
          <p class="text-sm">{{ t('lotteryPortal.period', { time: date(data.period_start) }) }}</p>
          <p class="text-sm text-gray-500">{{ t('lotteryPortal.accounting') }}</p>
          <div v-for="pool in pools" :key="pool.name" class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <caption class="py-3 text-left font-medium">{{ t(pool.name) }}</caption>
              <thead><tr class="border-b dark:border-dark-700"><th class="py-2 font-medium">{{ t('lotteryPortal.prize') }}</th><th class="font-medium">{{ t('lotteryPortal.amount') }}</th><th class="font-medium">{{ t('lotteryPortal.probability') }}</th></tr></thead>
              <tbody><tr v-for="(prize, i) in pool.prizes" :key="i" class="border-b dark:border-dark-700"><td class="py-2">{{ prize.name }}</td><td>{{ prize.amount }}</td><td>{{ (prize.probability_bps / 100).toFixed(2) }}%</td></tr></tbody>
            </table>
          </div>
        </section>
      </template>
      <section class="card space-y-4 p-6">
        <h2 class="text-lg font-medium">{{ t('lotteryPortal.history') }}</h2>
        <p v-if="historyError" role="alert" class="text-red-600">{{ historyError }}</p>
        <template v-else-if="history">
          <p v-if="!history.items.length">{{ t('lotteryPortal.emptyHistory') }}</p>
          <div v-else class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead><tr class="border-b dark:border-dark-700"><th class="py-2 font-medium">{{ t('lotteryPortal.date') }}</th><th class="font-medium">{{ t('lotteryPortal.prize') }}</th><th class="font-medium">{{ t('lotteryPortal.amount') }}</th></tr></thead>
              <tbody><tr v-for="draw in history.items" :key="draw.id" class="border-b dark:border-dark-700"><td class="whitespace-nowrap py-3 pr-3">{{ date(draw.created_at) }}</td><td>{{ draw.prize_name }} <span v-if="draw.is_guaranteed">({{ t('lotteryPortal.guaranteed') }})</span></td><td>{{ draw.amount }}</td></tr></tbody>
            </table>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <button class="btn btn-secondary" :disabled="historyLoading || history.page <= 1" @click="loadHistory(history.page - 1)">{{ t('lotteryPortal.previous') }}</button>
            <span class="text-sm">{{ t('lotteryPortal.page', { page: history.page, total: history.total }) }}</span>
            <button class="btn btn-secondary" :disabled="historyLoading || history.page * history.page_size >= history.total" @click="loadHistory(history.page + 1)">{{ t('lotteryPortal.nextPage') }}</button>
          </div>
        </template>
      </section>
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileQQBotCard from '@/components/user/profile/ProfileQQBotCard.vue'
import { lotteryAPI, type UserLottery, type UserLotteryDraws, type LotteryMembershipStatus } from '@/api/lottery'
import type { QQLotteryPublicGroup } from '@/api/qqcommunity'
import { safeJoinURL } from '@/utils/lottery'
const { t, locale } = useI18n()
const data = ref<UserLottery | null>(null), history = ref<UserLotteryDraws | null>(null)
const error = ref(''), historyError = ref(''), copyNotice = ref('')
const loading = ref(false), checking = ref(false), historyLoading = ref(false)
const membership = ref<LotteryMembershipStatus>('unknown')
const qr = ref<{ id: string; url: string } | null>(null)
let version = 0, historyVersion = 0, verificationVersion = 0, stopped = false
let refreshPending = false
let poll: ReturnType<typeof setTimeout> | undefined
const pollAttempts = ref(0)
const membershipText = computed(() => checking.value ? t('lotteryPortal.checking') :
  t('lotteryPortal.' + (data.value?.bound_qq ? membership.value : 'unbound'), { qq: data.value?.bound_qq, count: data.value?.state.remaining }))
const pools = computed(() => [
  { name: 'lotteryPortal.normalPool', prizes: data.value?.prizes || [] },
  { name: 'lotteryPortal.guaranteePool', prizes: data.value?.guarantee_prizes || [] },
].filter(pool => pool.prizes.length))
function date(value: string) { return new Intl.DateTimeFormat(locale.value, { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' }).format(new Date(value)) }
async function verify() {
  if (checking.value || !data.value?.bound_qq || !data.value.enabled) return
  const id = ++verificationVersion, qq = data.value.bound_qq
  checking.value = true
  try { const value = await lotteryAPI.membership(); if (id === verificationVersion && !stopped && data.value?.bound_qq === qq) membership.value = value.status }
  catch { if (id === verificationVersion) membership.value = 'unknown' }
  finally { if (id === verificationVersion) checking.value = false }
}
async function load(verifyMember: boolean) {
  if (loading.value) { refreshPending ||= verifyMember; return }
  const id = ++version; loading.value = true; error.value = ''
  try {
    const value = await lotteryAPI.overview()
    if (stopped || id !== version) return
    if (data.value?.bound_qq !== value.bound_qq || !value.enabled) { membership.value = 'unknown'; verificationVersion++; checking.value = false }
    data.value = value
    if (verifyMember) void verify()
    if (value.enabled && value.pending_recharge && pollAttempts.value < 12) {
      poll = setTimeout(() => { pollAttempts.value++; void load(false) }, 5000)
    }
  } catch { if (!stopped) error.value = t('lotteryPortal.loadFailed') }
  finally {
    if (id === version) {
      loading.value = false
      if (refreshPending && !stopped) { refreshPending = false; clearTimeout(poll); void load(true) }
    }
  }
}
async function loadHistory(page: number) {
  const id = ++historyVersion; historyLoading.value = true; historyError.value = ''
  try { const value = await lotteryAPI.draws(page); if (!stopped && id === historyVersion) history.value = value }
  catch { if (!stopped && id === historyVersion) historyError.value = t('lotteryPortal.historyFailed') }
  finally { if (id === historyVersion) historyLoading.value = false }
}
function refresh() { clearTimeout(poll); pollAttempts.value = 0; void load(true); void loadHistory(1) }
async function copy(text: string) { try { await navigator.clipboard.writeText(text); copyNotice.value = t('lotteryPortal.copied') } catch { copyNotice.value = t('lotteryPortal.copyFailed') } }
async function showQR(group: QQLotteryPublicGroup) {
  if (!safeJoinURL(group.join_url)) return
  try { qr.value = { id: group.group_id, url: await QRCode.toDataURL(group.join_url, { width: 192, margin: 2 }) } }
  catch { error.value = t('lotteryPortal.loadFailed') }
}
onMounted(() => { refresh(); window.addEventListener('focus', refresh) })
onBeforeUnmount(() => { stopped = true; version++; historyVersion++; verificationVersion++; clearTimeout(poll); window.removeEventListener('focus', refresh) })
</script>
