<template>
  <div class="space-y-4">
    <div>
      <label :for="`${idPrefix}-concurrency`" class="input-label">{{ t('admin.groups.operations.concurrency') }}</label>
      <input :id="`${idPrefix}-concurrency`" :value="concurrency" type="number" min="0" max="100000" step="1" class="input"
        @input="emit('update:concurrency', Number(($event.target as HTMLInputElement).value))" />
      <p class="input-hint">{{ t('admin.groups.operations.concurrencyHint') }}</p>
    </div>
    <div v-if="platform !== 'composite'">
      <label :for="`${idPrefix}-fallback`" class="input-label">{{ t('admin.groups.operations.fallback') }}</label>
      <select :id="`${idPrefix}-fallback`" :value="fallback || 0" class="input" :disabled="loading || failed"
        @change="emit('update:fallback', Number(($event.target as HTMLSelectElement).value))">
        <option :value="0">{{ t('admin.groups.operations.disabled') }}</option>
        <option v-if="fallback && !candidates.some(group => group.id === fallback)" :value="fallback">#{{ fallback }}</option>
        <option v-for="group in candidates" :key="group.id" :value="group.id">{{ group.name }}</option>
      </select>
      <p class="input-hint">{{ t('admin.groups.operations.fallbackHint') }}</p>
      <button v-if="failed" type="button" class="text-sm text-red-600 dark:text-red-400" @click="loadGroups">{{ t('admin.groups.operations.reload') }}</button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAll } from '@/api/admin/groups'
import type { AdminGroup, GroupPlatform } from '@/types'

const props = defineProps<{ idPrefix: string; platform: GroupPlatform; groupId?: number; concurrency: number; fallback: number | null }>()
const emit = defineEmits<{ 'update:concurrency': [number]; 'update:fallback': [number] }>()
const { t } = useI18n()
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const failed = ref(false)
let generation = 0
const candidates = computed(() => groups.value.filter(group => group.id !== props.groupId && group.platform === props.platform && group.status === 'active'))
async function loadGroups() {
  const current = ++generation
  loading.value = true
  failed.value = false
  try {
    const result = await getAll(props.platform)
    if (current === generation) groups.value = result
  } catch {
    if (current === generation) failed.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
watch(() => props.platform, (platform, old) => {
  if (old !== undefined && platform !== old) emit('update:fallback', 0)
  void loadGroups()
}, { immediate: true })
</script>
