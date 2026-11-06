<template>
  <AppLayout>
    <div class="workspace-page checkin-page">
      <header class="checkin-heading">
        <div><h1>{{ t('checkin.title') }}</h1><p>{{ t('checkin.reset') }}</p></div>
        <button class="btn btn-secondary" data-testid="refresh" :disabled="loading || claiming" @click="refresh">{{ t('checkin.refresh') }}</button>
      </header>
      <p v-if="error" class="checkin-error" role="alert">{{ error }}</p>
      <p v-if="!data && loading" role="status">{{ t('checkin.loading') }}</p>
      <template v-if="data">
        <p v-if="!data.enabled" class="checkin-notice" role="status">{{ t('checkin.disabled') }}</p>
        <div class="checkin-main">
          <section class="checkin-calendar" :aria-label="t('checkin.calendar')" :aria-busy="loading">
            <div class="calendar-toolbar">
              <h2>{{ monthLabel }}</h2>
              <div class="calendar-controls">
                <button class="calendar-arrow" data-testid="previous-month" :disabled="claiming || data.month <= '2000-01'" :aria-label="t('checkin.previousMonth')" @click="moveMonth(-1)">‹</button>
                <button class="calendar-current" :disabled="claiming || data.month === data.today.slice(0, 7)" @click="thisMonth">{{ t('checkin.currentMonth') }}</button>
                <button class="calendar-arrow" data-testid="next-month" :disabled="claiming || data.month >= data.today.slice(0, 7)" :aria-label="t('checkin.nextMonth')" @click="moveMonth(1)">›</button>
              </div>
            </div>
            <div class="calendar-week" aria-hidden="true"><span v-for="weekday in weekdays" :key="weekday">{{ weekday }}</span></div>
            <ol class="calendar-days">
              <li v-for="cell in cells" :key="cell.key" :data-date="cell.date || undefined" :class="{ 'is-empty': !cell.date, 'is-signed': !!cell.record, 'is-today': cell.date === data.today, 'is-future': cell.date > data.today }" :aria-current="cell.date === data.today ? 'date' : undefined" :aria-label="cell.date ? cellLabel(cell) : undefined" :title="cell.date ? cellLabel(cell) : undefined" :aria-hidden="!cell.date || undefined">
                <template v-if="cell.date">
                  <span class="calendar-number">{{ cell.number }}</span>
                  <svg v-if="cell.record" class="calendar-check" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m5 12 4 4L19 6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                  <span v-else-if="cell.date === data.today" class="calendar-today">{{ t('checkin.today') }}</span>
                  <span v-else class="calendar-placeholder" aria-hidden="true">{{ cell.date < data.today ? '—' : '' }}</span>
                </template>
              </li>
            </ol>
            <div class="calendar-footer"><span>{{ t('checkin.monthCount', { count: data.days.length }) }}</span><span class="calendar-legend"><span aria-hidden="true">✓</span> {{ t('checkin.signed') }}</span></div>
          </section>

          <section class="card checkin-account" :aria-busy="claiming">
            <div class="checkin-today-heading"><h2>{{ t('checkin.dailyReward') }}</h2><time :datetime="data.today">{{ data.today }}</time></div>
            <p class="checkin-amount-label">{{ t(data.checked_in ? 'checkin.earnedToday' : data.min_amount === data.max_amount ? 'checkin.fixedReward' : 'checkin.rewardRange') }}</p>
            <div class="checkin-amount" data-testid="reward-amount">{{ data.checked_in ? amount(data.today_amount) : rewardRange }}</div>
            <button class="btn btn-primary checkin-claim" data-testid="checkin-claim" :disabled="!canClaim" @click="claim">
              <span v-if="data.checked_in" aria-hidden="true">✓ </span>{{ claimLabel }}
            </button>
            <div class="checkin-feedback" aria-live="polite">
              <p v-if="claimError" class="checkin-error" role="alert">{{ claimError }}</p>
              <p v-else-if="data.checked_in" class="checkin-success">{{ t(data.today_source === 'qq' ? 'checkin.viaQQ' : 'checkin.viaWebsite') }}<span v-if="data.today_amount > 0"> · {{ t('checkin.credited') }}</span><span v-else> · {{ t('checkin.zeroReward') }}</span></p>
              <p v-else-if="!data.eligible" class="checkin-ineligible">{{ t('checkin.ineligible') }}<RouterLink to="/purchase">{{ t('checkin.recharge') }} →</RouterLink></p>
              <p v-else>{{ t('checkin.shared') }}</p>
            </div>

            <div class="checkin-streak">
              <div class="streak-heading"><h2>{{ t('checkin.streak') }}</h2><span><strong data-testid="streak-count">{{ data.streak_days }}</strong> {{ t('checkin.dayUnit') }}</span></div>
              <template v-if="data.streak_reward">
                <p class="streak-reward">{{ t('checkin.bonusEvery', { count: data.streak_interval }) }}<span>{{ rewardText(data.streak_reward) }}</span></p>
                <progress :value="streakProgress" :max="data.streak_interval" :aria-label="t('checkin.bonusIn', { count: daysUntilBonus })" />
                <p class="streak-next">{{ t('checkin.bonusIn', { count: daysUntilBonus }) }}</p>
              </template>
              <div v-if="data.today_bonus" class="checkin-bonus" role="status">
                <p>{{ t('checkin.bonusReceived') }} · {{ rewardText(data.today_bonus.reward) }}</p>
                <p v-if="data.today_bonus.status === 'pending'">{{ t('checkin.pending') }} · {{ data.today_bonus.reward.claim_instructions || t('checkin.contact') }}</p>
              </div>
            </div>
            <dl class="checkin-totals"><div><dt>{{ t('checkin.totalDays') }}</dt><dd>{{ data.total_days }} {{ t('checkin.dayUnit') }}</dd></div><div><dt>{{ t('checkin.totalAmount') }}</dt><dd>USD {{ amount(data.total_amount) }}</dd></div></dl>
            <RouterLink class="checkin-wheel-link" to="/lottery">{{ t('checkin.wheel') }} <span aria-hidden="true">→</span></RouterLink>
          </section>
        </div>
        <details class="checkin-rules">
          <summary>{{ t('checkin.rules') }}</summary>
          <div class="rules-content"><p>{{ t('checkin.ruleDaily') }}</p><p>{{ t('checkin.ruleReset') }}</p><p>{{ t('checkin.ruleAmount') }}</p><p v-if="data.streak_reward">{{ t('checkin.ruleBonus') }}</p></div>
        </details>
        <section v-if="data.min_recharge_total > 0 || data.min_recharge_30d > 0" class="checkin-eligibility">
          <h2>{{ t('checkin.eligibility') }}</h2><span v-if="data.min_recharge_total > 0 && data.min_recharge_30d > 0">{{ t('checkin.either') }}</span>
          <p v-if="data.min_recharge_total > 0">{{ t('checkin.lifetime', { amount: amount(data.min_recharge_total) }) }}</p>
          <p v-if="data.min_recharge_30d > 0">{{ t('checkin.recent', { days: data.recharge_window_days, amount: amount(data.min_recharge_30d) }) }}</p>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { checkinAPI, type CheckinDay, type UserCheckin } from '@/api/checkin'
import type { QQGrowthReward } from '@/api/qqcommunity'
import { useAuthStore } from '@/stores/auth'

const { t, tm, locale } = useI18n()
const auth = useAuthStore()
const data = ref<UserCheckin | null>(null)
const loading = ref(false), claiming = ref(false)
const error = ref(''), claimError = ref(''), selectedMonth = ref(''), pendingDay = ref('')
let version = 0, stopped = false, resetTimer: ReturnType<typeof setTimeout> | undefined
const weekdays = computed(() => tm('checkin.weekdays') as string[])
const monthLabel = computed(() => data.value ? new Intl.DateTimeFormat(locale.value, { year: 'numeric', month: 'long', timeZone: 'UTC' }).format(new Date(`${data.value.month}-01T00:00:00Z`)) : '')
const amount = (value: number) => new Intl.NumberFormat(locale.value, { minimumFractionDigits: 2, maximumFractionDigits: 8 }).format(value)
const rewardRange = computed(() => !data.value ? '' : data.value.min_amount === data.value.max_amount ? amount(data.value.min_amount) : `${amount(data.value.min_amount)} – ${amount(data.value.max_amount)}`)
const daysUntilBonus = computed(() => !data.value ? 0 : data.value.streak_interval - data.value.streak_days % data.value.streak_interval)
const streakProgress = computed(() => !data.value ? 0 : data.value.checked_in && data.value.streak_days > 0 && data.value.streak_days % data.value.streak_interval === 0 ? data.value.streak_interval : data.value.streak_days % data.value.streak_interval)
const canClaim = computed(() => !!data.value && !loading.value && !claiming.value && !error.value && (data.value.can_checkin || !!pendingDay.value) && !data.value.checked_in)
const claimLabel = computed(() => t(claiming.value ? 'checkin.claiming' : data.value?.checked_in ? 'checkin.done' : pendingDay.value ? 'checkin.retry' : 'checkin.claim'))
interface CalendarCell { key: string; date: string; number: number; record?: CheckinDay }
const cells = computed<CalendarCell[]>(() => {
  if (!data.value) return []
  const [year, month] = data.value.month.split('-').map(Number) as [number, number]
  const offset = (new Date(Date.UTC(year, month - 1, 1)).getUTCDay() + 6) % 7
  const length = new Date(Date.UTC(year, month, 0)).getUTCDate()
  const records = new Map(data.value.days.map(record => [record.date, record]))
  return Array.from({ length: Math.ceil((offset + length) / 7) * 7 }, (_, index) => {
    const number = index - offset + 1
    const date = number < 1 || number > length ? '' : `${data.value!.month}-${String(number).padStart(2, '0')}`
    return { key: `${data.value!.month}-${index}`, date, number, record: records.get(date) }
  })
})
function cellLabel(cell: CalendarCell) {
  const status = cell.record ? `${t('checkin.signed')} · USD ${amount(cell.record.amount)} · ${t(cell.record.source === 'qq' ? 'checkin.viaQQ' : 'checkin.viaWebsite')}` : t(cell.date > data.value!.today ? 'checkin.future' : 'checkin.missed')
  return `${cell.date} · ${status}`
}
function rewardText(reward: QQGrowthReward) {
  if (reward.type === 'balance') return t('checkin.balance', { amount: amount(reward.amount) })
  if (reward.type === 'chances') return t('checkin.chances', { count: reward.quantity })
  return t('checkin.item', { name: reward.prize_name, count: reward.quantity })
}
async function load() {
  if (stopped) return
  const id = ++version
  loading.value = true; error.value = ''; clearTimeout(resetTimer)
  try {
    const value = await checkinAPI.overview(selectedMonth.value)
    if (stopped || id !== version) return
    if (pendingDay.value && pendingDay.value !== value.today) { pendingDay.value = ''; claimError.value = t('checkin.dayChanged') }
    if (value.checked_in && pendingDay.value === value.today) { pendingDay.value = ''; claimError.value = ''; void auth.refreshUser().catch(() => {}) }
    data.value = value
    // Base the timer on the server's clock, independent of the browser's zone.
    resetTimer = setTimeout(refresh, Math.max(1000, new Date(value.next_reset_at).getTime() - new Date(value.server_time).getTime() + 500))
  } catch { if (!stopped && id === version) error.value = t('checkin.loadFailed') }
  finally { if (id === version) loading.value = false }
}
function refresh() { if (!claiming.value) void load() }
function moveMonth(delta: number) {
  if (!data.value || claiming.value) return
  const [year, month] = (selectedMonth.value || data.value.month).split('-').map(Number) as [number, number]
  const target = new Date(Date.UTC(year, month - 1 + delta, 1)).toISOString().slice(0, 7)
  if (target < '2000-01' || target > data.value.today.slice(0, 7)) return
  selectedMonth.value = target === data.value.today.slice(0, 7) ? '' : target
  void load()
}
function thisMonth() { selectedMonth.value = ''; void load() }
async function claim() {
  if (!canClaim.value || !data.value) return
  const date = pendingDay.value || data.value.today
  pendingDay.value = date; claiming.value = true; claimError.value = ''; clearTimeout(resetTimer)
  try {
    const result = await checkinAPI.claim(date)
    if (stopped) return
    pendingDay.value = ''
    if (data.value.today === result.date) {
      data.value.checked_in = true; data.value.can_checkin = false
      data.value.today_amount = result.amount; data.value.today_source = result.source; data.value.today_bonus = result.bonus
    }
    void auth.refreshUser().catch(() => {})
    await load()
  } catch (e) {
    if (stopped) return
    const failure = e as { status?: number; message?: string }
    if (failure.status === 400 || failure.status === 403) {
      pendingDay.value = ''; claimError.value = failure.message || t('checkin.claimFailed'); await load()
    } else { claimError.value = t('checkin.claimFailed') }
  } finally { if (!stopped) claiming.value = false }
}
onMounted(() => { void load(); window.addEventListener('focus', refresh) })
onBeforeUnmount(() => { stopped = true; version++; clearTimeout(resetTimer); window.removeEventListener('focus', refresh) })
</script>

<style scoped>
.checkin-heading { display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-bottom: 24px; }
.checkin-heading p { margin-top: 5px; color: #6b7280; font-size: 13px; }
.checkin-main { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(340px, 1fr); gap: 32px; align-items: start; }
.checkin-calendar { min-width: 0; padding: 24px; border: 1px solid #e4e8df; border-radius: 8px; background: #f8f8f1; }
.calendar-toolbar, .calendar-controls { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.calendar-toolbar h2 { font-size: 20px; color: #34533d; }
.calendar-controls { gap: 4px; }
.calendar-arrow, .calendar-current { height: 36px; border-radius: 6px; color: #536557; }
.calendar-arrow { width: 36px; font-size: 26px; line-height: 1; }
.calendar-current { padding: 0 8px; font-size: 12px; }
.calendar-controls button:hover:not(:disabled) { background: #e8eee3; }
.calendar-controls button:disabled { opacity: .4; cursor: default; }
.calendar-controls button:focus-visible, .checkin-rules summary:focus-visible { outline: 2px solid #0f766e; outline-offset: 3px; }
.calendar-week, .calendar-days { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 8px; }
.calendar-week { margin: 24px 0 12px; color: #738071; font-size: 12px; text-align: center; }
.calendar-days { list-style: none; padding: 0; margin: 0; }
.calendar-days li { min-height: 76px; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; border: 1px solid #e4e8df; border-radius: 6px; background: #fdfdf9; color: #536557; font-variant-numeric: tabular-nums; }
.calendar-number { font-size: 18px; font-weight: 500; line-height: 1.4; }
.calendar-days .is-empty { border-color: transparent; background: transparent; }
.calendar-days .is-future { color: #899285; background: transparent; border-color: #ebede4; }
.calendar-days .is-signed { background: #e2eee5; border-color: #c9decd; color: #21634d; }
.calendar-days .is-today { border: 2px solid #b18b43; background: #fff8e8; color: #805e23; }
.calendar-check { width: 18px; height: 18px; }
.calendar-today, .calendar-placeholder { height: 18px; font-size: 11px; }
.calendar-placeholder { color: #b2b8a8; }
.calendar-footer { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; margin-top: 20px; color: #536557; font-size: 12px; }
.calendar-legend { color: #21634d; }
.calendar-legend > span { margin-right: 3px; }
.checkin-account { padding: 24px; min-width: 0; }
.checkin-today-heading, .streak-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.checkin-today-heading time { font-size: 12px; color: #6b7280; font-variant-numeric: tabular-nums; }
.checkin-amount-label { margin-top: 24px; color: #6b7280; font-size: 12px; }
.checkin-amount { margin-top: 4px; font-size: clamp(24px, 2.6vw, 36px); line-height: 1.35; font-weight: 600; color: #0f766e; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.checkin-page .checkin-claim { width: 100%; min-height: 46px; margin-top: 24px; font-size: 16px; }
.checkin-feedback { margin-top: 12px; font-size: 12px; color: #6b7280; }
.checkin-success { color: #21634d; }
.checkin-ineligible a { margin-left: 8px; color: #0f766e; white-space: nowrap; }
.checkin-streak { margin-top: 24px; padding-top: 20px; border-top: 1px solid #e5e7eb; }
.streak-heading > span { color: #6b7280; font-size: 12px; }
.streak-heading strong { color: #34533d; font-size: 28px; font-weight: 600; margin-right: 4px; }
.streak-reward { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; margin: 12px 0; color: #6b7280; font-size: 12px; }
.streak-reward span { color: #805e23; font-weight: 500; overflow-wrap: anywhere; }
.checkin-streak progress { display: block; width: 100%; height: 6px; border: 0; border-radius: 3px; overflow: hidden; background: #e5e7eb; }
.checkin-streak progress::-webkit-progress-bar { background: #e5e7eb; }
.checkin-streak progress::-webkit-progress-value { background: #0f766e; }
.checkin-streak progress::-moz-progress-bar { background: #0f766e; }
.streak-next { margin-top: 8px; color: #6b7280; font-size: 12px; }
.checkin-bonus { margin-top: 12px; padding: 12px; border-radius: 6px; background: #f0f7f3; color: #21634d; font-size: 13px; overflow-wrap: anywhere; }
.checkin-bonus p + p { margin-top: 4px; }
.checkin-totals { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 16px; margin-top: 24px; padding-top: 18px; border-top: 1px solid #e5e7eb; font-size: 12px; }
.checkin-totals dt { color: #6b7280; }
.checkin-totals dd { margin-top: 4px; font-size: 14px; font-variant-numeric: tabular-nums; }
.checkin-wheel-link { display: flex; justify-content: space-between; align-items: center; margin-top: 20px; color: #0f766e; font-weight: 500; }
.checkin-rules { margin-top: 24px; border-top: 1px solid #e5e7eb; }
.checkin-rules summary { cursor: pointer; padding: 16px 0; }
.rules-content { display: grid; gap: 8px; padding-bottom: 16px; color: #6b7280; font-size: 13px; }
.checkin-eligibility { display: flex; flex-wrap: wrap; align-items: baseline; gap: 8px 20px; color: #6b7280; font-size: 13px; }
.checkin-eligibility h2 { color: #374151; font-size: 14px; }
.checkin-error { color: #b91c1c; font-size: 13px; }
.checkin-heading + .checkin-error, .checkin-notice { margin-bottom: 16px; }
.checkin-notice { padding: 12px 16px; border: 1px solid #e5e7eb; border-radius: 6px; color: #6b7280; }
.dark .checkin-calendar { background: #17251f; border-color: #34463a; }
.dark .calendar-toolbar h2, .dark .calendar-controls button, .dark .calendar-footer, .dark .streak-heading strong { color: #dae7da; }
.dark .calendar-week, .dark .checkin-heading p, .dark .checkin-today-heading time, .dark .checkin-amount-label, .dark .checkin-feedback, .dark .streak-heading > span, .dark .streak-reward, .dark .streak-next, .dark .checkin-totals dt, .dark .rules-content, .dark .checkin-eligibility, .dark .checkin-notice { color: #a0b0ab; }
.dark .calendar-controls button:hover:not(:disabled) { background: #34463a; }
.dark .calendar-days li { background: #213127; border-color: #34463a; color: #c9d6cd; }
.dark .calendar-days .is-future { color: #8b9a90; background: transparent; border-color: #2a3a30; }
.dark .calendar-days .is-empty { background: transparent; border-color: transparent; }
.dark .calendar-days .is-signed { background: #244f3e; color: #b2dfc7; border-color: #37634e; }
.dark .calendar-days .is-today { background: #3b3425; border-color: #c9a45c; color: #f0d49a; }
.dark .calendar-placeholder { color: #718078; }
.dark .checkin-amount, .dark .checkin-success, .dark .calendar-legend, .dark .checkin-wheel-link, .dark .checkin-ineligible a { color: #8bd3b6; }
.dark .streak-reward span { color: #e2c58d; }
.dark .checkin-bonus { background: #183c31; color: #a9d6be; }
.dark .checkin-streak, .dark .checkin-totals, .dark .checkin-rules, .dark .checkin-notice { border-color: #334155; }
.dark .checkin-eligibility h2 { color: #dae7da; }
.dark .checkin-error { color: #fca5a5; }
@media (max-width: 1023px) { .checkin-main { grid-template-columns: minmax(0, 1fr); gap: 20px; } .checkin-account { order: -1; } .checkin-heading { margin-bottom: 20px; } .checkin-amount { font-size: 32px; } }
@media (max-width: 480px) {
  .checkin-calendar, .checkin-account { padding: 18px 16px; }
  .calendar-toolbar { gap: 8px; } .calendar-toolbar h2 { font-size: 18px; }
  .calendar-week, .calendar-days { gap: 4px; } .calendar-days li { min-height: 58px; }
  .calendar-number { font-size: 16px; } .calendar-check { width: 16px; height: 16px; }
  .calendar-controls { gap: 0; } .calendar-current { padding: 0 4px; }
  .calendar-arrow { width: 30px; } .calendar-footer { margin-top: 16px; }
  .checkin-amount-label { margin-top: 16px; } .checkin-amount { font-size: 28px; }
  .checkin-page .checkin-claim { margin-top: 16px; }
  .checkin-streak { margin-top: 18px; padding-top: 14px; } .checkin-totals { margin-top: 16px; padding-top: 14px; }
}
</style>
