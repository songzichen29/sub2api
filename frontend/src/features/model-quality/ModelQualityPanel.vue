<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <h2 class="font-semibold">{{ t('modelQuality.title') }}</h2>
      <div class="flex items-center gap-2">
        <select v-model.number="groupFilter" class="input !w-auto" :aria-label="t('modelQuality.group')"><option :value="0">{{ t('modelQuality.allGroups') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select>
        <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="refresh">{{ t('modelQuality.refresh') }}</button>
        <button v-if="admin" type="button" class="btn btn-primary btn-sm" @click="openForm(null)">{{ t('modelQuality.create') }}</button>
      </div>
    </div>
    <p v-if="error || actionError" role="alert" class="text-sm text-red-600">{{ error || actionError }}</p>
    <p v-if="!filteredPlans.length" class="py-10 text-center text-sm text-gray-500">{{ t(loading ? 'modelQuality.loading' : 'modelQuality.empty') }}</p>
    <div class="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
      <article v-for="plan in filteredPlans" :key="plan.id" class="min-w-0 rounded-xl border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <header class="mb-4 flex items-start justify-between gap-3">
          <div class="min-w-0"><h3 class="truncate text-sm font-semibold">{{ plan.name }}</h3><p class="mt-1 truncate text-xs text-gray-500">{{ plan.target.group_name || `#${plan.group_id}` }}</p></div>
          <span class="shrink-0 text-xs text-gray-500" :title="plan.enabled ? `${t('modelQuality.next')}: ${qualityTime(plan.next_run_at)}` : ''">{{ plan.enabled ? t('modelQuality.intervalShort', { minutes: plan.interval_minutes }) : t('modelQuality.manual') }}</span>
        </header>
        <ModelQualityStrip :plans="[plan]" @select="selection = $event" />
        <div class="mt-4 flex items-center justify-between gap-3 text-xs text-gray-500">
          <span :title="t('modelQuality.samples', { count: plan.stats.passed + plan.stats.failed })">{{ t('modelQuality.rate') }} <strong class="ml-1 font-medium tabular-nums text-gray-800 dark:text-gray-100">{{ passRate(plan) }}</strong></span>
          <button type="button" class="hover:text-primary-600" @click="selection = { planId: plan.id, groupId: plan.group_id, groupName: plan.target.group_name, planName: plan.name, benchmark: plan.target.benchmark }">{{ t('modelQuality.history') }}</button>
        </div>
        <div v-if="admin" class="mt-4 flex flex-wrap items-center gap-2 border-t border-gray-100 pt-3 dark:border-dark-700">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="busy === plan.id || !!plan.active_run_id" @click="start(plan)">{{ t('modelQuality.run') }}</button>
          <button v-if="plan.can_edit" type="button" class="btn btn-ghost btn-sm" :disabled="!!plan.active_run_id" @click="openForm(plan)">{{ t('modelQuality.edit') }}</button>
          <button v-if="plan.active_run_id" type="button" class="btn btn-ghost btn-sm" :disabled="busy === plan.id" @click="cancelRun(plan)">{{ t('modelQuality.cancel') }}</button>
          <button type="button" class="btn btn-ghost btn-sm text-red-600" :disabled="busy === plan.id || !!plan.active_run_id" @click="deleting = plan; actionError = ''">{{ t('modelQuality.delete') }}</button>
        </div>
      </article>
    </div>
    <ModelQualityForm v-if="formOpen" :plan="editing" @close="formOpen = false" @saved="saved" />
    <ConfirmDialog :show="!!deleting" :title="t('modelQuality.delete')" :message="t('modelQuality.deleteConfirm', { name: deleting?.name })" danger @confirm="deletePlan" @cancel="deleting = null">
      <p v-if="actionError" role="alert" class="text-sm text-red-600">{{ actionError }}</p>
    </ConfirmDialog>
    <ModelQualityDetails :selection="selection" :admin="admin" @close="selection = null" />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import ModelQualityForm from './ModelQualityForm.vue'
import ModelQualityDetails from './ModelQualityDetails.vue'
import ModelQualityStrip from './ModelQualityStrip.vue'
import { useModelQuality } from './useModelQuality'
import { qualityTime, type QualitySelection } from './qualityPresentation'
import * as api from '@/api/modelQuality'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = withDefaults(defineProps<{ admin?: boolean; groupIds?: number[] }>(), { admin: false })
const { t } = useI18n()
const { plans, loading, error, refresh } = useModelQuality(() => props.admin)
const actionError = ref('')
const busy = ref('')
const groupFilter = ref(0)
const formOpen = ref(false)
const editing = ref<api.QualityPlan | null>(null)
const deleting = ref<api.QualityPlan | null>(null)
const selection = ref<QualitySelection | null>(null)
const groups = computed(() => Array.from(new Map(plans.value.map(p => [p.group_id, { id: p.group_id, name: p.target.group_name || `#${p.group_id}` }])).values()))
const filteredPlans = computed(() => plans.value.filter(p => (!groupFilter.value || p.group_id === groupFilter.value) && (!props.groupIds?.length || props.groupIds.includes(p.group_id))))
const passRate = (p: api.QualityPlan) => p.stats.passed + p.stats.failed ? `${(100 * p.stats.passed / (p.stats.passed + p.stats.failed)).toFixed(1)}%` : '—'
function openForm(plan: api.QualityPlan | null) { editing.value = plan; formOpen.value = true }
async function saved() { formOpen.value = false; await refresh() }
async function start(plan: api.QualityPlan) {
  busy.value = plan.id; actionError.value = ''
  try { await api.run(plan.id); await refresh() }
  catch (e) { actionError.value = extractApiErrorMessage(e, t('modelQuality.operationFailed')) }
  finally { busy.value = '' }
}
async function cancelRun(plan: api.QualityPlan) {
  const runID = plan.active_run_id; if (!runID) return
  busy.value = plan.id; actionError.value = ''
  try { await api.cancel(runID); await refresh() }
  catch (e) { actionError.value = extractApiErrorMessage(e, t('modelQuality.operationFailed')) }
  finally { busy.value = '' }
}
async function deletePlan() {
  const plan = deleting.value
  if (!plan || busy.value) return
  busy.value = plan.id; actionError.value = ''
  try {
    await api.remove(plan.id)
    deleting.value = null
    if (selection.value?.planId === plan.id) selection.value = null
    await refresh()
  } catch (e) { actionError.value = extractApiErrorMessage(e, t('modelQuality.operationFailed')) }
  finally { busy.value = '' }
}
</script>
