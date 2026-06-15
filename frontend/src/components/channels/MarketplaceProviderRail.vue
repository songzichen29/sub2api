<template>
  <aside class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
    <div class="mb-4 flex items-center justify-between">
      <div>
        <h2 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ title }}
        </h2>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ subtitle }}
        </p>
      </div>

      <button type="button" class="btn btn-ghost btn-sm" @click="$emit('reset')">
        {{ resetLabel }}
      </button>
    </div>

    <div class="space-y-2">
      <button
        type="button"
        class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm transition-colors"
        :class="
          selectedProvider === ''
            ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300'
            : 'border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'
        "
        @click="$emit('select', '')"
      >
        <span>{{ allLabel }}</span>
        <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          {{ totalCount }}
        </span>
      </button>

      <button
        v-for="facet in facets"
        :key="facet.provider"
        type="button"
        class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm transition-colors"
        :class="
          selectedProvider === facet.provider
            ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300'
            : 'border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'
        "
        @click="$emit('select', facet.provider)"
      >
        <span class="flex min-w-0 items-center gap-2">
          <PlatformIcon :platform="facet.provider as GroupPlatform" size="sm" :class="platformIconClass(facet.provider)" />
          <span class="truncate">{{ platformLabel(facet.provider) }}</span>
        </span>
        <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
          {{ facet.model_count }}
        </span>
      </button>
    </div>

    <div class="mt-6 border-t border-gray-200 pt-4 dark:border-dark-700">
      <div class="mb-3">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ groupTitle }}
        </h3>
      </div>

      <div class="mb-3">
        <input
          :value="groupQuery"
          type="text"
          :placeholder="groupSearchPlaceholder"
          class="input"
          @input="handleGroupQueryInput"
        />
      </div>

      <div class="max-h-80 space-y-2 overflow-y-auto pr-1">
        <button
          type="button"
          class="flex w-full items-center justify-between rounded-lg border px-3 py-2 text-left text-sm transition-colors"
          :class="
            selectedGroupId == null
              ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'
          "
          @click="$emit('select-group', null)"
        >
          <span>{{ allGroupsLabel }}</span>
        </button>

        <button
          v-for="facet in groupFacets"
          :key="facet.id"
          type="button"
          class="w-full rounded-lg border px-3 py-2 text-left transition-colors"
          :class="
            selectedGroupId === facet.id
              ? 'border-primary-200 bg-primary-50 text-primary-700 dark:border-primary-800 dark:bg-primary-900/20 dark:text-primary-300'
              : 'border-gray-200 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700'
          "
          @click="$emit('select-group', facet.id)"
        >
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <div class="flex min-w-0 items-center gap-2">
                <span class="truncate text-sm font-medium">{{ facet.name }}</span>
                <span
                  class="inline-flex flex-shrink-0 items-center rounded-full bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-600 dark:bg-dark-700 dark:text-gray-300"
                >
                  {{ formatRateMultiplier(facet.rate_multiplier) }}
                </span>
              </div>
              <p
                v-if="facet.description"
                class="mt-1 line-clamp-2 text-xs leading-4 text-gray-500 dark:text-gray-400"
              >
                {{ facet.description }}
              </p>
            </div>
            <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ facet.model_count }}
            </span>
          </div>
        </button>

        <div
          v-if="groupFacets.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-3 py-4 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
        >
          {{ emptyGroupsLabel }}
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import type { GroupPlatform } from '@/types'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { MarketplaceGroupFacet, MarketplaceProviderFacet } from '@/utils/modelMarketplace'
import { platformIconClass, platformLabel } from '@/utils/platformColors'
defineProps<{
  facets: MarketplaceProviderFacet[]
  groupFacets: MarketplaceGroupFacet[]
  selectedProvider: string
  selectedGroupId: number | null
  totalCount: number
  title: string
  subtitle: string
  allLabel: string
  resetLabel: string
  groupTitle: string
  groupQuery: string
  groupSearchPlaceholder: string
  allGroupsLabel: string
  emptyGroupsLabel: string
}>()

const emit = defineEmits<{
  (e: 'select', provider: string): void
  (e: 'select-group', groupId: number | null): void
  (e: 'update:groupQuery', value: string): void
  (e: 'reset'): void
}>()

function handleGroupQueryInput(event: Event) {
  emit('update:groupQuery', (event.target as HTMLInputElement).value)
}

function formatRateMultiplier(value: number): string {
  if (!Number.isFinite(value)) return '-'
  return `${value}x`
}
</script>
