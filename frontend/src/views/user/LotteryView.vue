<template>
  <AppLayout>
    <div class="workspace-page lottery-page space-y-5">
      <div class="flex items-center justify-between gap-4">
        <h1>{{ data?.title || t('lotteryPortal.title') }}</h1>
        <button class="btn btn-secondary" :disabled="loading || spinning" @click="refresh">{{ t('lotteryPortal.refresh') }}</button>
      </div>
      <p v-if="error" role="alert" class="text-red-600">{{ error }}</p>
      <template v-if="data">
        <p v-if="!data.enabled" role="status" class="rounded-lg border p-4 text-sm text-gray-500">{{ t('lotteryPortal.disabled') }}</p>
        <div class="lottery-main">
          <section class="lottery-play" :aria-busy="spinning">
            <div class="lottery-stage-heading">
              <span>{{ t('lotteryPortal.stageTitle') }}</span>
            </div>
            <LotteryWheel :prizes="wheelPrizes" :rotation="rotation" :disabled="!canSpin" :spinning="spinning" :rolling="spinning && !!result && !result.is_guaranteed" :won="!!result && !spinning && result.prize.amount > 0" :action-label="spinLabel" @spin="spin" @settled="finishSpin" />
            <div class="lottery-outcome">
              <div v-if="result && !spinning" role="status" aria-live="polite" class="lottery-result" :class="{ 'is-win': result.prize.amount > 0 }" data-testid="lottery-result">
                <span class="lottery-result-label">{{ t('lotteryPortal.result') }}<span v-if="result.is_guaranteed"> · {{ t('lotteryPortal.guaranteed') }}</span></span>
                <strong>{{ result.prize.name }}</strong>
                <span v-if="result.prize.amount > 0">+ USD {{ result.prize.amount }} · {{ t('lotteryPortal.credited') }}</span>
              </div>
              <p v-else class="lottery-idle-hint" role="status">{{ stageHint }}</p>
              <p class="lottery-odds-note">{{ t('lotteryPortal.wheelHelp') }}</p>
            </div>
          </section>
          <aside class="card lottery-account">
            <div class="lottery-chances">
              <div><h2>{{ t('lotteryPortal.myChances') }}</h2><p>{{ t('lotteryPortal.spinCost') }}</p></div>
              <div class="lottery-chances-count"><strong data-testid="lottery-remaining">{{ data.state.remaining }}</strong><span>{{ t('lotteryPortal.chanceUnit') }}</span></div>
            </div>
            <button data-testid="lottery-spin" class="btn btn-primary lottery-spin" :disabled="!canSpin" @click="spin">{{ spinLabel }}</button>
            <p v-if="spinError" role="alert" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ spinError }}</p>
            <div class="lottery-recharge">
              <span>{{ t('lotteryPortal.next', { amount: data.state.next_recharge.toFixed(2), count: data.chances_per_step }) }}</span>
              <RouterLink to="/purchase">{{ t('lotteryPortal.rechargeAction') }} <span aria-hidden="true">→</span></RouterLink>
            </div>
            <p v-if="data.pending_recharge" role="status" class="mt-3 text-sm text-amber-700 dark:text-amber-300">{{ t(pollAttempts >= 12 ? 'lotteryPortal.pendingLater' : 'lotteryPortal.pending') }}</p>
            <div class="lottery-prizes">
              <h2>{{ t('lotteryPortal.availablePrizes') }}</h2>
              <p v-if="data.token_bonus && data.token_bonus.bonus_bps > 0 && !isGuaranteeNext" class="lottery-token-bonus" data-testid="lottery-token-bonus">{{ t('lotteryPortal.tokenBonus', { bonus: (data.token_bonus.bonus_bps / 100).toLocaleString(locale, { maximumFractionDigits: 2 }) }) }}</p>
              <ul>
                <li v-for="(prize, index) in listedPrizes" :key="index">
                  <span class="lottery-prize-name"><span class="lottery-prize-dot" :class="{ 'is-top': prize.amount > 0 && prize.amount === nextTopPrize }" aria-hidden="true"></span>{{ prize.name }}</span>
                  <span v-if="prize.remaining != null" class="lottery-prize-stock">{{ t('lotteryPortal.stockRemaining', { count: prize.remaining }) }}</span>
                  <span v-else-if="prize.amount > 0" class="lottery-prize-stock">{{ t('lotteryPortal.stockUnlimited') }}</span>
                </li>
              </ul>
              <p v-if="!listedPrizes.length" class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('lotteryPortal.noAvailablePrizes') }}</p>
            </div>
            <dl class="lottery-account-stats">
              <div><dt>{{ t('lotteryPortal.recharge') }}</dt><dd>CNY {{ data.state.recharge_total.toFixed(2) }}</dd></div>
              <div><dt>{{ t('lotteryPortal.used') }}</dt><dd>{{ data.state.used }}</dd></div>
            </dl>
            <LotteryChanceBreakdown :state="data.state" />
          </aside>
        </div>

        <details v-if="data.enabled" class="card space-y-4 p-5 text-sm">
          <summary class="cursor-pointer">{{ t('lotteryPortal.groupOptional') }}</summary>
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
        <div v-if="!data.bound_qq" id="lottery-binding" class="space-y-3">
          <p class="text-sm">{{ t('lotteryPortal.bindingHelp') }}</p>
          <ProfileQQBotCard @changed="refresh" />
        </div>
        </details>

        <details class="card space-y-3 p-5">
          <summary class="cursor-pointer">{{ t('lotteryPortal.rules') }}</summary>
          <p class="text-sm">{{ t('lotteryPortal.rule', { amount: data.recharge_step.toFixed(2), count: data.chances_per_step }) }}</p>
          <p class="text-sm">{{ t('lotteryPortal.sharedChances') }}</p>
          <p v-if="data.token_bonus" class="text-sm">{{ t('lotteryPortal.tokenBonusRule') }}</p>
          <p v-if="data.prizes.some(prize => prize.stock_limit != null)" class="text-sm">{{ t('lotteryPortal.stockRule') }}</p>
          <p class="text-sm">{{ t('lotteryPortal.period', { time: date(data.period_start) }) }}</p>
          <p class="text-sm text-gray-500">{{ t('lotteryPortal.accounting') }}</p>
        </details>
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileQQBotCard from '@/components/user/profile/ProfileQQBotCard.vue'
import LotteryWheel from '@/components/user/LotteryWheel.vue'
import LotteryChanceBreakdown from '@/components/user/LotteryChanceBreakdown.vue'
import { lotteryAPI, type UserLottery, type UserLotteryDraws, type UserLotterySpin, type LotteryMembershipStatus } from '@/api/lottery'
import type { QQLotteryPublicGroup } from '@/api/qqcommunity'
import { safeJoinURL } from '@/utils/lottery'
const { t, locale } = useI18n()
const data = ref<UserLottery | null>(null), history = ref<UserLotteryDraws | null>(null)
const error = ref(''), historyError = ref(''), copyNotice = ref('')
const loading = ref(false), checking = ref(false), historyLoading = ref(false)
const membership = ref<LotteryMembershipStatus>('unknown')
const qr = ref<{ id: string; url: string } | null>(null)
const spinning = ref(false), spinError = ref(''), requestID = ref(''), rotation = ref(0)
const result = ref<UserLotterySpin | null>(null)
const isGuaranteeNext = computed(() => data.value?.state.draws_until_guarantee === 1 && !!data.value.guarantee_prizes.length)
const listedPrizes = computed(() => {
  const prizes: { name: string; amount: number; remaining: number | null }[] = []
  for (const prize of data.value?.prizes || []) {
    if (prize.amount <= 0) continue
    const remaining = prize.stock_limit == null ? null : Math.max(prize.stock_limit - (prize.allocated || 0), 0)
    const existing = prizes.find(item => item.name === prize.name && item.amount === prize.amount)
    if (existing) {
      existing.remaining = existing.remaining == null || remaining == null ? null : existing.remaining + remaining
      continue
    }
    prizes.push({ name: prize.name, amount: prize.amount, remaining })
  }
  return prizes
})
const wheelPrizes = computed(() => spinning.value && result.value && !result.value.is_guaranteed ? result.value.prizes : data.value?.prizes || [])
const nextTopPrize = computed(() => Math.max(0, ...(data.value?.prizes || []).map(prize => prize.amount)))
const spinLabel = computed(() => t(spinning.value ? 'lotteryPortal.spinning' : requestID.value ? 'lotteryPortal.retrySpin' : data.value?.state.remaining ? 'lotteryPortal.spin' : 'lotteryPortal.noChances'))
const canSpin = computed(() => !spinning.value && !loading.value && !!data.value && (!!requestID.value || (data.value.enabled && data.value.state.remaining > 0 && data.value.prizes.length > 0)))
const stageHint = computed(() => {
  if (spinning.value) return t('lotteryPortal.spinningHint')
  if (!data.value?.state.remaining) return t('lotteryPortal.noChances')
  return t('lotteryPortal.tapWheel')
})
let spinTimer: ReturnType<typeof setTimeout> | undefined
let version = 0, historyVersion = 0, verificationVersion = 0, stopped = false
let refreshPending = false
let poll: ReturnType<typeof setTimeout> | undefined
const pollAttempts = ref(0)
const membershipText = computed(() => checking.value ? t('lotteryPortal.checking') :
  t('lotteryPortal.' + (data.value?.bound_qq ? membership.value : 'unbound'), { qq: data.value?.bound_qq, count: data.value?.state.remaining }))
const storageKey = () => `lottery-pending:${data.value?.state.user_id}`
function persistRequest(value: string) {
  requestID.value = value
  try { if (value) sessionStorage.setItem(storageKey(), value); else sessionStorage.removeItem(storageKey()) } catch { /* In-memory retry still works without storage. */ }
}
async function spin() {
  if (!canSpin.value) return
  spinning.value = true; spinError.value = ''; result.value = null
  clearTimeout(poll)
  try {
    if (!requestID.value) persistRequest(crypto.randomUUID())
    const outcome = await lotteryAPI.spin(requestID.value)
    if (stopped) return
    result.value = outcome
    if (outcome.is_guaranteed) {
      finishSpin()
      return
    }
    await nextTick()
    const target = (360 - outcome.prize_index * 360 / outcome.prizes.length) % 360
    rotation.value += 360 * 5 + (target - rotation.value % 360 + 360) % 360
    spinTimer = setTimeout(finishSpin, window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ? 30 : 4000)
  } catch (e) {
    if (stopped) return
    spinning.value = false
    const failure = e as { status?: number; message?: string }
    if (failure.status === 400 || failure.status === 403) {
      persistRequest(''); spinError.value = failure.message || t('lotteryPortal.spinFailed'); void load(false)
    } else { spinError.value = t('lotteryPortal.spinFailed') }
  }
}
function finishSpin() {
  if (!spinning.value || !result.value || stopped) return
  clearTimeout(spinTimer); spinning.value = false; persistRequest('')
  if (data.value) data.value.state.remaining = result.value.remaining
  void load(false); void loadHistory(1)
}
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
    try { requestID.value = sessionStorage.getItem(storageKey()) || requestID.value } catch { /* optional persistence */ }
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
function refresh() { if (spinning.value) return; clearTimeout(poll); pollAttempts.value = 0; void load(true); void loadHistory(1) }
async function copy(text: string) { try { await navigator.clipboard.writeText(text); copyNotice.value = t('lotteryPortal.copied') } catch { copyNotice.value = t('lotteryPortal.copyFailed') } }
async function showQR(group: QQLotteryPublicGroup) {
  if (!safeJoinURL(group.join_url)) return
  try { qr.value = { id: group.group_id, url: await QRCode.toDataURL(group.join_url, { width: 192, margin: 2 }) } }
  catch { error.value = t('lotteryPortal.loadFailed') }
}
onMounted(() => { refresh(); window.addEventListener('focus', refresh) })
onBeforeUnmount(() => { stopped = true; version++; historyVersion++; verificationVersion++; clearTimeout(poll); clearTimeout(spinTimer); window.removeEventListener('focus', refresh) })
</script>
<style scoped>
.lottery-main { display: grid; grid-template-columns: minmax(0, 1.3fr) minmax(340px, 1fr); gap: 24px; align-items: start; }
.lottery-play { min-width: 0; padding: 24px; border: 1px solid #e5e7eb; border-radius: 8px; background: #fff; text-align: center; }
.lottery-stage-heading { margin-bottom: 16px; color: #374151; font-size: 15px; font-weight: 600; text-align: left; }
.lottery-outcome { min-height: 88px; margin-top: 16px; }
.lottery-idle-hint { color: #374151; font-size: 14px; }
.lottery-odds-note { margin-top: 8px; color: #6b7280; font-size: 13px; }
.lottery-result { display: flex; flex-direction: column; gap: 3px; color: #536557; font-size: 13px; animation: result-reveal 280ms ease-out; }
.lottery-result.is-win { color: #176349; }
.lottery-result-label { font-size: 12px; }
.lottery-result strong { font-size: 24px; font-weight: 600; }
.lottery-account { padding: 24px; }
.lottery-chances { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.lottery-chances p { margin-top: 5px; color: #6b7280; font-size: 13px; }
.lottery-chances-count { display: flex; align-items: baseline; gap: 6px; color: #0f766e; }
.lottery-chances-count strong { font-size: 48px; line-height: 1.2; font-weight: 600; font-variant-numeric: tabular-nums; }
.lottery-chances-count span { font-size: 13px; }
.lottery-page .lottery-spin { width: 100%; min-height: 46px; margin-top: 20px; font-size: 16px; }
.lottery-recharge { display: flex; align-items: baseline; justify-content: space-between; gap: 16px; margin-top: 12px; color: #6b7280; font-size: 12px; }
.lottery-recharge a { flex-shrink: 0; color: #0f766e; font-size: 13px; font-weight: 500; }
.lottery-prizes { margin-top: 24px; padding-top: 20px; border-top: 1px solid #e5e7eb; }
.lottery-prizes ul { margin-top: 8px; }
.lottery-token-bonus { margin-top: 10px; color: #0f766e; font-size: 14px; line-height: 1.6; }
.lottery-prizes li { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 13px 0; border-bottom: 1px solid #f1f3f1; }
.lottery-prizes li:last-child { border-bottom: 0; }
.lottery-prize-name { display: flex; align-items: center; gap: 10px; min-width: 0; overflow-wrap: anywhere; font-size: 14px; }
.lottery-prize-dot { width: 8px; height: 8px; flex-shrink: 0; border-radius: 2px; background: #aac9b9; }
.lottery-prize-dot.is-top { background: #c29a4e; }
.lottery-prize-stock { flex-shrink: 0; color: #6b7280; font-size: 14px; font-variant-numeric: tabular-nums; }
.lottery-account-stats { display: flex; justify-content: space-between; gap: 12px; margin-top: 20px; padding-top: 18px; border-top: 1px solid #e5e7eb; font-size: 12px; }
.lottery-account-stats dt { color: #6b7280; }
.lottery-account-stats dd { margin-top: 5px; font-size: 14px; font-variant-numeric: tabular-nums; }
.dark .lottery-play { background: #0f172a; border-color: #334155; }
.dark .lottery-stage-heading, .dark .lottery-idle-hint { color: #dae7da; }
.dark .lottery-odds-note, .dark .lottery-chances p, .dark .lottery-recharge, .dark .lottery-account-stats dt, .dark .lottery-prize-stock { color: #a0b0ab; }
.dark .lottery-token-bonus { color: #8bd3b6; }
.dark .lottery-recharge a, .dark .lottery-chances-count, .dark .lottery-result.is-win { color: #8bd3b6; }
.dark .lottery-result { color: #d5e2da; }
.dark .lottery-prizes, .dark .lottery-account-stats, .dark .lottery-prizes li { border-color: #334155; }
@keyframes result-reveal { from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); } }
@media (max-width: 1023px) { .lottery-main { grid-template-columns: minmax(0, 1fr); gap: 20px; } .lottery-play { padding: 18px 16px; } }
@media (max-width: 480px) { .lottery-stage-heading { font-size: 13px; } .lottery-outcome { min-height: 60px; } .lottery-chances-count strong { font-size: 40px; } }
@media (prefers-reduced-motion: reduce) { .lottery-result { animation: none; } }
</style>
