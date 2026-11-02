<template>
  <AppLayout>
    <div class="space-y-6">
      <header class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 class="text-xl font-semibold">{{ t('qqBot.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500">{{ t('qqBot.description') }}</p>
        </div>
        <button class="btn btn-secondary" :disabled="busy || recordsBusy" @click="refresh">{{ t('qqBot.refresh') }}</button>
      </header>

      <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
      <section class="card space-y-3 p-5" aria-live="polite">
        <p class="text-sm">
          {{ t('qqBot.connection') }}：
          <span :class="connection?.connected ? 'text-green-600' : 'text-gray-500'">{{ t(connection?.connected ? 'qqBot.connected' : 'qqBot.disconnected') }}</span>
          <span v-if="connection?.connected_since" class="ml-2 text-gray-500">{{ new Date(connection.connected_since).toLocaleString() }}</span>
        </p>
        <p class="text-xs text-gray-500">{{ t('qqBot.connectionHint') }}</p>
        <label for="qq-ws" class="input-label">{{ t('qqBot.wsURL') }}</label>
        <input id="qq-ws" :value="wsURL" class="input font-mono text-sm" readonly />
        <p class="text-xs text-gray-500">{{ t('qqBot.wsHint') }}</p>
      </section>

      <form v-if="config" class="space-y-6" @submit.prevent="save">
        <fieldset class="card space-y-5 p-5" :disabled="busy">
          <label class="flex items-center gap-2"><input v-model="config.enabled" type="checkbox" />{{ t('qqBot.enabled') }}</label>
          <div class="grid gap-5 md:grid-cols-2">
            <div>
              <label for="qq-number" class="input-label">{{ t('qqBot.botQQ') }}</label>
              <input id="qq-number" v-model.trim="config.bot_qq" class="input" inputmode="numeric" maxlength="20" />
            </div>
            <div>
              <label for="qq-groups" class="input-label">{{ t('qqBot.groups') }}</label>
              <textarea id="qq-groups" v-model="groupsText" class="input" rows="3" />
              <p class="mt-1 text-xs text-gray-500">{{ t('qqBot.groupsHint') }}</p>
            </div>
          </div>
          <div>
            <label for="qq-token" class="input-label">{{ t('qqBot.token') }} · {{ t(config.access_token_set ? 'qqBot.configured' : 'qqBot.notConfigured') }}</label>
            <input id="qq-token" v-model="config.access_token" class="input" type="password" autocomplete="new-password" :disabled="config.clear_access_token" maxlength="4096" />
            <p class="mt-1 text-xs text-gray-500">{{ t('qqBot.tokenHint') }}</p>
            <label v-if="config.access_token_set" class="mt-2 flex items-center gap-2 text-sm"><input v-model="config.clear_access_token" type="checkbox" />{{ t('qqBot.clearToken') }}</label>
          </div>
          <div>
            <div class="grid max-w-xl gap-4 sm:grid-cols-2">
              <div>
                <label for="qq-reward" class="input-label">{{ t('qqBot.reward') }}</label>
                <input id="qq-reward" v-model.number="config.checkin_reward" type="number" min="0" max="100" step="0.00000001" class="input" required />
              </div>
              <div>
                <label for="qq-reward-max" class="input-label">{{ t('qqBot.rewardMax') }}</label>
                <input id="qq-reward-max" v-model.number="config.checkin_reward_max" type="number" min="0" max="100" step="0.00000001" class="input" required />
              </div>
            </div>
            <p class="mt-1 text-xs text-gray-500">{{ t('qqBot.rewardHint') }}</p>
          </div>
        </fieldset>

        <fieldset class="card space-y-5 p-5" :disabled="busy">
          <label class="flex items-center gap-2"><input v-model="config.chat_enabled" type="checkbox" />{{ t('qqBot.chatEnabled') }}</label>
          <div>
            <label for="qq-key" class="input-label">{{ t('qqBot.chatKey') }} · {{ t(config.api_key_set ? 'qqBot.configured' : 'qqBot.notConfigured') }}</label>
            <input id="qq-key" v-model="config.api_key" type="password" autocomplete="new-password" class="input" :disabled="config.clear_api_key" maxlength="4096" />
            <p class="mt-1 text-xs text-gray-500">{{ t('qqBot.chatKeyHint') }}</p>
            <label v-if="config.api_key_set" class="mt-2 flex items-center gap-2 text-sm"><input v-model="config.clear_api_key" type="checkbox" />{{ t('qqBot.clearKey') }}</label>
          </div>
          <div>
            <label for="qq-model" class="input-label">{{ t('qqBot.model') }}</label>
            <input id="qq-model" v-model.trim="config.chat_model" class="input" maxlength="200" />
          </div>
          <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <div v-for="field in numberFields" :key="field.key">
              <label :for="`qq-${field.key}`" class="input-label">{{ t(`qqBot.${field.label}`) }}</label>
              <input :id="`qq-${field.key}`" v-model.number="config[field.key]" type="number" :min="field.min" :max="field.max" step="1" class="input" required />
            </div>
          </div>
          <div>
            <label for="qq-prompt" class="input-label">{{ t('qqBot.prompt') }}</label>
            <textarea id="qq-prompt" v-model="config.system_prompt" rows="4" maxlength="8000" class="input" />
          </div>
          <p class="text-xs text-gray-500">{{ t('qqBot.quotaHint') }}</p>
          <button type="button" class="btn btn-secondary" :disabled="!config.chat_model || (!config.api_key && !config.api_key_set)" @click="testModel">{{ t('qqBot.test') }}</button>
          <p v-if="testReply" role="status" class="text-sm text-green-600">{{ t('qqBot.testSuccess', { reply: testReply }) }}</p>
        </fieldset>
        <button class="btn btn-primary" type="submit" :disabled="busy">{{ busy ? t('qqBot.loading') : t('qqBot.save') }}</button>
      </form>

      <section class="card p-5">
        <h2 class="font-medium">{{ t('qqBot.commands') }}</h2>
        <p class="mt-2 text-sm text-gray-500">{{ t('qqBot.commandHelp') }}</p>
      </section>

      <section class="card overflow-hidden">
        <div class="flex gap-5 overflow-x-auto border-b border-gray-200 px-5 dark:border-dark-700" role="tablist">
          <button v-for="kind in recordKinds" :key="kind" type="button" role="tab" :aria-selected="recordKind === kind" :disabled="recordsBusy" class="border-b-2 py-3 text-sm" :class="recordKind === kind ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" @click="selectRecords(kind)">{{ t(`qqBot.${kind}`) }}</button>
        </div>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead class="border-b border-gray-100 dark:border-dark-700"><tr><th v-for="column in columns" :key="column" class="whitespace-nowrap px-4 py-3 font-medium">{{ t(`qqBot.${column}`) }}</th><th v-if="recordKind === 'bindings'" class="px-4 py-3 font-medium">{{ t('qqBot.actions') }}</th></tr></thead>
            <tbody>
              <tr v-for="(row, index) in records.items" :key="'id' in row ? row.id : index" class="border-b border-gray-100 dark:border-dark-700">
                <td v-for="column in columns" :key="column" class="max-w-md whitespace-pre-wrap break-words px-4 py-3">{{ cell(row, column) }}</td>
                <td v-if="recordKind === 'bindings'" class="px-4 py-3"><button class="btn btn-secondary" :disabled="busy || recordsBusy" @click="adminUnbind(row)">{{ t('qqBot.unbind') }}</button></td>
              </tr>
              <tr v-if="!records.items.length"><td :colspan="columns.length + 1" class="p-6 text-center text-gray-500">{{ recordsBusy ? t('qqBot.loading') : t('qqBot.empty') }}</td></tr>
            </tbody>
          </table>
        </div>
        <div class="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
          <p class="text-sm text-gray-500">{{ t('qqBot.page', { page, total: records.total }) }}</p>
          <div class="flex gap-2">
            <button class="btn btn-secondary" :disabled="page <= 1 || recordsBusy" @click="changePage(-1)">{{ t('qqBot.previous') }}</button>
            <button class="btn btn-secondary" :disabled="page * 20 >= records.total || recordsBusy" @click="changePage(1)">{{ t('qqBot.next') }}</button>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { qqbotApi, type QQBotConfig, type QQConnection, type QQPage, type QQRecord, type QQRecordKind } from '@/api/qqbot'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const app = useAppStore()
const config = ref<QQBotConfig | null>(null)
const groupsText = ref('')
const connection = ref<QQConnection | null>(null)
const busy = ref(false)
const error = ref('')
const testReply = ref('')
const recordKind = ref<QQRecordKind>('bindings')
const recordKinds: QQRecordKind[] = ['bindings', 'rewards', 'events']
const records = ref<QQPage>({ items: [], total: 0, page: 1, page_size: 20 })
const page = ref(1)
const recordsBusy = ref(false)
const wsURL = `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/api/v1/qq-bot/onebot/ws`
const numberFields = [
  { key: 'max_output_tokens', label: 'output', min: 1, max: 4096 },
  { key: 'history_messages', label: 'history', min: 0, max: 30 },
  { key: 'daily_chat_limit', label: 'memberLimit', min: 1, max: 1000 },
  { key: 'total_daily_chat_limit', label: 'siteLimit', min: 1, max: 100000 },
  { key: 'cooldown_seconds', label: 'cooldown', min: 1, max: 3600 },
] as const
const columns = computed(() => {
  if (recordKind.value === 'bindings') return ['user_id', 'qq', 'group_id', 'created_at']
  if (recordKind.value === 'rewards') return ['user_id', 'qq', 'group_id', 'day', 'amount', 'balance_after', 'created_at']
  return ['qq', 'group_id', 'command', 'status', 'result', 'created_at']
})
function cell(row: QQRecord, key: string) {
  const value = (row as unknown as Record<string, unknown>)[key]
  if (key === 'created_at' && typeof value === 'string') return new Date(value).toLocaleString()
  if ((key === 'amount' || key === 'balance_after') && typeof value === 'number') return `$${value.toFixed(8)}`
  if (key === 'status' && typeof value === 'string') return t(`qqBot.${value === 'failed' ? 'failedStatus' : value}`)
  return value == null ? '—' : String(value)
}
async function run(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try { await action() } catch (err) { error.value = extractApiErrorMessage(err, t('qqBot.failed')) }
  finally { busy.value = false }
}
function payload(): QQBotConfig {
  if (!config.value) throw new Error(t('qqBot.failed'))
  return { ...config.value, allowed_groups: groupsText.value.split(/[\s,，]+/).filter(Boolean) }
}
async function refreshConnection() { connection.value = await qqbotApi.connection() }
async function refresh() {
  await run(async () => {
    config.value = await qqbotApi.config()
    groupsText.value = config.value.allowed_groups.join('\n')
    testReply.value = ''
    await refreshConnection()
  })
  await loadRecords()
}
async function save() {
  await run(async () => {
    config.value = await qqbotApi.save(payload())
    groupsText.value = config.value.allowed_groups.join('\n')
    testReply.value = ''
    app.showSuccess(t('qqBot.saved'))
    await refreshConnection()
  })
}
async function testModel() { await run(async () => { testReply.value = (await qqbotApi.test(payload())).reply }) }
async function loadRecords() {
  if (recordsBusy.value) return
  recordsBusy.value = true
  try { records.value = await qqbotApi.records(recordKind.value, page.value) }
  catch (err) { error.value = extractApiErrorMessage(err, t('qqBot.failed')) }
  finally { recordsBusy.value = false }
}
async function selectRecords(kind: QQRecordKind) { recordKind.value = kind; page.value = 1; await loadRecords() }
async function changePage(delta: number) { page.value += delta; await loadRecords() }
async function adminUnbind(row: QQRecord) {
  if (!('user_id' in row) || !window.confirm(t('qqBot.unbindConfirm'))) return
  await run(async () => { await qqbotApi.adminUnbind(row.user_id); app.showSuccess(t('qqBot.unbound')); await loadRecords() })
}
let timer: ReturnType<typeof setInterval> | undefined
let disposed = false
onMounted(async () => {
  await refresh()
  if (!disposed) timer = setInterval(() => { void refreshConnection().catch(() => { connection.value = null }) }, 10000)
})
onUnmounted(() => { disposed = true; if (timer) clearInterval(timer) })
</script>
