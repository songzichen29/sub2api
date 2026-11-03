<template>
  <section class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700" :aria-label="t('qqbot.login.title')">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h3 class="font-medium">{{ t('qqbot.login.title') }}</h3>
      <button type="button" class="btn btn-secondary" :disabled="busy" @click="load(false)">{{ t('qqbot.login.refreshStatus') }}</button>
    </div>
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
          <p v-else-if="!status.scan_accepted" class="text-sm text-gray-500">{{ t('qqbot.login.waitQR') }}</p>
          <button type="button" class="btn btn-secondary" :disabled="busy || status.scan_accepted || status.phase === 'reconnecting' || status.phase === 'initializing'" @click="load(true)">{{ t('qqbot.login.refreshQR') }}</button>
        </template>
        <p class="text-xs text-gray-500">{{ t('qqbot.login.pollHelp') }}</p>
      </template>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { qqbotAPI, type QQNapCatLogin } from '@/api/qqcommunity'

const { t } = useI18n()
const status = ref<QQNapCatLogin | null>(null), error = ref(''), busy = ref(false), qrImage = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let active = true
const controller = new AbortController()
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
  if (s.scan_accepted || s.phase === 'initializing') return t('qqbot.login.confirming')
  if (s.phase === 'reconnecting') return t('qqbot.login.reconnecting')
  if (s.login_failed) return t('qqbot.login.failed')
  return t(s.offline ? 'qqbot.login.offline' : 'qqbot.login.waitScan')
})
async function load(refresh: boolean) {
  if (!active || busy.value) return
  clearTimeout(timer)
  busy.value = true
  try {
    const result = refresh ? await qqbotAPI.refreshLoginQR(controller.signal) : await qqbotAPI.login(controller.signal)
    if (!active) return
    status.value = result
    error.value = ''
    qrImage.value = ''
    if (result.configured && !result.online && !result.scan_accepted && result.phase !== 'initializing' && result.phase !== 'reconnecting' && result.qr_code) {
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
function schedulePoll() {
  timer = setTimeout(() => {
    if (!active) return
    if (document.hidden) { schedulePoll(); return }
    void load(false)
  }, 5000)
}
onMounted(() => load(false))
onBeforeUnmount(() => { active = false; clearTimeout(timer); controller.abort(); qrImage.value = '' })
</script>
