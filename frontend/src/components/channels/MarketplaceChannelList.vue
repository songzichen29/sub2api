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
          v-if="intervalRows(channel.pricing).length > 0"
          class="mt-3"
        >
          <button
            v-if="!intervalDetailsAlwaysOpen"
            type="button"
            class="inline-flex items-center text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
            @click="toggleIntervalDetails(channelKey(channel))"
          >
            {{
              isIntervalDetailsExpanded(channelKey(channel))
                ? t('modelMarketplace.hideIntervalDetails')
                : t('modelMarketplace.showIntervalDetails')
            }}
          </button>

          <div
            v-if="isIntervalDetailsVisible(channelKey(channel))"
            :class="[
              'overflow-x-auto rounded-md border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800',
              intervalDetailsAlwaysOpen ? 'mt-0' : 'mt-2',
            ]"
          >
            <table class="min-w-full divide-y divide-gray-200 text-left text-xs dark:divide-dark-700">
              <thead class="bg-gray-50 text-[11px] uppercase text-gray-500 dark:bg-dark-900 dark:text-gray-400">
                <tr>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">
                    {{ t('modelMarketplace.intervalTable.context') }}
                  </th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">
                    {{ t('availableChannels.pricing.inputPrice') }}
                  </th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">
                    {{ t('availableChannels.pricing.outputPrice') }}
                  </th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">
                    {{ t('availableChannels.pricing.cacheWritePrice') }}
                  </th>
                  <th class="whitespace-nowrap px-3 py-2 font-medium">
                    {{ t('availableChannels.pricing.cacheReadPrice') }}
                  </th>
                  <th
                    v-if="hasIntervalPerRequestPrice(channel.pricing)"
                    class="whitespace-nowrap px-3 py-2 font-medium"
                  >
                    {{ t('availableChannels.pricing.perRequestPrice') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                <tr
                  v-for="row in intervalRows(channel.pricing)"
                  :key="row.key"
                  class="text-gray-700 dark:text-gray-300"
                >
                  <td class="whitespace-nowrap px-3 py-2 font-medium text-gray-900 dark:text-white">
                    {{ row.context }}
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 font-mono">
                    {{ row.inputPrice }}
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 font-mono">
                    {{ row.outputPrice }}
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 font-mono">
                    {{ row.cacheWritePrice }}
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 font-mono">
                    {{ row.cacheReadPrice }}
                  </td>
                  <td
                    v-if="hasIntervalPerRequestPrice(channel.pricing)"
                    class="whitespace-nowrap px-3 py-2 font-mono"
                  >
                    {{ row.perRequestPrice }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <div
          v-if="pricingStats(channel.pricing).length === 0"
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
import type { UserPricingInterval, UserSupportedModelPricing } from '@/api/channels'
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
    intervalDetailsAlwaysOpen?: boolean
  }>(),
  {
    defaultVisibleCount: 2,
    intervalDetailsAlwaysOpen: false,
  },
)

const { t } = useI18n()
const expanded = ref(false)
const expandedIntervalDetails = ref<Set<string>>(new Set())
const perMillionScale = 1_000_000

const visibleChannels = computed(() =>
  expanded.value ? props.channels : props.channels.slice(0, props.defaultVisibleCount),
)

const intervalDetailsAlwaysOpen = computed(() => props.intervalDetailsAlwaysOpen)

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
      pricing.intervals.length > 0 ? { label: t('availableChannels.pricing.intervals'), value: intervalSummary(pricing.intervals) } : null,
    ].filter((stat): stat is { label: string; value: string } => !!stat)
  }

  if (pricing.billing_mode === BILLING_MODE_PER_REQUEST || pricing.billing_mode === BILLING_MODE_IMAGE) {
    const stats: Array<{ label: string; value: string }> = []
    // image 计费在 admin 表单里写入 per_request_price + intervals.per_request_price；
    // image_output_price 是 token 模式下的图片输出 token 单价，与 image 模式无关。
    if (pricing.per_request_price != null) {
      stats.push({
        label: t('availableChannels.pricing.perRequestPrice'),
        value: formatScaled(pricing.per_request_price, 1),
      })
    }
    for (const interval of pricing.intervals) {
      if (interval.per_request_price == null) continue
      const label = interval.tier_label?.trim() || formatRange(interval.min_tokens, interval.max_tokens)
      stats.push({
        label,
        value: formatScaled(interval.per_request_price, 1),
      })
    }
    return stats
  }

  return []
}

function formatRange(min: number, max: number | null): string {
  if (max == null) return t('modelMarketplace.intervalRangeOpen', { min: formatTokens(min) })
  return t('modelMarketplace.intervalRangeBounded', { min: formatTokens(min), max: formatTokens(max) })
}

function formatTokens(tokens: number): string {
  if (tokens >= 1_000_000) return `${formatCompactNumber(tokens / 1_000_000)}M`
  if (tokens >= 1_000) return `${formatCompactNumber(tokens / 1_000)}K`
  return formatCompactNumber(tokens)
}

function formatCompactNumber(value: number): string {
  return value.toLocaleString(undefined, {
    maximumFractionDigits: 1,
  })
}

function formatMillionPrice(price: number | null): string {
  return price == null ? '-' : `${formatScaled(price, perMillionScale)} / 1M`
}

function formatUnitPrice(price: number | null): string {
  return price == null ? '-' : formatScaled(price, 1)
}

function intervalSummary(intervals: UserPricingInterval[]): string {
  const first = intervals[0]
  if (!first) return ''
  const count = t('modelMarketplace.intervalCount', { count: intervals.length })
  return `${formatRange(first.min_tokens, first.max_tokens)} · ${count}`
}

function intervalRows(pricing: UserSupportedModelPricing | null) {
  if (!pricing || pricing.intervals.length === 0) return []

  return pricing.intervals.map((interval, index) => ({
    key: `${interval.min_tokens}-${interval.max_tokens ?? 'open'}-${index}`,
    context: interval.tier_label?.trim() || formatRange(interval.min_tokens, interval.max_tokens),
    inputPrice: formatMillionPrice(interval.input_price),
    outputPrice: formatMillionPrice(interval.output_price),
    cacheWritePrice: formatMillionPrice(interval.cache_write_price),
    cacheReadPrice: formatMillionPrice(interval.cache_read_price),
    perRequestPrice: formatUnitPrice(interval.per_request_price),
  }))
}

function hasIntervalPerRequestPrice(pricing: UserSupportedModelPricing | null): boolean {
  return !!pricing?.intervals.some((interval) => interval.per_request_price != null)
}

function channelKey(channel: ModelMarketplaceChannelEntry): string {
  return `${channel.channel_name}:${channel.channel_description || ''}`
}

function isIntervalDetailsExpanded(key: string): boolean {
  return expandedIntervalDetails.value.has(key)
}

function isIntervalDetailsVisible(key: string): boolean {
  return intervalDetailsAlwaysOpen.value || isIntervalDetailsExpanded(key)
}

function toggleIntervalDetails(key: string) {
  const next = new Set(expandedIntervalDetails.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  expandedIntervalDetails.value = next
}
</script>
