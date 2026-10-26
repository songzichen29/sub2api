<template>
  <span v-if="stats" class="inline-flex shrink-0 items-baseline gap-1 text-xs text-gray-500 dark:text-gray-400" :title="t('modelQuality.successRateHint', { ...stats })" data-quality-success-rate>
    {{ label || t('modelQuality.successRate') }}
    <strong class="font-semibold tabular-nums text-primary-600 dark:text-primary-400">{{ qualitySuccessRate(stats) }}</strong>
    <span v-if="stats.total && showCount" class="tabular-nums">({{ stats.passed }}/{{ stats.total }})</span>
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { QualityStats } from '@/api/modelQuality'
import { qualitySuccessRate } from './qualityPresentation'

withDefaults(defineProps<{ stats?: QualityStats; label?: string; showCount?: boolean }>(), { showCount: true })
const { t } = useI18n()
</script>
