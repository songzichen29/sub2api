<template>
  <article
    class="group flex min-h-[114px] flex-col rounded-2xl border border-gray-200 bg-white px-5 py-4 shadow-[0_1px_6px_rgba(15,23,42,0.04)] transition-colors hover:border-gray-300 dark:border-dark-700 dark:bg-dark-800 dark:hover:border-dark-500"
  >
    <div class="flex items-center justify-between gap-3">
      <div class="flex min-w-0 items-center gap-3">
        <div
          class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-700"
        >
          <PlatformIcon :platform="item.provider as GroupPlatform" size="md" :class="platformIconClass(item.provider)" />
        </div>

        <div class="min-w-0 flex-1">
          <div
            class="truncate text-[0.98rem] font-semibold leading-6 text-gray-900 dark:text-white"
          >
            {{ item.model_name }}
          </div>
        </div>
      </div>

      <button
        type="button"
        class="inline-flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-gray-200 text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:border-dark-600 dark:text-gray-400 dark:hover:bg-dark-700 dark:hover:text-white"
        :title="copyButtonTitle"
        @click="copyModelName"
      >
        <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.8">
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M15.75 17.25h3A2.25 2.25 0 0021 15V6.75a2.25 2.25 0 00-2.25-2.25h-8.25a2.25 2.25 0 00-2.25 2.25v3"
          />
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M5.25 8.25h8.25A2.25 2.25 0 0115.75 10.5v8.25A2.25 2.25 0 0113.5 21h-8.25A2.25 2.25 0 013 18.75V10.5a2.25 2.25 0 012.25-2.25z"
          />
        </svg>
      </button>
    </div>

    <div class="mt-3 min-h-[3.75rem] flex-1 space-y-0.5 pl-[3.25rem] text-[13px] leading-5 text-gray-700 dark:text-gray-300">
      <template v-if="pricingLines.length > 0">
        <div v-for="line in pricingLines" :key="line.label">
          {{ line.label }} {{ line.value }}
        </div>
      </template>
      <div v-else class="text-sm text-gray-400">
        {{ noPricingLabel }}
      </div>
    </div>

    <div
      v-if="mappingNoteEntries.length > 0"
      class="mt-3 space-y-1 pl-[3.25rem] text-xs leading-5 text-amber-700 dark:text-amber-300"
    >
      <div v-for="entry in mappingNoteEntries" :key="`${entry.channel_name}:${entry.note}`">
        <span v-if="showChannelNameInNotes" class="font-medium">{{ entry.channel_name }}：</span>
        {{ entry.note }}
      </div>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-2 pl-[3.25rem]">
      <span
        class="inline-flex items-center rounded-full bg-violet-50 px-2 py-0.5 text-[11px] font-medium text-violet-700 dark:bg-violet-900/20 dark:text-violet-300"
      >
        {{ billingModeLabel }}
      </span>
      <span
        v-if="intervalBadge"
        class="inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
      >
        {{ intervalBadge }}
      </span>
    </div>
  </article>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupPlatform } from '@/types'
import type { UserPricingInterval, UserSupportedModelPricing } from '@/api/channels'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { useAppStore } from '@/stores/app'
import type { MarketplaceModelItem } from '@/utils/modelMarketplace'
import { BILLING_MODE_IMAGE, BILLING_MODE_PER_REQUEST, BILLING_MODE_TOKEN } from '@/constants/channel'
import { formatScaled } from '@/utils/pricing'
import { platformIconClass } from '@/utils/platformColors'

const props = defineProps<{
  item: MarketplaceModelItem
  noPricingLabel: string
}>()

const { t } = useI18n()
const appStore = useAppStore()
const perMillionScale = 1_000_000
const copied = ref(false)

const primaryChannel = computed(() => props.item.channels.find((channel) => channel.pricing) ?? props.item.channels[0] ?? null)
const primaryPricing = computed(() => primaryChannel.value?.pricing ?? null)
const mappingNoteEntries = computed(() => {
  const seen = new Set<string>()
  return props.item.channels
    .map((channel) => ({
      channel_name: channel.channel_name,
      note: channel.model_note?.trim() || '',
    }))
    .filter((entry) => {
      if (!entry.note) return false
      const key = `${entry.channel_name}:${entry.note}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
})
const showChannelNameInNotes = computed(() => mappingNoteEntries.value.length > 1)

const billingModeLabel = computed(() => {
  const pricing = primaryPricing.value
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
})

const intervalBadge = computed(() => {
  const intervals = primaryPricing.value?.intervals ?? []
  if (intervals.length === 0) return ''
  return intervalSummary(intervals)
})

const copyButtonTitle = computed(() =>
  copied.value ? t('common.copied') : t('common.copy'),
)

async function copyModelName() {
  try {
    await navigator.clipboard.writeText(props.item.model_name)
    copied.value = true
    appStore.showSuccess(t('common.copiedToClipboard'))
    window.setTimeout(() => {
      copied.value = false
    }, 1200)
  } catch {
    copied.value = false
    appStore.showError(t('common.copyFailed'))
  }
}

function buildPricingLines(pricing: UserSupportedModelPricing | null): Array<{ label: string; value: string }> {
  if (!pricing) return []

  if (pricing.billing_mode === BILLING_MODE_TOKEN) {
    return [
      pricing.input_price != null
        ? { label: `${t('availableChannels.pricing.inputPrice')}`, value: `${formatScaled(pricing.input_price, perMillionScale)} / 1M Tokens` }
        : null,
      pricing.output_price != null
        ? { label: `${t('availableChannels.pricing.outputPrice')}`, value: `${formatScaled(pricing.output_price, perMillionScale)} / 1M Tokens` }
        : null,
      pricing.cache_read_price != null
        ? { label: `${t('availableChannels.pricing.cacheReadPrice')}`, value: `${formatScaled(pricing.cache_read_price, perMillionScale)} / 1M Tokens` }
        : null,
      pricing.cache_write_price != null
        ? { label: `${t('availableChannels.pricing.cacheWritePrice')}`, value: `${formatScaled(pricing.cache_write_price, perMillionScale)} / 1M Tokens` }
        : null,
    ].filter((line): line is { label: string; value: string } => !!line)
  }

  if (pricing.billing_mode === BILLING_MODE_PER_REQUEST || pricing.billing_mode === BILLING_MODE_IMAGE) {
    const lines: Array<{ label: string; value: string }> = []
    // 默认按次价格：image 计费在 admin 表单里写入 per_request_price，
    // image_output_price 是 token 模式下"图片输出 token 单价"的另一语义，
    // image 模式下不会被回填，因此两种 per-request 类计费统一读 per_request_price。
    if (pricing.per_request_price != null) {
      lines.push({
        label: `${t('availableChannels.pricing.perRequestPrice')}`,
        value: formatScaled(pricing.per_request_price, 1),
      })
    }
    for (const interval of pricing.intervals) {
      if (interval.per_request_price == null) continue
      const label = interval.tier_label?.trim() || formatRange(interval.min_tokens, interval.max_tokens)
      lines.push({
        label,
        value: formatScaled(interval.per_request_price, 1),
      })
    }
    return lines
  }

  return []
}

function formatRange(min: number, max: number | null): string {
  if (max == null) return t('modelMarketplace.intervalRangeOpen', { min: formatTokens(min) })
  return t('modelMarketplace.intervalRangeBounded', { min: formatTokens(min), max: formatTokens(max) })
}

function formatTokens(tokens: number): string {
  if (tokens >= 1_000_000) return `${formatScaled(tokens, 1_000_000)}M`
  if (tokens >= 1_000) return `${formatScaled(tokens, 1_000)}K`
  return String(tokens)
}

function intervalSummary(intervals: UserPricingInterval[]): string {
  const first = intervals[0]
  if (!first) return ''
  const count = t('modelMarketplace.intervalCount', { count: intervals.length })
  return `${formatRange(first.min_tokens, first.max_tokens)} · ${count}`
}

const pricingLines = computed(() => buildPricingLines(primaryPricing.value))
</script>
