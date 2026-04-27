<template>
  <div class="space-y-3">
    <div
      v-for="channel in visibleChannels"
      :key="`${channel.channel_name}-${channel.channel_description}`"
      class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900"
    >
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <span class="text-sm font-medium text-gray-900 dark:text-white">
              {{ channel.channel_name }}
            </span>
            <span class="rounded-md bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ platformLabel(channel.groups[0]?.platform || '') }}
            </span>
          </div>

          <div v-if="channel.groups.length > 0" class="mt-2 flex flex-wrap gap-1.5">
            <GroupBadge
              v-for="group in channel.groups.slice(0, 3)"
              :key="`${channel.channel_name}-${group.id}`"
              :name="group.name"
              :platform="group.platform as GroupPlatform"
              :subscription-type="group.subscription_type as SubscriptionType"
              :rate-multiplier="group.rate_multiplier"
              :always-show-rate="true"
            />
            <span
              v-if="channel.groups.length > 3"
              class="inline-flex items-center rounded-md border border-dashed border-gray-200 px-2 py-0.5 text-[11px] text-gray-500 dark:border-dark-600 dark:text-gray-400"
            >
              +{{ channel.groups.length - 3 }}
            </span>
          </div>

          <p
            v-if="channel.channel_description"
            class="mt-1 line-clamp-2 text-xs text-gray-500 dark:text-gray-400"
          >
            {{ channel.channel_description }}
          </p>
        </div>
      </div>

      <div class="mt-3">
        <div class="mb-2">
          <span class="rounded-md bg-primary-50 px-2 py-0.5 text-[11px] text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">
            {{ billingModeLabel(channel.pricing) }}
          </span>
        </div>

        <div
          v-if="pricingStats(channel.pricing).length > 0"
          class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3"
        >
          <div
            v-for="stat in pricingStats(channel.pricing)"
            :key="stat.label"
            class="rounded-md border border-gray-200 bg-white px-3 py-2 dark:border-dark-700 dark:bg-dark-800"
          >
            <div class="text-[11px] text-gray-500 dark:text-gray-400">
              {{ stat.label }}
            </div>
            <div class="mt-1 text-sm font-medium text-gray-900 dark:text-white">
              {{ stat.value }}
            </div>
          </div>
        </div>

        <div
          v-else
          class="rounded-md border border-dashed border-gray-200 px-3 py-3 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ noPricingLabel }}
        </div>
      </div>
    </div>

    <button
      v-if="channels.length > defaultVisibleCount"
      type="button"
      class="text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
      @click="expanded = !expanded"
    >
      {{
        expanded
          ? t('modelMarketplace.collapseChannels')
          : t('modelMarketplace.moreChannels', { count: channels.length - defaultVisibleCount })
      }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserSupportedModelPricing } from '@/api/channels'
import type { GroupPlatform, SubscriptionType } from '@/types'
import GroupBadge from '@/components/common/GroupBadge.vue'
import type { ModelMarketplaceChannelEntry } from '@/utils/modelMarketplace'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN } from '@/constants/channel'
import { formatScaled } from '@/utils/pricing'
import { platformLabel } from '@/utils/platformColors'

const props = withDefaults(
  defineProps<{
    channels: ModelMarketplaceChannelEntry[]
    noPricingLabel: string
    defaultVisibleCount?: number
  }>(),
  {
    defaultVisibleCount: 2,
  },
)

const { t } = useI18n()
const expanded = ref(false)
const perMillionScale = 1_000_000

const visibleChannels = computed(() =>
  expanded.value ? props.channels : props.channels.slice(0, props.defaultVisibleCount),
)

function billingModeLabel(pricing: UserSupportedModelPricing | null): string {
  if (!pricing) return t('availableChannels.noPricing')
  switch (pricing.billing_mode) {
    case BILLING_MODE_TOKEN:
      return t('availableChannels.pricing.billingModeToken')
    case BILLING_MODE_PER_REQUEST:
      return t('availableChannels.pricing.billingModePerRequest')
    case BILLING_MODE_IMAGE:
      return t('availableChannels.pricing.billingModeImage')
    default:
      return '-'
  }
}

function pricingStats(pricing: UserSupportedModelPricing | null): Array<{ label: string; value: string }> {
  if (!pricing) return []

  if (pricing.billing_mode === BILLING_MODE_TOKEN) {
    return [
      pricing.input_price != null ? { label: t('availableChannels.pricing.inputPrice'), value: `${formatScaled(pricing.input_price, perMillionScale)} / 1M` } : null,
      pricing.output_price != null ? { label: t('availableChannels.pricing.outputPrice'), value: `${formatScaled(pricing.output_price, perMillionScale)} / 1M` } : null,
      pricing.cache_write_price != null ? { label: t('availableChannels.pricing.cacheWritePrice'), value: `${formatScaled(pricing.cache_write_price, perMillionScale)} / 1M` } : null,
      pricing.cache_read_price != null ? { label: t('availableChannels.pricing.cacheReadPrice'), value: `${formatScaled(pricing.cache_read_price, perMillionScale)} / 1M` } : null,
      pricing.per_request_price != null ? { label: t('availableChannels.pricing.perRequestPrice'), value: formatScaled(pricing.per_request_price, 1) } : null,
      pricing.intervals.length > 0 ? { label: t('availableChannels.pricing.intervals'), value: t('modelMarketplace.intervalCount', { count: pricing.intervals.length }) } : null,
    ].filter((stat): stat is { label: string; value: string } => !!stat)
  }

  if (pricing.billing_mode === BILLING_MODE_PER_REQUEST) {
    return pricing.per_request_price != null
      ? [{ label: t('availableChannels.pricing.perRequestPrice'), value: formatScaled(pricing.per_request_price, 1) }]
      : []
  }

  if (pricing.billing_mode === BILLING_MODE_IMAGE) {
    return pricing.image_output_price != null
      ? [{ label: t('availableChannels.pricing.imageOutputPrice'), value: formatScaled(pricing.image_output_price, 1) }]
      : []
  }

  return []
}
</script>
