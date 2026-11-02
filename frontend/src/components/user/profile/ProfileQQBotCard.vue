<template>
  <section class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium">{{ t('qqBot.profileTitle') }}</h2>
      <p class="mt-1 text-sm text-gray-500">{{ t('qqBot.profileHint') }}</p>
    </div>
    <div class="space-y-4 px-6 py-5">
      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <p v-if="status && !status.enabled" class="text-sm text-gray-500">{{ t('qqBot.disabled') }}</p>
      <template v-if="status?.binding">
        <p>{{ t('qqBot.bound', { qq: status.binding.qq }) }}</p>
        <button class="btn btn-secondary" :disabled="busy" @click="unbind">{{ t('qqBot.unbind') }}</button>
      </template>
      <template v-else-if="status?.enabled">
        <p class="text-sm">{{ t('qqBot.robot', { qq: status.bot_qq }) }}</p>
        <div v-if="code" class="space-y-2">
          <p class="text-sm">{{ t('qqBot.codeHint') }}</p>
          <code class="block select-all break-all rounded border border-gray-200 p-3 dark:border-dark-700">/绑定 {{ code }}</code>
          <p class="text-xs text-gray-500">{{ t('qqBot.expires', { time: new Date(expiresAt).toLocaleString() }) }}</p>
        </div>
        <div v-if="status.pending_qq" class="space-y-3">
          <p>{{ t('qqBot.pending', { qq: status.pending_qq }) }}</p>
          <button class="btn btn-primary" :disabled="busy" @click="confirm">{{ t('qqBot.confirm') }}</button>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-secondary" :disabled="busy" @click="createCode">{{ t(code ? 'qqBot.regenerate' : 'qqBot.createCode') }}</button>
          <button class="btn btn-secondary" :disabled="busy" @click="load">{{ t('qqBot.refresh') }}</button>
        </div>
        <p class="text-xs text-gray-500">{{ t('qqBot.privacy') }}</p>
      </template>
      <button v-if="!status" class="btn btn-secondary" :disabled="busy" @click="load">{{ t('qqBot.refresh') }}</button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { qqbotApi, type QQBindingStatus } from '@/api/qqbot'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const app = useAppStore()
const status = ref<QQBindingStatus | null>(null)
const busy = ref(false)
const error = ref('')
const code = ref('')
const expiresAt = ref('')
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await action() } catch (err) { error.value = extractApiErrorMessage(err, t('qqBot.failed')) }
  finally { busy.value = false }
}
async function load() { await run(async () => { status.value = await qqbotApi.binding() }) }
async function createCode() {
  await run(async () => {
    const result = await qqbotApi.code()
    code.value = result.code
    expiresAt.value = result.expires_at
    status.value = await qqbotApi.binding()
  })
}
async function confirm() {
  if (!status.value?.pending_qq) return
  const qq = status.value.pending_qq
  await run(async () => {
    await qqbotApi.confirm(qq)
    code.value = ''
    status.value = await qqbotApi.binding()
    app.showSuccess(t('qqBot.confirmed'))
  })
}
async function unbind() {
  if (!window.confirm(t('qqBot.unbindConfirm'))) return
  await run(async () => {
    await qqbotApi.unbind()
    code.value = ''
    status.value = await qqbotApi.binding()
    app.showSuccess(t('qqBot.unbound'))
  })
}
onMounted(load)
</script>
