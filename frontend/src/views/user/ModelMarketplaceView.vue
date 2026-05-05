<template>
  <HeaderOnlyLayout>
    <div class="mx-auto max-w-[1680px]">
      <div class="grid min-h-[calc(100vh-7.5rem)] gap-6 xl:grid-cols-[280px,minmax(0,1fr)]">
        <div class="xl:sticky xl:top-24 xl:self-start">
          <MarketplaceProviderRail
            :facets="providerFacets"
            :group-facets="filteredGroupFacets"
            :selected-provider="selectedProvider"
            :selected-group-id="selectedGroupId"
            :total-count="modelItems.length"
            :title="t('modelMarketplace.filtersTitle')"
            :subtitle="t('modelMarketplace.filtersSubtitle')"
            :all-label="t('modelMarketplace.allProviders')"
            :reset-label="t('common.reset')"
            :group-title="t('modelMarketplace.groupFiltersTitle')"
            :group-query="groupQuery"
            :group-search-placeholder="t('modelMarketplace.groupSearchPlaceholder')"
            :all-groups-label="t('modelMarketplace.allGroups')"
            :empty-groups-label="t('modelMarketplace.empty.noGroups')"
            @select="selectedProvider = $event"
            @select-group="selectedGroupId = $event"
            @update:group-query="groupQuery = $event"
            @reset="resetFilters"
          />
        </div>

        <section class="flex min-h-[640px] flex-col gap-4">
          <div class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
              <div class="min-w-0">
                <button
                  type="button"
                  class="mb-3 inline-flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-white"
                  @click="goBack"
                >
                  <Icon name="chevronLeft" size="sm" />
                  {{ t('common.back') }}
                </button>

                <div class="flex flex-wrap items-center gap-2">
                  <h1 class="text-xl font-semibold text-gray-900 dark:text-white">
                    {{ selectedProviderTitle }}
                  </h1>
                  <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                    {{ t('modelMarketplace.totalModels', { count: total }) }}
                  </span>
                </div>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                  {{ t('modelMarketplace.description') }}
                </p>
              </div>

              <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
                <div class="relative min-w-[16rem] flex-1">
                  <Icon
                    name="search"
                    size="md"
                    class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
                  />
                  <input
                    v-model="searchQuery"
                    type="text"
                    :placeholder="t('modelMarketplace.modelSearchPlaceholder')"
                    class="input pl-10"
                  />
                </div>

                <button
                  @click="loadChannels"
                  :disabled="loading"
                  class="btn btn-secondary"
                  :title="t('common.refresh')"
                >
                  <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
                </button>
              </div>
            </div>
          </div>

          <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
            <div class="flex-1 overflow-y-auto p-4">
              <div
                v-if="loading"
                class="flex h-full min-h-[420px] items-center justify-center text-center"
              >
                <Icon name="refresh" size="lg" class="mx-auto animate-spin text-gray-400" />
              </div>

              <div
                v-else-if="modelItems.length === 0"
                class="flex h-full min-h-[420px] items-center justify-center text-center"
              >
                <p class="text-sm text-gray-500 dark:text-gray-400">
                  {{ emptyLabel }}
                </p>
              </div>

              <div
                v-else
                class="grid grid-cols-1 gap-3 xl:grid-cols-3"
              >
                <MarketplaceModelCard
                  v-for="item in modelItems"
                  :key="item.key"
                  :item="item"
                  :no-pricing-label="t('availableChannels.noPricing')"
                />
              </div>
            </div>

            <div class="border-t border-gray-200 dark:border-dark-700">
              <Pagination
                v-if="total > 0"
                :total="total"
                :page="page"
                :page-size="pageSize"
                @update:page="page = $event"
                @update:page-size="handlePageSizeChange"
              />
            </div>
          </div>
        </section>
      </div>
    </div>
  </HeaderOnlyLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import HeaderOnlyLayout from '@/components/layout/HeaderOnlyLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import MarketplaceProviderRail from '@/components/channels/MarketplaceProviderRail.vue'
import MarketplaceModelCard from '@/components/channels/MarketplaceModelCard.vue'
import modelMarketplaceAPI from '@/api/modelMarketplace'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { platformLabel } from '@/utils/platformColors'
import {
  type MarketplaceGroupFacet,
  type MarketplaceModelItem,
  type MarketplaceProviderFacet,
} from '@/utils/modelMarketplace'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const modelItems = ref<MarketplaceModelItem[]>([])
const providerFacets = ref<MarketplaceProviderFacet[]>([])
const groupFacets = ref<MarketplaceGroupFacet[]>([])
const total = ref(0)
const loading = ref(false)
const searchQuery = ref('')
const selectedProvider = ref('')
const selectedGroupId = ref<number | null>(null)
const groupQuery = ref('')
const page = ref(1)
const pageSize = ref(20)
const requestController = ref<AbortController | null>(null)
let reloadTimer: ReturnType<typeof setTimeout> | null = null

const filteredGroupFacets = computed(() => {
  const query = groupQuery.value.trim().toLowerCase()
  if (!query) return groupFacets.value
  return groupFacets.value.filter((facet) => facet.name.toLowerCase().includes(query))
})

const selectedProviderTitle = computed(() =>
  selectedProvider.value ? platformLabel(selectedProvider.value) : t('modelMarketplace.allProviders'),
)

const emptyLabel = computed(() => {
  if (total.value === 0 && !searchQuery.value && !selectedProvider.value && selectedGroupId.value == null) {
    return t('modelMarketplace.empty.noChannels')
  }
  return t('modelMarketplace.empty.noModelResults')
})

function resetFilters() {
  searchQuery.value = ''
  selectedProvider.value = ''
  selectedGroupId.value = null
  groupQuery.value = ''
  page.value = 1
}

function handlePageSizeChange(value: number) {
  pageSize.value = value
  page.value = 1
}

function goBack() {
  if (window.history.length > 1) {
    router.back()
    return
  }
  router.push(authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
}

async function loadChannels() {
  requestController.value?.abort()
  const controller = new AbortController()
  requestController.value = controller
  loading.value = true
  try {
    const result = await modelMarketplaceAPI.list(
      {
        page: page.value,
        page_size: pageSize.value,
        search: searchQuery.value.trim() || undefined,
        provider: selectedProvider.value || undefined,
        group_id: selectedGroupId.value,
      },
      { signal: controller.signal },
    )

    modelItems.value = result.items
    providerFacets.value = result.provider_facets
    groupFacets.value = result.group_facets
    total.value = result.total
  } catch (err: unknown) {
    if (
      typeof err === 'object' &&
      err !== null &&
      'code' in err &&
      (err as { code?: string }).code === 'ERR_CANCELED'
    ) {
      return
    }
    if (err instanceof Error && err.name === 'CanceledError') return
    if (err instanceof DOMException && err.name === 'AbortError') return
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    if (requestController.value === controller) {
      loading.value = false
      requestController.value = null
    }
  }
}

function scheduleReload() {
  if (reloadTimer) {
    clearTimeout(reloadTimer)
  }

  reloadTimer = setTimeout(() => {
    void loadChannels()
  }, 250)
}

// skipNextGroupReload 用于在「切换供应商」流程中静音 selectedGroupId 自身的 watch：
// 切供应商时需要先把旧 group 清空（否则 group_id 过滤会落到空结果），但这次清空
// 不应触发额外的 reload——下方 selectedProvider 的 watch 会手动 await loadChannels。
let skipNextGroupReload = false

watch(searchQuery, () => {
  page.value = 1
  scheduleReload()
})

watch(selectedGroupId, () => {
  if (skipNextGroupReload) {
    skipNextGroupReload = false
    return
  }
  page.value = 1
  scheduleReload()
})

// 切换供应商时：清空旧分组、拉取该供应商下的 group_facets，再自动选中第一个分组。
// 旧 group_id 若保留会让后端 filterMarketplaceItemsByGroup 命中为空 → 页面空白；
// 切到"全部供应商"（newProvider === ''）时保持 selectedGroupId = null，避免在
// 跨供应商聚合视图里强行钉一个分组导致歧义。
watch(selectedProvider, async (newProvider, oldProvider) => {
  if (newProvider === oldProvider) return

  page.value = 1

  // 取消未触发的防抖请求，避免和下面 await loadChannels 抢资源
  if (reloadTimer) {
    clearTimeout(reloadTimer)
    reloadTimer = null
  }

  // 重置旧分组但不触发 selectedGroupId 自身的 reload（下方手动 loadChannels）
  if (selectedGroupId.value !== null) {
    skipNextGroupReload = true
    selectedGroupId.value = null
  }

  await loadChannels()

  // 竞态保护：await 期间用户又切了一次供应商时，放弃后续默认选择
  if (selectedProvider.value !== newProvider) return

  if (newProvider && groupFacets.value.length > 0) {
    selectedGroupId.value = groupFacets.value[0].id
  }
})

watch(pageSize, () => {
  page.value = 1
  scheduleReload()
})

watch(page, () => {
  scheduleReload()
})

onMounted(loadChannels)
onBeforeUnmount(() => {
  if (reloadTimer) {
    clearTimeout(reloadTimer)
  }
  requestController.value?.abort()
})
</script>
