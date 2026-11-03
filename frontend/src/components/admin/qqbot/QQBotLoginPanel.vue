<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700" :aria-label="t('qqbot.login.title')">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h3 class="font-medium">{{ t('qqbot.login.title') }}</h3>
      <div class="flex flex-wrap gap-2">
        <button v-if="status?.configured" type="button" class="btn btn-secondary" data-testid="qq-restart-login" :disabled="busy || loginInProgress || restartCoolingDown" @click="openRestart">{{ t('qqbot.login.restart') }}</button>
        <button type="button" class="btn btn-secondary" :disabled="busy" @click="load(false)">{{ t('qqbot.login.refreshStatus') }}</button>
      </div>
    </div>
    <p v-if="restartNotice" role="status" class="text-sm text-gray-600 dark:text-gray-300" data-testid="qq-restart-notice">{{ restartNotice }}</p>
    <p v-if="restartCoolingDown" class="text-xs text-gray-500">{{ t('qqbot.login.restartCooldown') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
    <p v-if="!status && !error" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
    <template v-if="status">
      <p v-if="!status.configured" class="text-sm text-gray-500">{{ t('qqbot.login.unconfigured') }}</p>
      <template v-else>
        <dl class="grid gap-2 text-sm sm:grid-cols-2">
          <div><dt class="text-gray-500">{{ t('qqbot.login.expected') }}</dt><dd>{{ status.expected_qq || '—' }}</dd></div>
          <div><dt class="text-gray-500">{{ t('qqbot.login.state') }}</dt><dd role="status">{{ stateLabel }}</dd></div>
          <div><dt class="text-gray-500">{{ t('qqbot.login.sessionState') }}</dt><dd data-testid="qq-session-health">{{ sessionLabel }}</dd></div>
          <div><dt class="text-gray-500">{{ t('qqbot.login.lastMessage') }}</dt><dd>{{ displayTime(status.session_health?.last_message_at) }}</dd></div>
          <div><dt class="text-gray-500">{{ t('qqbot.login.lastProbe') }}</dt><dd>{{ displayTime(status.session_health?.checked_at) }}</dd></div>
        </dl>
        <p v-if="sessionUnhealthy" role="alert" class="text-sm text-red-600 dark:text-red-400" data-testid="qq-session-alert">{{ t('qqbot.login.sessionAlert') }}</p>
        <p class="text-xs text-gray-500">{{ t('qqbot.login.receptionHelp') }}</p>
        <p v-if="status.account_mismatch" role="alert" class="text-sm text-red-600">{{ t('qqbot.login.mismatch', { qq: status.logged_in_qq }) }}</p>
        <p v-if="status.auto_recovery" class="text-sm text-gray-600 dark:text-gray-300" data-testid="login-auto-recovery">{{ t('qqbot.login.autoRecovery') }}</p>
        <p v-if="status.online && !status.connected && !status.account_mismatch" class="text-sm text-amber-700 dark:text-amber-300">{{ t('qqbot.login.connectionHelp') }}</p>
        <template v-if="!status.online">
          <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('qqbot.login.scanHelp') }}</p>
          <img v-if="qrImage" :src="qrImage" :alt="t('qqbot.login.qrAlt')" width="232" height="232" class="border border-gray-200 bg-white p-2" />
          <p v-else-if="!loginInProgress" class="text-sm text-gray-500">{{ t('qqbot.login.waitQR') }}</p>
          <button type="button" class="btn btn-secondary" :disabled="busy || loginInProgress" @click="load(true)">{{ t('qqbot.login.refreshQR') }}</button>
        </template>
        <p class="text-xs text-gray-500">{{ t('qqbot.login.pollHelp') }}</p>
      </template>
    </template>
    <ConfirmDialog :show="showRestart" :title="t('qqbot.login.restart')" :message="t('qqbot.login.restartConfirm', { qq: restartQQ })" :confirm-text="t('qqbot.login.restartConfirmButton')" :danger="true" @confirm="restartLogin" @cancel="cancelRestart" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { isAxiosError } from 'axios'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { qqbotAPI, type QQNapCatLogin } from '@/api/qqcommunity'

const { t } = useI18n()
const status = ref<QQNapCatLogin | null>(null), error = ref(''), busy = ref(false), qrImage = ref('')
const showRestart = ref(false), restartQQ = ref(''), restartNotice = ref(''), restartAvailableAt = ref(0), now = ref(Date.now())
let timer: ReturnType<typeof setTimeout> | undefined
let active = true
const controller = new AbortController()
function inLoginFlow(s: QQNapCatLogin) { return s.scan_accepted || ['initializing', 'reconnecting', 'qrcode_scanned', 'logging_in'].includes(s.phase) }
const loginInProgress = computed(() => status.value ? inLoginFlow(status.value) : false)
const restartCoolingDown = computed(() => now.value < restartAvailableAt.value)
const sessionUnhealthy = computed(() => ['degraded', 'stale', 'offline_reported'].includes(status.value?.session_health?.state || ''))
const sessionLabel = computed(() => t(`qqbot.login.session.${status.value?.session_health?.state || 'unknown'}`))
function displayTime(value?: string) { return value ? new Date(value).toLocaleString() : t('qqbot.login.noEvidence') }
const stateLabel = computed(() => {
  const s = status.value
  if (!s) return ''
  if (s.online) {
    if (s.account_mismatch) return t('qqbot.login.wrongAccount')
    if (sessionUnhealthy.value) return t('qqbot.login.sessionProblem')
    return t(s.connected ? 'qqbot.login.online' : 'qqbot.login.waitConnection')
  }
  if (s.scan_accepted || ['initializing', 'qrcode_scanned', 'logging_in'].includes(s.phase)) return t('qqbot.login.confirming')
  if (s.phase === 'reconnecting') return t('qqbot.login.reconnecting')
  if (s.login_failed) return t('qqbot.login.failed')
  return t(s.offline ? 'qqbot.login.offline' : 'qqbot.login.waitScan')
})
async function load(refresh: boolean) {
  if (!active || busy.value || showRestart.value) return
  clearTimeout(timer)
  busy.value = true
  try {
    const result = refresh ? await qqbotAPI.refreshLoginQR(controller.signal) : await qqbotAPI.login(controller.signal)
    if (!active) return
    status.value = result
    now.value = Date.now()
    if (result.restart_available_at) restartAvailableAt.value = Math.max(restartAvailableAt.value, Date.parse(result.restart_available_at) || 0)
    error.value = ''
    qrImage.value = ''
    if (result.configured && !result.online && !inLoginFlow(result) && result.qr_code) {
      const image = await QRCode.toDataURL(result.qr_code, { width: 232, margin: 2, errorCorrectionLevel: 'M' })
      if (active) qrImage.value = image
    }
  } catch {
    if (active) { error.value = t('qqbot.login.loadFailed'); status.value = null; qrImage.value = '' }
  } finally {
    if (active) {
      busy.value = false
      if (status.value?.configured !== false) schedulePoll()
    }
  }
}
function openRestart() {
  if (busy.value || loginInProgress.value || restartCoolingDown.value || !status.value?.configured) return
  clearTimeout(timer)
  restartQQ.value = status.value.expected_qq
  showRestart.value = true
}
function cancelRestart() { showRestart.value = false; if (active) schedulePoll() }
async function restartLogin() {
  if (!active || busy.value || !showRestart.value || restartCoolingDown.value) return
  showRestart.value = false
  busy.value = true
  error.value = ''
  restartNotice.value = ''
  qrImage.value = ''
  clearTimeout(timer)
  now.value = Date.now()
  restartAvailableAt.value = now.value + 60000
  try {
    const result = await qqbotAPI.restartLogin(restartQQ.value, controller.signal)
    if (!active) return
    if (!result.accepted) throw new Error('unconfirmed')
    restartAvailableAt.value = Math.max(restartAvailableAt.value, Date.parse(result.restart_available_at) || 0)
    restartNotice.value = t('qqbot.login.restartAccepted')
    status.value = null
  } catch (e) {
    if (active) error.value = isAxiosError(e) && typeof e.response?.data?.message === 'string' ? e.response.data.message : t('qqbot.login.restartFailed')
  } finally {
    if (active) { busy.value = false; schedulePoll() }
  }
}
function schedulePoll() {
  clearTimeout(timer)
  timer = setTimeout(() => {
    if (!active) return
    now.value = Date.now()
    if (document.hidden || showRestart.value) { schedulePoll(); return }
    void load(false)
  }, 5000)
}
onMounted(() => load(false))
onBeforeUnmount(() => { active = false; clearTimeout(timer); controller.abort(); qrImage.value = '' })
</script>
