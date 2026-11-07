<template>
  <div v-if="available" class="chance-breakdown" data-testid="lottery-chance-breakdown">
    <table>
      <caption class="sr-only">{{ t('lotteryPortal.chanceSources') }}</caption>
      <thead><tr><th scope="col">{{ t('lotteryPortal.chanceSources') }}</th><th scope="col">{{ t('lotteryPortal.sourceEarned') }}</th><th scope="col">{{ t('lotteryPortal.used') }}</th><th scope="col">{{ t('lotteryPortal.remaining') }}</th></tr></thead>
      <tbody>
        <tr data-testid="recharge-chances"><th scope="row">{{ t('lotteryPortal.rechargeChances') }}</th><td>{{ state.recharge_earned }}</td><td>{{ state.recharge_used }}</td><td>{{ state.recharge_remaining }}</td></tr>
        <tr data-testid="activity-chances"><th scope="row">{{ t('lotteryPortal.activityChances') }}</th><td>{{ state.bonus_chances }}</td><td>{{ state.bonus_used }}</td><td>{{ state.bonus_remaining }}</td></tr>
      </tbody>
    </table>
    <p>{{ t('lotteryPortal.separateChances') }}</p>
    <p v-if="state.legacy_used" data-testid="legacy-chances">{{ t('lotteryPortal.legacyChances', { count: state.legacy_used }) }}</p>
    <p v-if="state.recharge_used! > state.recharge_earned!">{{ t('lotteryPortal.rechargeDeficit') }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQLotteryState } from '@/api/qqcommunity'
const props = defineProps<{ state: QQLotteryState }>()
const { t } = useI18n()
// An older API has no source breakdown. Do not guess which draws used gifts.
const available = computed(() => [props.state.recharge_earned, props.state.recharge_used, props.state.recharge_remaining, props.state.bonus_chances, props.state.bonus_used, props.state.bonus_remaining].every(value => typeof value === 'number'))
</script>
<style scoped>
.chance-breakdown { margin-top: 16px; font-size: 13px; }
table { width: 100%; border-collapse: collapse; font-variant-numeric: tabular-nums; }
th, td { padding: 10px 4px; text-align: right; border-bottom: 1px solid #e5e7eb; }
th { font-weight: 500; }
th:first-child { padding-left: 0; text-align: left; }
thead { color: #6b7280; font-size: 12px; }
p { margin-top: 10px; color: #6b7280; font-size: 12px; line-height: 1.6; }
:global(.dark) th, :global(.dark) td { border-color: #374151; }
:global(.dark) thead, :global(.dark) p { color: #9ca3af; }
</style>
