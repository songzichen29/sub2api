<template>
  <fieldset class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-700">
    <legend class="font-medium">{{ t('lotteryPortal.publicGroups') }}</legend>
    <p class="text-sm text-gray-500">{{ t('lotteryPortal.publicGroupsHelp') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600">{{ error }}</p>
    <div v-for="(group, index) in value" :key="index" class="space-y-3 border-b border-gray-200 pb-4 dark:border-dark-700">
      <label class="block space-y-1">{{ t('lotteryPortal.group') }}
        <select v-model="group.group_id" class="input" required @change="choose(group)">
          <option value="" disabled>{{ t('lotteryPortal.selectGroup') }}</option>
          <option v-if="group.group_id && !available.some(g => g.id === group.group_id)" :value="group.group_id" disabled>{{ group.group_id }}</option>
          <option v-for="g in available" :key="g.id" :value="g.id" :disabled="value.some((other, i) => i !== index && other.group_id === g.id)">{{ g.name }} · {{ g.id }}</option>
        </select>
      </label>
      <label class="block space-y-1">{{ t('lotteryPortal.groupName') }}<input v-model.trim="group.name" class="input" required maxlength="60" /></label>
      <label class="block space-y-1">{{ t('lotteryPortal.joinURL') }}<input v-model.trim="group.join_url" class="input" type="url" maxlength="2048" placeholder="https://qun.qq.com/…" /></label>
      <div class="flex gap-3">
        <button type="button" class="btn btn-secondary" :disabled="index === 0" @click="move(index, -1)">{{ t('lotteryPortal.moveUp') }}</button>
        <button type="button" class="btn btn-secondary" :disabled="index === value.length - 1" @click="move(index, 1)">{{ t('lotteryPortal.moveDown') }}</button>
        <button type="button" class="btn btn-secondary" @click="value.splice(index, 1)">{{ t('lotteryPortal.removeGroup') }}</button>
      </div>
    </div>
    <p class="text-sm text-gray-500">{{ t('lotteryPortal.joinURLHelp') }}</p>
    <button type="button" class="btn btn-secondary" :disabled="!available.length || value.length >= 20" @click="value.push({ group_id: '', name: '', join_url: '' })">{{ t('lotteryPortal.addGroup') }}</button>
  </fieldset>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { qqbotAPI, type QQLotteryPublicGroup } from '@/api/qqcommunity'
const value = defineModel<QQLotteryPublicGroup[]>({ default: () => [] })
const { t } = useI18n()
const available = ref<{ id: string; name: string }[]>([])
const error = ref('')
function move(index: number, offset: number) {
  const group = value.value.splice(index, 1)[0]!
  value.value.splice(index + offset, 0, group)
}
function choose(group: QQLotteryPublicGroup) { group.name = available.value.find(g => g.id === group.group_id)?.name || group.group_id }
onMounted(async () => {
  try {
    const c = await qqbotAPI.config()
    available.value = c.allowed_groups.filter(id => !c.group_policies?.some(p => p.group_id === id && p.disable_activities))
      .map(id => ({ id, name: c.group_policies?.find(p => p.group_id === id)?.label || id }))
  } catch { error.value = t('lotteryPortal.groupConfigFailed') }
})
</script>
