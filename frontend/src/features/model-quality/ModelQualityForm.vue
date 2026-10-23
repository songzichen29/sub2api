<template>
  <BaseDialog :show="true" :title="t(plan ? 'modelQuality.edit' : 'modelQuality.create')" width="wide" @close="emit('close')">
    <form id="quality-form" class="space-y-4" @submit.prevent="save">
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.name') }}</span>
          <input v-model="form.name" class="input" required maxlength="160" />
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.monitor') }}</span>
          <select v-model.number="form.config.monitor_id" class="input" required :disabled="!!plan?.config?.monitor_id" @change="changeMonitor">
            <option :value="0" disabled>{{ t('modelQuality.selectMonitor') }}</option>
            <option v-for="monitor in options.monitors" :key="monitor.id" :value="monitor.id">{{ monitor.name }}</option>
            <option v-if="plan?.config?.monitor_id && !options.monitors.some(m => m.id === plan?.config?.monitor_id)" :value="plan.config.monitor_id">{{ plan.target.monitor_name || `#${plan.config.monitor_id}` }}</option>
          </select>
          <span v-if="!loading && !options.monitors.length" class="block text-xs text-amber-700 dark:text-amber-400">{{ t('modelQuality.noMonitors') }}</span>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.group') }}</span>
          <select v-model.number="form.group_id" class="input" required :disabled="!!plan || !!selectedMonitor?.group_id">
            <option :value="0" disabled>—</option>
            <option v-for="group in options.groups" :key="group.id" :value="group.id">{{ group.name }}{{ group.channel_name ? ` · ${group.channel_name}` : '' }}</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.benchmark') }}</span>
          <select v-model="form.config.benchmark" class="input" :disabled="!!plan" @change="changeBenchmark">
            <option v-for="mode in ['candy', 'pelican', 'creative']" :key="mode" :value="mode">{{ t(`modelQuality.benchmarks.${mode}`) }}</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.model') }}</span>
          <input v-model.trim="form.config.model" class="input" required maxlength="200" :disabled="!!plan || form.config.benchmark !== 'candy'" />
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.effort') }}</span>
          <select v-model="form.config.reasoning_effort" class="input" :disabled="!!plan">
            <option v-for="effort in efforts" :key="effort">{{ effort }}</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.tier') }}</span>
          <select v-model="form.config.service_tier" class="input" :disabled="!!plan">
            <option value="">{{ t('modelQuality.defaultTier') }}</option>
            <option value="priority">priority</option><option value="ultrafast">ultrafast</option>
          </select>
        </label>
        <label class="space-y-1 text-sm">
          <span>{{ t('modelQuality.interval') }}</span>
          <input v-model.number="form.interval_minutes" class="input" type="number" min="15" max="10080" required />
        </label>
      </div>
      <label class="flex items-center gap-2 text-sm"><input v-model="form.enabled" type="checkbox" />{{ t('modelQuality.enabled') }}</label>
      <label class="flex items-center gap-2 text-sm"><input v-model="form.public_visible" type="checkbox" />{{ t('modelQuality.publicVisible') }}</label>
      <p class="text-xs text-gray-500">{{ t('modelQuality.boundCredentialNotice') }}</p>
    </form>
    <template #footer>
      <button class="btn btn-secondary" :disabled="saving" @click="emit('close')">{{ t('modelQuality.close') }}</button>
      <button class="btn btn-primary" type="submit" form="quality-form" :disabled="saving || loading || !form.group_id || !form.config.monitor_id">{{ t('modelQuality.save') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import * as api from '@/api/modelQuality'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{ plan: api.QualityPlan | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const saving = ref(false)
const loading = ref(true)
const error = ref('')
const options = ref<api.QualityOptions>({ groups: [], keys: [], monitors: [] })
const form = reactive<api.QualityInput>({
  name: props.plan?.name ?? '', group_id: props.plan?.group_id ?? 0,
  enabled: props.plan?.enabled ?? false, public_visible: props.plan?.public_visible ?? false,
  interval_minutes: props.plan?.interval_minutes ?? 60,
  config: props.plan?.config ? { ...props.plan.config } : {
    monitor_id: 0, benchmark: 'candy', model: 'gpt-6-astra', reasoning_effort: 'low', service_tier: '',
  },
})
const selectedMonitor = computed(() => options.value.monitors.find(m => m.id === form.config.monitor_id))
function changeMonitor() {
  if (!props.plan) {
    form.group_id = selectedMonitor.value?.group_id || 0
    if (form.config.benchmark === 'candy' && selectedMonitor.value?.model) form.config.model = selectedMonitor.value.model
  }
  delete form.config.base_url
  delete form.config.api_key_id
}
const efforts = computed(() => form.config.benchmark === 'candy' ? ['low', 'medium', 'high', 'xhigh', 'max', 'ultra'] : ['low', 'medium', 'high'])
function changeBenchmark() {
  if (form.config.benchmark !== 'candy') form.config.model = 'gpt-6-astra'
  if (!efforts.value.includes(form.config.reasoning_effort)) form.config.reasoning_effort = 'low'
}
async function save() {
  if (saving.value) return
  saving.value = true; error.value = ''
  try { await api.save(props.plan?.id ?? null, form); emit('saved') }
  catch (e) { error.value = extractApiErrorMessage(e, t('modelQuality.operationFailed')) }
  finally { saving.value = false }
}
onMounted(async () => {
  try {
    options.value = await api.options()
    if (!form.config.monitor_id && props.plan?.monitor_ids?.length === 1) form.config.monitor_id = props.plan.monitor_ids[0]
    if (form.config.monitor_id) changeMonitor()
  }
  catch (e) { error.value = extractApiErrorMessage(e, t('modelQuality.loadFailed')) }
  finally { loading.value = false }
})
</script>
