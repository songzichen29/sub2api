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
              <span v-if="topPrize > 0" class="lottery-top-prize">{{ t('lotteryPortal.topPrize') }} <strong>USD {{ topPrize }}</strong></span>
            </div>
            <LotteryWheel :prizes="wheelPrizes" :rotation="rotation" :disabled="!canSpin" :spinning="spinning" :action-label="spinLabel" @spin="spin" @settled="finishSpin" />
            <div class="lottery-outcome">
              <div v-if="result && !spinning" role="status" aria-live="polite" class="lottery-result" :class="{ 'is-win': result.prize.amount > 0 }" data-testid="lottery-result">
                <span class="lottery-result-label">{{ t('lotteryPortal.result') }}<span v-if="result.is_guaranteed"> · {{ t('lotteryPortal.guaranteed') }}</span></span>
                <strong>{{ result.prize.name }}</strong>
                <span v-if="result.prize.amount > 0">+ USD {{ result.prize.amount }} · {{ t('lotteryPortal.credited') }}</span>
              </div>
              <p v-else class="lottery-idle-hint" role="status">{{ t(spinning ? 'lotteryPortal.spinningHint' : 'lotteryPortal.tapWheel') }}</p>
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
            <div v-if="data.state.guarantee_every" class="lottery-guarantee">
              <p>{{ t('lotteryPortal.guaranteeShort', { count: data.state.draws_until_guarantee }) }}</p>
              <progress class="guarantee-progress" :max="data.state.guarantee_every" :value="data.state.guarantee_every - data.state.draws_until_guarantee" :aria-label="t('lotteryPortal.guaranteeShort', { count: data.state.draws_until_guarantee })" />
            </div>
            <div class="lottery-prizes">
              <div class="flex items-center justify-between gap-3"><h2>{{ t(isGuaranteePool ? 'lotteryPortal.guaranteePool' : 'lotteryPortal.normalPool') }}</h2><span class="text-xs text-gray-500 dark:text-gray-400">{{ t('lotteryPortal.probability') }}</span></div>
              <ul>
                <li v-for="(prize, index) in nextPool" :key="index">
                  <span class="lottery-prize-name"><span class="lottery-prize-dot" :class="{ 'is-top': prize.amount > 0 && prize.amount === nextTopPrize }" aria-hidden="true"></span>{{ prize.name }}</span>
                  <span class="lottery-prize-value"><strong>{{ prize.amount > 0 ? `USD ${prize.amount}` : '—' }}</strong><small>{{ (prize.probability_bps / 100).toFixed(2) }}%</small></span>
                </li>
              </ul>
            </div>
            <dl class="lottery-account-stats">
              <div><dt>{{ t('lotteryPortal.recharge') }}</dt><dd>USD {{ data.state.recharge_total.toFixed(2) }}</dd></div>
              <div><dt>{{ t('lotteryPortal.earned') }}</dt><dd>{{ data.state.earned }}</dd></div>
              <div><dt>{{ t('lotteryPortal.used') }}</dt><dd>{{ data.state.used }}</dd></div>
            </dl>
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
          <p v-if="data.state.guarantee_every" class="text-sm">{{ t('lotteryPortal.guarantee', { count: data.state.draws_until_guarantee, every: data.state.guarantee_every }) }}</p>
          <p class="text-sm">{{ t('lotteryPortal.period', { time: date(data.period_start) }) }}</p>
          <p class="text-sm text-gray-500">{{ t('lotteryPortal.accounting') }}</p>
          <div v-for="pool in pools" :key="pool.name" class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <caption class="py-3 text-left font-medium">{{ t(pool.name) }}</caption>
              <thead><tr class="border-b dark:border-dark-700"><th class="py-2 font-medium">{{ t('lotteryPortal.prize') }}</th><th class="font-medium">{{ t('lotteryPortal.amount') }}</th><th class="font-medium">{{ t('lotteryPortal.probability') }}</th></tr></thead>
              <tbody><tr v-for="(prize, i) in pool.prizes" :key="i" class="border-b dark:border-dark-700"><td class="py-2">{{ prize.name }}</td><td>{{ prize.amount }}</td><td>{{ (prize.probability_bps / 100).toFixed(2) }}%</td></tr></tbody>
            </table>
          </div>
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
const isGuaranteePool = computed(() => data.value?.state.draws_until_guarantee === 1 && !!data.value.guarantee_prizes.length)
const nextPool = computed(() => (isGuaranteePool.value ? data.value?.guarantee_prizes : data.value?.prizes) || [])
const wheelPrizes = computed(() => result.value?.prizes || nextPool.value)
const topPrize = computed(() => Math.max(0, ...wheelPrizes.value.map(prize => prize.amount)))
const nextTopPrize = computed(() => Math.max(0, ...nextPool.value.map(prize => prize.amount)))
const spinLabel = computed(() => t(spinning.value ? 'lotteryPortal.spinning' : requestID.value ? 'lotteryPortal.retrySpin' : data.value?.state.remaining ? 'lotteryPortal.spin' : 'lotteryPortal.noChances'))
const canSpin = computed(() => !spinning.value && !loading.value && !!data.value && (!!requestID.value || (data.value.enabled && data.value.state.remaining > 0 && nextPool.value.length > 0)))
let spinTimer: ReturnType<typeof setTimeout> | undefined
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
.lottery-main { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(340px, 1fr); gap: 32px; align-items: start; }
.lottery-play { min-width: 0; padding: 20px 24px 24px; border: 1px solid #e4e8df; border-radius: 8px; background: #f8f8f1; text-align: center; }
.lottery-stage-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; color: #536557; font-size: 14px; text-align: left; }
.lottery-top-prize { font-size: 12px; }
.lottery-top-prize strong { margin-left: 6px; color: #34533d; font-size: 16px; font-weight: 600; }
.lottery-outcome { min-height: 76px; margin-top: 8px; }
.lottery-idle-hint { color: #34533d; font-size: 14px; }
.lottery-odds-note { margin-top: 8px; color: #738071; font-size: 12px; }
.lottery-result { display: flex; flex-direction: column; gap: 3px; color: #536557; font-size: 13px; animation: result-reveal 180ms ease-out; }
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
.lottery-guarantee { margin-top: 20px; padding: 14px 16px; border-radius: 6px; background: #f0f7f3; color: #21634d; font-size: 13px; }
.lottery-guarantee p { margin-bottom: 9px; }
.lottery-prizes { margin-top: 24px; padding-top: 20px; border-top: 1px solid #e5e7eb; }
.lottery-prizes ul { margin-top: 8px; }
.lottery-prizes li { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 13px 0; border-bottom: 1px solid #f1f3f1; }
.lottery-prizes li:last-child { border-bottom: 0; }
.lottery-prize-name { display: flex; align-items: center; gap: 10px; min-width: 0; overflow-wrap: anywhere; font-size: 14px; }
.lottery-prize-dot { width: 8px; height: 8px; flex-shrink: 0; border-radius: 2px; background: #aac9b9; }
.lottery-prize-dot.is-top { background: #c29a4e; }
.lottery-prize-value { display: flex; flex-direction: column; align-items: flex-end; flex-shrink: 0; font-variant-numeric: tabular-nums; }
.lottery-prize-value strong { font-size: 15px; font-weight: 500; }
.lottery-prize-value small { color: #6b7280; font-size: 12px; }
.lottery-account-stats { display: flex; justify-content: space-between; gap: 12px; margin-top: 20px; padding-top: 18px; border-top: 1px solid #e5e7eb; font-size: 12px; }
.lottery-account-stats dt { color: #6b7280; }
.lottery-account-stats dd { margin-top: 5px; font-size: 14px; font-variant-numeric: tabular-nums; }
.guarantee-progress { display: block; width: 100%; height: 6px; border: 0; border-radius: 3px; overflow: hidden; background: #e5e7eb; }
.guarantee-progress::-webkit-progress-bar { background: #e5e7eb; }
.guarantee-progress::-webkit-progress-value { background: #0d9488; }
.guarantee-progress::-moz-progress-bar { background: #0d9488; }
.dark .lottery-play { background: #17251f; border-color: #34463a; }
.dark .lottery-stage-heading, .dark .lottery-idle-hint, .dark .lottery-top-prize strong { color: #dae7da; }
.dark .lottery-odds-note, .dark .lottery-chances p, .dark .lottery-recharge, .dark .lottery-account-stats dt, .dark .lottery-prize-value small { color: #a0b0ab; }
.dark .lottery-recharge a, .dark .lottery-chances-count, .dark .lottery-result.is-win { color: #8bd3b6; }
.dark .lottery-result { color: #d5e2da; }
.dark .lottery-guarantee { background: #183c31; color: #a9d6be; }
.dark .lottery-prizes, .dark .lottery-account-stats, .dark .lottery-prizes li { border-color: #334155; }
@keyframes result-reveal { from { opacity: 0; } to { opacity: 1; } }
@media (max-width: 1023px) { .lottery-main { grid-template-columns: minmax(0, 1fr); gap: 20px; } .lottery-play { padding: 18px 16px; } }
@media (max-width: 480px) { .lottery-stage-heading { font-size: 13px; } .lottery-top-prize strong { font-size: 14px; } .lottery-outcome { min-height: 60px; } .lottery-chances-count strong { font-size: 40px; } }
@media (prefers-reduced-motion: reduce) { .lottery-result { animation: none; } }
</style>
