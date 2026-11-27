<template>
  <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
    <label class="text-sm">{{ t('qqbot.prizeName') }}<input v-model.trim="prize.name" class="input mt-1" maxlength="60" required /></label>
    <label class="text-sm">{{ t('qqbot.codeCampaigns.type') }}<select v-model="prize.type" class="input mt-1" :disabled="locked"><option v-for="kind in types" :key="kind" :value="kind">{{ t(`qqbot.codeCampaigns.types.${kind}`) }}</option></select></label>
    <label v-if="prize.type !== 'subscription'" class="text-sm">{{ t(prize.type === 'balance' ? 'qqbot.codeCampaigns.balanceValue' : 'qqbot.codeCampaigns.countValue') }}<input v-model.number="prize.value" class="input mt-1" type="number" :min="prize.type === 'balance' ? 0.00000001 : 1" max="1000" :step="prize.type === 'balance' ? 0.00000001 : 1" :disabled="locked" required /></label>
    <template v-else>
      <label class="text-sm">{{ t('qqbot.codeCampaigns.subscriptionGroup') }}<select v-model="prize.subscription_group_id" class="input mt-1" :disabled="locked" required><option :value="null" disabled>{{ t('qqbot.codeCampaigns.chooseGroup') }}</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
      <label class="text-sm">{{ t('qqbot.codeCampaigns.validityDays') }}<input v-model.number="prize.validity_days" class="input mt-1" type="number" min="1" max="3650" :disabled="locked" required /></label>
    </template>
    <label v-if="probability" class="text-sm">{{ t('qqbot.probability') }}<input :value="prize.probability_bps / 100" class="input mt-1" type="number" min="0.01" max="100" step="0.01" required @input="prize.probability_bps = Math.round(Number(($event.target as HTMLInputElement).value) * 100)" /></label>
    <label class="text-sm">{{ t('qqbot.prizeQuantity') }}<input v-model.number="prize.stock_limit" class="input mt-1" type="number" :min="prize.allocated" max="100000" step="1" required /><span class="mt-1 block text-gray-500">{{ t('qqbot.prizeAllocated', { count: prize.allocated }) }}</span></label>
  </div>
</template>
<script setup lang="ts">
import { watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQCampaignPrize } from '@/api/qqcommunity'
import type { AdminGroup } from '@/types'

const prize = defineModel<QQCampaignPrize>({ required: true })
defineProps<{ groups: AdminGroup[]; locked: boolean; probability?: boolean }>()
const { t } = useI18n()
const types = ['lottery_chances', 'balance', 'subscription', 'concurrency'] as const
watch(() => prize.value.type, kind => {
  if (kind !== 'balance') prize.value.value = Math.max(1, Math.trunc(prize.value.value))
}, { flush: 'sync' })
</script>
