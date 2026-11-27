<template>
  <div class="space-y-3" data-testid="token-tiers">
    <h3 class="font-semibold">{{ t('qqbot.codeCampaigns.tokenBonus') }}</h3>
    <div v-for="(tier, index) in tiers" :key="index" class="grid items-start gap-3 sm:grid-cols-[1fr_1fr_auto]">
      <label class="text-sm">
        {{ t('qqbot.codeCampaigns.tokenThreshold') }}
        <input v-model.number="tier.tokens" class="input mt-1" type="number" min="1" max="9007199254740991" step="1" required />
        <span v-if="tier.tokens > 0" class="mt-1 block text-gray-500 dark:text-gray-400" data-testid="token-equivalent">{{ t('qqbot.codeCampaigns.tokenEquivalent', { millions: tokenNumber.format(tier.tokens / 1000000), tenThousands: tokenNumber.format(tier.tokens / 10000) }) }}</span>
      </label>
      <label class="text-sm">{{ t('qqbot.codeCampaigns.bonusPoints') }}<input :value="tier.bonus_bps / 100" class="input mt-1" type="number" min="0.01" max="100" step="0.01" required @input="tier.bonus_bps = Math.round(Number(($event.target as HTMLInputElement).value) * 100)" /></label>
      <button type="button" class="btn btn-secondary sm:mt-6" @click="tiers.splice(index, 1)">{{ t('common.delete') }}</button>
    </div>
    <button type="button" class="btn btn-secondary" :disabled="tiers.length >= 20" @click="tiers.push({ tokens: 0, bonus_bps: 0 })">{{ t('qqbot.codeCampaigns.addTier') }}</button>
    <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('qqbot.codeCampaigns.tokenUnitHelp') }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQCampaignTokenTier } from '@/api/qqcommunity'

const tiers = defineModel<QQCampaignTokenTier[]>({ required: true })
const { t, locale } = useI18n()
const tokenNumber = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 6 }))
</script>
