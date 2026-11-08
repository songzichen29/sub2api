<template>
  <div class="space-y-4">
    <div v-if="type === 'apikey'" class="space-y-2 border-t border-gray-200 pt-4 dark:border-dark-600">
      <label class="flex items-center justify-between gap-4">
        <span class="input-label mb-0">{{ t('admin.accounts.upstreamBilling.syncRate') }}</span>
        <Toggle v-model="policy.rateSyncEnabled" data-testid="create-rate-sync" />
      </label>
      <p class="input-hint">{{ t('admin.accounts.upstreamBilling.syncRateHint') }}</p>
    </div>
    <div v-if="supportsSchedulingThreshold(platform)" class="space-y-2 border-t border-gray-200 pt-4 dark:border-dark-600">
      <label class="flex items-center justify-between gap-4">
        <span class="input-label mb-0">{{ t('admin.accounts.accountSchedulingThresholdOverride') }}</span>
        <Toggle v-model="policy.schedulingEnabled" data-testid="create-scheduling-override" />
      </label>
      <p class="input-hint">{{ t('admin.accounts.accountSchedulingThresholdOverrideHint') }}</p>
      <label v-if="policy.schedulingEnabled" class="block">
        <span class="input-label">{{ t('admin.accounts.accountSchedulingThresholdOverrideValue') }}</span>
        <input v-model.number="policy.schedulingThreshold" type="number" min="1" max="100" step="1" class="input" data-testid="create-scheduling-threshold" />
      </label>
    </div>
    <template v-if="platform === 'openai'">
      <div class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="flex items-center justify-between gap-4">
          <span class="input-label mb-0">{{ t('admin.accounts.autoPause5hDisabled') }}</span>
          <Toggle v-model="policy.pause5hDisabled" data-testid="create-pause-5h-disabled" />
        </label>
        <label class="block">
          <span class="input-label">{{ t('admin.accounts.autoPause5hThreshold') }}</span>
          <input v-model.number="policy.pause5hThreshold" type="number" min="0" max="100" step="0.1" :disabled="policy.pause5hDisabled" class="input" data-testid="create-pause-5h-threshold" />
        </label>
        <label class="flex items-center justify-between gap-4">
          <span class="input-label mb-0">{{ t('admin.accounts.autoPause7dDisabled') }}</span>
          <Toggle v-model="policy.pause7dDisabled" data-testid="create-pause-7d-disabled" />
        </label>
        <label class="block">
          <span class="input-label">{{ t('admin.accounts.autoPause7dThreshold') }}</span>
          <input v-model.number="policy.pause7dThreshold" type="number" min="0" max="100" step="0.1" :disabled="policy.pause7dDisabled" class="input" data-testid="create-pause-7d-threshold" />
        </label>
        <p class="input-hint">{{ t('admin.accounts.autoPauseThresholdHint') }}</p>
      </div>
      <div v-if="type === 'oauth'" class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="block">
          <span class="input-label">{{ t('admin.accounts.openai.planType') }}</span>
          <Select v-model="policy.planType" :options="planOptions" data-testid="create-plan-type" />
        </label>
        <p class="input-hint">{{ t('admin.accounts.openai.planTypeDesc') }}</p>
        <label class="flex items-center justify-between gap-4">
          <span class="input-label mb-0">{{ t('admin.accounts.autoResetCredit.title') }}</span>
          <Toggle v-model="policy.resetCreditEnabled" data-testid="create-reset-credit-enabled" />
        </label>
        <p class="input-hint">{{ t('admin.accounts.autoResetCredit.hint') }}</p>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="block">
            <span class="input-label">{{ t('admin.accounts.autoResetCredit.threshold5h') }}</span>
            <input v-model.number="policy.resetCredit5hThreshold" type="number" min="0.1" max="100" step="0.1" :disabled="!policy.resetCreditEnabled" class="input" data-testid="create-reset-credit-5h" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.accounts.autoResetCredit.threshold7d') }}</span>
            <input v-model.number="policy.resetCredit7dThreshold" type="number" min="0.1" max="100" step="0.1" :disabled="!policy.resetCreditEnabled" class="input" data-testid="create-reset-credit-7d" />
          </label>
        </div>
      </div>
      <div class="space-y-2 border-t border-gray-200 pt-4 dark:border-dark-600">
        <label class="block">
          <span class="input-label">{{ t('admin.accounts.openai.codexImageTool') }}</span>
          <select v-model="policy.imageToolMode" class="input" data-testid="create-image-tool-mode">
            <option value="inherit">{{ t('admin.accounts.openai.codexImageToolInherit') }}</option>
            <option value="enabled">{{ t('admin.accounts.openai.codexImageToolEnabled') }}</option>
            <option value="disabled">{{ t('admin.accounts.openai.codexImageToolDisabled') }}</option>
            <option value="block">{{ t('admin.accounts.openai.codexImageToolBlock') }}</option>
          </select>
        </label>
        <p class="input-hint">{{ t('admin.accounts.openai.codexImageToolDesc') }}</p>
      </div>
    </template>
    <div v-if="platform === 'grok' && type === 'oauth'" class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600">
      <label class="flex items-center justify-between gap-4">
        <span class="input-label mb-0">{{ t('admin.accounts.grokClientToolCache.title') }}</span>
        <Toggle v-model="policy.grokClientToolCache" data-testid="create-grok-tool-cache" />
      </label>
      <p class="input-hint">{{ t('admin.accounts.grokClientToolCache.hint') }}</p>
      <label class="block">
        <span class="input-label">{{ t('admin.accounts.grokMediaEligibility.title') }}</span>
        <select v-model="policy.grokMediaMode" class="input" data-testid="create-grok-media-mode">
          <option value="auto">{{ t('admin.accounts.grokMediaEligibility.auto') }}</option>
          <option value="enabled">{{ t('admin.accounts.grokMediaEligibility.enabled') }}</option>
          <option value="disabled">{{ t('admin.accounts.grokMediaEligibility.disabled') }}</option>
        </select>
      </label>
      <p class="input-hint">{{ t('admin.accounts.grokMediaEligibility.hint') }}</p>
      <p v-if="policy.grokMediaMode === 'enabled'" class="text-xs text-amber-700 dark:text-amber-400">{{ t('admin.accounts.grokMediaEligibility.forceEnableWarning') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import Select from '@/components/common/Select.vue'
import { buildPlanTypeOptions } from './credentialsBuilder'
import { supportsSchedulingThreshold, type AccountCreatePolicies } from './accountCreatePolicies'

defineProps<{ platform: string; type: string }>()
const policy = defineModel<AccountCreatePolicies>({ required: true })
const { t } = useI18n()
const planOptions = computed(() => buildPlanTypeOptions(policy.value.planType, t('admin.accounts.openai.planTypeClear')))
</script>
