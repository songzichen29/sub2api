<template>
  <section class="card space-y-4 p-6" :aria-label="t('qqbot.runtimeChat.title')">
    <h2 class="text-lg font-semibold">{{ t('qqbot.runtimeChat.title') }}</h2>
    <p class="text-sm text-gray-500">{{ t('qqbot.runtimeChat.memoryHelp') }}</p>
    <p class="text-sm" role="status" data-testid="dsh-runtime-status">{{ t(status?.connected ? 'qqbot.runtimeChat.connected' : 'qqbot.runtimeChat.pending') }}</p>
    <p v-if="status?.last_seen_at" class="text-xs text-gray-500">{{ t('qqbot.runtimeChat.memoryStats', { sessions: status.sessions, messages: status.messages, summaries: status.summaries }) }}</p>
    <QQChatDiagnostics :status="status" :show-records="false" />
    <label class="block space-y-1">{{ t('qqbot.runtimeChat.names') }}<textarea v-model="names" class="input" rows="2" data-testid="runtime-names" /></label>
    <label class="flex items-center gap-2"><input v-model="value.ambient_enabled" type="checkbox" data-testid="runtime-ambient-enabled" />{{ t('qqbot.runtimeChat.ambient') }}</label>
    <QQGroupPicker v-model="value.ambient_groups" :groups="groups" :policies="policies" :names="groupNames" :label="t('qqbot.runtimeChat.ambientGroups')" />
    <div class="grid gap-4 md:grid-cols-2">
      <label v-for="field in fields" :key="field.key" class="space-y-1">{{ t('qqbot.runtimeChat.' + field.key) }}<input v-model.number="value[field.key]" class="input" type="number" :min="field.min" :max="field.max" step="1" required :data-testid="'runtime-' + field.key" /></label>
    </div>
    <label class="flex items-center gap-2"><input v-model="value.image_input_enabled" type="checkbox" />{{ t('qqbot.runtimeChat.images') }}</label>
    <label class="flex items-center gap-2"><input v-model="value.web_enabled" type="checkbox" data-testid="runtime-web-enabled" />{{ t('qqbot.runtimeChat.web') }}</label>
    <label class="flex items-center gap-2"><input v-model="value.memes_enabled" type="checkbox" data-testid="runtime-memes-enabled" />{{ t('qqbot.runtimeChat.memes') }}</label>
    <label class="flex items-center gap-2"><input v-model="value.collect_memes" type="checkbox" :disabled="!value.memes_enabled" />{{ t('qqbot.runtimeChat.collectMemes') }}</label>
    <p class="text-xs text-gray-500">{{ t('qqbot.runtimeChat.toolsHelp') }}</p>
    <fieldset class="space-y-3 rounded-lg border p-4 dark:border-dark-700">
      <legend>{{ t('qqbot.runtimeChat.proactiveTitle') }}</legend>
      <label class="flex items-center gap-2"><input v-model="value.proactive_enabled" type="checkbox" data-testid="proactive-enabled" />{{ t('qqbot.runtimeChat.proactiveEnabled') }}</label>
      <p class="text-xs text-gray-500">{{ t('qqbot.runtimeChat.timezoneHelp') }}</p>
      <QQGroupPicker v-model="value.proactive_groups" :groups="groups" :policies="policies" :names="groupNames" :label="t('qqbot.runtimeChat.proactiveGroups')" />
      <label class="block space-y-1">{{ t('qqbot.runtimeChat.proactiveTimes') }}<textarea v-model="proactiveTimes" class="input" rows="2" placeholder="12:00, 14:00, 18:00, 21:00" data-testid="proactive-times" :required="value.proactive_enabled" /></label>
      <label class="block space-y-1">{{ t('qqbot.runtimeChat.proactiveIdle') }}<input v-model.number="value.proactive_idle_minutes" class="input" type="number" min="0" max="1440" step="1" required data-testid="proactive-idle" /></label>
      <p class="text-xs text-gray-500">{{ t('qqbot.runtimeChat.proactiveHelp') }}</p>
    </fieldset>
    <fieldset class="space-y-3 rounded-lg border p-4 dark:border-dark-700">
      <legend>{{ t('qqbot.runtimeChat.welcomeTitle') }}</legend>
      <label class="flex items-center gap-2"><input v-model="value.welcome_enabled" type="checkbox" data-testid="welcome-enabled" />{{ t('qqbot.runtimeChat.welcomeEnabled') }}</label>
      <label class="block space-y-1">{{ t('qqbot.runtimeChat.welcomeText') }}<textarea v-model="value.welcome_message" class="input" rows="3" maxlength="1000" data-testid="welcome-message" :required="value.welcome_enabled" /></label>
      <p class="text-xs text-gray-500">{{ t('qqbot.runtimeChat.welcomeHelp') }}</p>
    </fieldset>
    <details>
      <summary class="cursor-pointer text-sm">{{ t('qqbot.workspace.advanced') }}</summary>
      <div class="mt-3 grid gap-4 md:grid-cols-2">
        <label class="space-y-1">{{ t('qqbot.runtimeChat.contextCapacity') }}<input v-model.number="value.context_window_tokens" class="input" type="number" min="8192" max="262144" required /></label>
        <label class="space-y-1">{{ t('qqbot.runtimeChat.media_daily_calls') }}<input v-model.number="value.media_daily_calls" class="input" type="number" min="0" max="100000" required /></label>
      </div>
    </details>
  </section>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import QQGroupPicker from './QQGroupPicker.vue'
import QQChatDiagnostics from './QQChatDiagnostics.vue'
import type { QQRuntimeChatConfig, QQChatRuntimeStatus, QQGroupPolicy } from '@/api/qqcommunity'
const value = defineModel<QQRuntimeChatConfig>({ required: true })
defineProps<{ status?: QQChatRuntimeStatus; groups: string[]; policies?: QQGroupPolicy[]; groupNames?: Record<string, string> }>()
const { t } = useI18n()
const names = computed({ get: () => value.value.names.join('\n'), set: (text: string) => { value.value.names = [...new Set(text.split(/[\n,，]+/).map(s => s.trim()).filter(Boolean))] } })
const splitTimes = (text: string) => text.split(/[\s,，]+/).filter(Boolean)
const proactiveTimes = ref(value.value.proactive_times.join(', '))
watch(proactiveTimes, text => { value.value.proactive_times = splitTimes(text) }, { flush: 'sync' })
watch(() => value.value.proactive_times, times => {
  // Keep a trailing comma/newline while typing; refresh on external reload.
  if (JSON.stringify(times) !== JSON.stringify(splitTimes(proactiveTimes.value))) proactiveTimes.value = times.join(', ')
})
const fields = [
  { key: 'sample_percent', min: 0, max: 100 }, { key: 'start_hour', min: 0, max: 23 }, { key: 'end_hour', min: 0, max: 23 },
  { key: 'hourly_replies', min: 0, max: 100000 }, { key: 'daily_replies', min: 0, max: 100000 }, { key: 'daily_calls', min: 0, max: 100000 }
] as const
</script>
