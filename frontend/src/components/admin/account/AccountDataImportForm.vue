<template>
  <div class="space-y-4">
    <form :id="formId" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.dataImportHint') }}
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.accounts.dataImportWarning') }}
      </div>

      <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
        <div class="mb-3 flex items-center justify-between gap-3">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-white">
              {{ t('admin.accounts.dataImportTemplateTitle') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('admin.accounts.dataImportTemplateHint') }}
            </div>
          </div>
        </div>

        <div
          :class="[
            'grid grid-cols-1 gap-3',
            templatesSaver ? 'sm:grid-cols-[1fr_auto_auto]' : 'sm:grid-cols-1'
          ]"
        >
          <Select
            :model-value="selectedTemplateId"
            :options="templateOptions"
            :placeholder="t('admin.accounts.dataImportTemplateSelect')"
            @update:model-value="handleTemplateSelect"
          />
          <button v-if="templatesSaver" type="button" class="btn btn-secondary" @click="openSaveTemplateInput">
            {{ t('admin.accounts.dataImportTemplateSave') }}
          </button>
          <button
            v-if="templatesSaver"
            type="button"
            class="btn btn-secondary"
            :disabled="!selectedTemplateId"
            @click="deleteSelectedTemplate"
          >
            {{ t('common.delete') }}
          </button>
        </div>

        <div v-if="showTemplateNameInput" class="mt-3 flex flex-col gap-2 sm:flex-row">
          <input
            v-model.trim="pendingTemplateName"
            type="text"
            class="input flex-1"
            :placeholder="t('admin.accounts.dataImportTemplateNamePlaceholder')"
            @keydown.enter.prevent="confirmSaveTemplate"
            @keydown.esc.prevent="cancelSaveTemplate"
          />
          <div class="flex gap-2">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="!pendingTemplateName"
              @click="confirmSaveTemplate"
            >
              {{ t('common.confirm') }}
            </button>
            <button type="button" class="btn btn-secondary" @click="cancelSaveTemplate">
              {{ t('common.cancel') }}
            </button>
          </div>
        </div>
      </div>

      <div>
        <label class="input-label">{{ t('admin.accounts.dataImportFile') }}</label>
        <div
          class="flex items-center justify-between gap-3 rounded-lg border border-dashed px-4 py-3 transition-colors"
          :class="dragActive
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          @dragenter.prevent="handleDragEnter"
          @dragover.prevent
          @dragleave.prevent="handleDragLeave"
          @drop.prevent="handleDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200">
              {{ fileLabel || t('admin.accounts.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.accounts.dataImportMultiHint') }}</div>
          </div>
          <button type="button" class="btn btn-secondary shrink-0" @click="openFilePicker">
            {{ t('common.chooseFile') }}
          </button>
        </div>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          accept="application/json,application/zip,application/x-zip-compressed,.json,.zip"
          multiple
          @change="handleFileChange"
        />
      </div>

      <!-- 应用到所有账号（可选）：勾选即覆盖文件原值，未勾保留文件原值。
           参考 BulkEditAccountModal 的"启用 checkbox + 字段值"模式，不抽子组件以
           避免和 BulkEdit 现有内嵌结构强行共用。 -->
      <details class="rounded-xl border border-gray-200 dark:border-dark-700">
        <summary
          class="flex cursor-pointer items-center justify-between gap-2 px-4 py-3 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-800"
        >
          <span>{{ t('admin.accounts.dataImportApplyTitle') }}</span>
          <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.dataImportApplyHint') }}
          </span>
        </summary>
        <div class="space-y-4 border-t border-gray-200 p-4 dark:border-dark-700">
          <!-- Tags -->
          <div>
            <div class="mb-2 flex items-center justify-between">
              <label
                id="import-apply-tags-label"
                class="input-label mb-0"
                for="import-apply-tags-enabled"
              >
                {{ t('admin.accounts.dataImportApplyTags') }}
              </label>
              <input
                v-model="enableTags"
                id="import-apply-tags-enabled"
                type="checkbox"
                aria-controls="import-apply-tags-body"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </div>
            <div
              id="import-apply-tags-body"
              :class="!enableTags && 'pointer-events-none opacity-50'"
            >
              <AccountTagsInput
                v-model="applyTags"
                :suggestions="availableTags"
                :disabled="!enableTags"
                :placeholder="t('admin.accounts.dataImportApplyTagsPlaceholder')"
              />
              <p class="input-hint">{{ t('admin.accounts.dataImportApplyTagsHint') }}</p>
            </div>
          </div>

          <!-- Groups -->
          <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
            <div class="mb-2 flex items-center justify-between">
              <label
                id="import-apply-groups-label"
                class="input-label mb-0"
                for="import-apply-groups-enabled"
              >
                {{ t('admin.accounts.dataImportApplyGroups') }}
              </label>
              <input
                v-model="enableGroups"
                id="import-apply-groups-enabled"
                type="checkbox"
                aria-controls="import-apply-groups-body"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </div>
            <div
              id="import-apply-groups-body"
              :class="!enableGroups && 'pointer-events-none opacity-50'"
            >
              <GroupSelector
                v-model="applyGroupIds"
                :groups="groups"
                aria-labelledby="import-apply-groups-label"
              />
              <p class="input-hint">{{ t('admin.accounts.dataImportApplyGroupsHint') }}</p>
            </div>
          </div>

          <!-- Proxy -->
          <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
            <div class="mb-2 flex items-center justify-between">
              <label
                id="import-apply-proxy-label"
                class="input-label mb-0"
                for="import-apply-proxy-enabled"
              >
                {{ t('admin.accounts.dataImportApplyProxy') }}
              </label>
              <input
                v-model="enableProxy"
                id="import-apply-proxy-enabled"
                type="checkbox"
                aria-controls="import-apply-proxy-body"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </div>
            <div
              id="import-apply-proxy-body"
              :class="!enableProxy && 'pointer-events-none opacity-50'"
            >
              <ProxySelector
                v-model="applyProxyId"
                :proxies="proxies"
                :disabled="!enableProxy"
                :allow-test="allowProxyTest"
              />
              <p class="input-hint">{{ t('admin.accounts.dataImportApplyProxyHint') }}</p>
            </div>
          </div>

          <!-- Concurrency + Priority（共一行）-->
          <div class="grid grid-cols-1 gap-4 border-t border-gray-200 pt-4 dark:border-dark-700 sm:grid-cols-2">
            <div>
              <div class="mb-2 flex items-center justify-between">
                <label
                  id="import-apply-concurrency-label"
                  class="input-label mb-0"
                  for="import-apply-concurrency-enabled"
                >
                  {{ t('admin.accounts.dataImportApplyConcurrency') }}
                </label>
                <input
                  v-model="enableConcurrency"
                  id="import-apply-concurrency-enabled"
                  type="checkbox"
                  aria-controls="import-apply-concurrency"
                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
              </div>
              <input
                v-model.number="applyConcurrency"
                id="import-apply-concurrency"
                type="number"
                min="1"
                :disabled="!enableConcurrency"
                class="input"
                :class="!enableConcurrency && 'cursor-not-allowed opacity-50'"
                aria-labelledby="import-apply-concurrency-label"
              />
            </div>
            <div>
              <div class="mb-2 flex items-center justify-between">
                <label
                  id="import-apply-priority-label"
                  class="input-label mb-0"
                  for="import-apply-priority-enabled"
                >
                  {{ t('admin.accounts.dataImportApplyPriority') }}
                </label>
                <input
                  v-model="enablePriority"
                  id="import-apply-priority-enabled"
                  type="checkbox"
                  aria-controls="import-apply-priority"
                  class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
              </div>
              <input
                v-model.number="applyPriority"
                id="import-apply-priority"
                type="number"
                min="1"
                :disabled="!enablePriority"
                class="input"
                :class="!enablePriority && 'cursor-not-allowed opacity-50'"
                aria-labelledby="import-apply-priority-label"
              />
            </div>
          </div>

          <!-- Model restriction -->
          <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
            <div class="mb-2 flex items-center justify-between">
              <label
                id="import-apply-model-label"
                class="input-label mb-0"
                for="import-apply-model-enabled"
              >
                {{ t('admin.accounts.dataImportApplyModelRestriction') }}
              </label>
              <input
                v-model="enableModelRestriction"
                id="import-apply-model-enabled"
                type="checkbox"
                aria-controls="import-apply-model-body"
                class="rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </div>
            <div
              id="import-apply-model-body"
              :class="!enableModelRestriction && 'pointer-events-none opacity-50'"
            >
              <div class="mb-3 flex gap-2">
                <button
                  type="button"
                  :disabled="!enableModelRestriction"
                  :class="[
                    'flex-1 rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                    modelRestrictionMode === 'whitelist'
                      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400'
                  ]"
                  @click="modelRestrictionMode = 'whitelist'"
                >
                  {{ t('admin.accounts.modelWhitelist') }}
                </button>
                <button
                  type="button"
                  :disabled="!enableModelRestriction"
                  :class="[
                    'flex-1 rounded-lg px-3 py-1.5 text-sm font-medium transition-all',
                    modelRestrictionMode === 'mapping'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400'
                  ]"
                  @click="modelRestrictionMode = 'mapping'"
                >
                  {{ t('admin.accounts.modelMapping') }}
                </button>
              </div>

              <div v-if="modelRestrictionMode === 'whitelist'">
                <ModelWhitelistSelector v-model="allowedModels" />
                <p class="input-hint">
                  {{ t('admin.accounts.selectedModels', { count: allowedModels.length }) }}
                </p>
              </div>

              <div v-else>
                <div v-if="modelMappings.length > 0" class="mb-3 space-y-2">
                  <div
                    v-for="(mapping, index) in modelMappings"
                    :key="index"
                    class="flex items-center gap-2"
                  >
                    <input
                      v-model="mapping.from"
                      type="text"
                      class="input flex-1"
                      :placeholder="t('admin.accounts.requestModel')"
                      :disabled="!enableModelRestriction"
                    />
                    <span class="text-gray-400">→</span>
                    <input
                      v-model="mapping.to"
                      type="text"
                      class="input flex-1"
                      :placeholder="t('admin.accounts.actualModel')"
                      :disabled="!enableModelRestriction"
                    />
                    <button
                      type="button"
                      class="rounded-lg p-2 text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20"
                      :disabled="!enableModelRestriction"
                      @click="removeModelMapping(index)"
                    >
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>
                <button
                  type="button"
                  class="w-full rounded-lg border-2 border-dashed border-gray-300 px-4 py-2 text-sm text-gray-600 hover:border-gray-400 dark:border-dark-500 dark:text-gray-400"
                  :disabled="!enableModelRestriction"
                  @click="addModelMapping"
                >
                  + {{ t('admin.accounts.addMapping') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </details>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.dataImportErrors') }}
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} — {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>


  <div v-if="showActions" class="flex justify-end gap-3">
    <button v-if="showCancelAction" class="btn btn-secondary" type="button" :disabled="importing" @click="handleCancel">
      {{ t('common.cancel') }}
    </button>
    <button class="btn btn-primary" type="submit" :form="formId" :disabled="importing">
      {{ importing ? t('admin.accounts.dataImporting') : t('admin.accounts.dataImportButton') }}
    </button>
  </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ProxySelector from '@/components/common/ProxySelector.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import AccountTagsInput from '@/components/account/AccountTagsInput.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AccountImportApplyTemplate } from '@/api/admin/settings'
import { useAppStore } from '@/stores/app'
import { buildModelMappingObject } from '@/composables/useModelWhitelist'
import { mergeDataPayloads, parseImportFiles } from './accountDataImportFiles'
import type {
  AdminDataPayload,
  AdminDataImportApply,
  AdminDataImportResult,
  AdminGroup,
  Proxy as ProxyConfig
} from '@/types'

type ImportApplyTemplate = AccountImportApplyTemplate

interface ImportDataPayload {
  data: AdminDataPayload
  skip_default_group_bind?: boolean
  apply?: AdminDataImportApply
}

interface Props {
  // Options for the apply-to-all panel.
  proxies?: ProxyConfig[]
  groups?: AdminGroup[]
  availableTags?: string[]
  templatesLoader: () => Promise<AccountImportApplyTemplate[]>
  templatesSaver?: (templates: AccountImportApplyTemplate[]) => Promise<AccountImportApplyTemplate[]>
  importer: (payload: ImportDataPayload) => Promise<AdminDataImportResult>
  formId?: string
  showActions?: boolean
  allowProxyTest?: boolean
  showCancelAction?: boolean
}

interface Emits {
  (e: 'cancel'): void
  (e: 'imported'): void
}

const props = withDefaults(defineProps<Props>(), {
  proxies: () => [],
  groups: () => [],
  availableTags: () => [],
  formId: 'account-data-import-form',
  showActions: false,
  allowProxyTest: true,
  showCancelAction: true
})
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()
const templatesSaver = computed(() => props.templatesSaver)

const importing = ref(false)
const files = ref<File[]>([])
const dragDepth = ref(0)
const result = ref<AdminDataImportResult | null>(null)

const fileInput = ref<HTMLInputElement | null>(null)
const dragActive = computed(() => dragDepth.value > 0)
const fileLabel = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0].name
  return t('admin.accounts.dataImportFilesSelected', { count: files.value.length })
})

const errorItems = computed(() => result.value?.errors || [])

const templates = ref<ImportApplyTemplate[]>([])
const selectedTemplateId = ref('')
const showTemplateNameInput = ref(false)
const pendingTemplateName = ref('')

const templateOptions = computed(() => [
  { value: '', label: t('admin.accounts.dataImportTemplateSelect') },
  ...templates.value.map((item) => ({ value: item.id, label: item.name }))
])

// ===== Apply 块状态：6 个 enable + 字段值 =====
// 启用语义：勾选某字段 → 该字段值会作为 apply 块的一部分发到后端，覆盖文件原值；
// 未勾选 → 该字段从 payload 整体省略，后端按指针 nil 判定为"不应用"。
const enableTags = ref(false)
const enableGroups = ref(false)
const enableProxy = ref(false)
const enableConcurrency = ref(false)
const enablePriority = ref(false)
const enableModelRestriction = ref(false)

const applyTags = ref<string[]>([])
const applyGroupIds = ref<number[]>([])
const availableGroupIdSet = computed(() => new Set(props.groups.map((group) => group.id)))
const sanitizeGroupIds = (ids: number[] | undefined | null): number[] => {
  if (!Array.isArray(ids)) return []
  const allowed = availableGroupIdSet.value
  const seen = new Set<number>()
  const result: number[] = []
  for (const id of ids) {
    if (!allowed.has(id) || seen.has(id)) continue
    seen.add(id)
    result.push(id)
  }
  return result
}
const applyProxyId = ref<number | null>(null)
const applyConcurrency = ref<number>(1)
const applyPriority = ref<number>(1)
const modelRestrictionMode = ref<'whitelist' | 'mapping'>('whitelist')
const allowedModels = ref<string[]>([])
const modelMappings = ref<{ from: string; to: string }[]>([])

const buildTemplateSnapshot = (): Omit<AccountImportApplyTemplate, 'id' | 'name'> => ({
  enableTags: enableTags.value,
  enableGroups: enableGroups.value,
  enableProxy: enableProxy.value,
  enableConcurrency: enableConcurrency.value,
  enablePriority: enablePriority.value,
  enableModelRestriction: enableModelRestriction.value,
  applyTags: [...applyTags.value],
  applyGroupIds: sanitizeGroupIds(applyGroupIds.value),
  applyProxyId: applyProxyId.value,
  applyConcurrency: applyConcurrency.value,
  applyPriority: applyPriority.value,
  modelRestrictionMode: modelRestrictionMode.value,
  allowedModels: [...allowedModels.value],
  modelMappings: modelMappings.value.map((item) => ({ ...item }))
})

const applyTemplateSnapshot = (snapshot: Omit<AccountImportApplyTemplate, 'id' | 'name'>) => {
  enableTags.value = snapshot.enableTags
  enableGroups.value = snapshot.enableGroups
  enableProxy.value = snapshot.enableProxy
  enableConcurrency.value = snapshot.enableConcurrency
  enablePriority.value = snapshot.enablePriority
  enableModelRestriction.value = snapshot.enableModelRestriction
  applyTags.value = [...snapshot.applyTags]
  applyGroupIds.value = sanitizeGroupIds(snapshot.applyGroupIds)
  applyProxyId.value = snapshot.applyProxyId
  applyConcurrency.value = snapshot.applyConcurrency
  applyPriority.value = snapshot.applyPriority
  modelRestrictionMode.value = snapshot.modelRestrictionMode
  allowedModels.value = [...snapshot.allowedModels]
  modelMappings.value = snapshot.modelMappings.map((item) => ({ ...item }))
}

const resetApplyState = () => {
  enableTags.value = false
  enableGroups.value = false
  enableProxy.value = false
  enableConcurrency.value = false
  enablePriority.value = false
  enableModelRestriction.value = false
  applyTags.value = []
  applyGroupIds.value = []
  applyProxyId.value = null
  applyConcurrency.value = 1
  applyPriority.value = 1
  modelRestrictionMode.value = 'whitelist'
  allowedModels.value = []
  modelMappings.value = []
}

watch(
  () => props.groups,
  () => {
    applyGroupIds.value = sanitizeGroupIds(applyGroupIds.value)
  },
  { deep: true }
)

const loadTemplates = () => {
  props.templatesLoader()
    .then((items) => {
      templates.value = items
    })
    .catch(() => {
      templates.value = []
    })
}

const persistTemplates = async () => {
  if (!props.templatesSaver) return
  templates.value = await props.templatesSaver(templates.value)
}

const persistSelectedTemplateSnapshot = async () => {
  if (!props.templatesSaver || !selectedTemplateId.value) return
  const index = templates.value.findIndex((item) => item.id === selectedTemplateId.value)
  if (index < 0) return

  const previousTemplates = templates.value
  const selected = previousTemplates[index]
  templates.value = previousTemplates.map((item, itemIndex) =>
    itemIndex === index
      ? {
          id: selected.id,
          name: selected.name,
          ...buildTemplateSnapshot()
        }
      : item
  )

  try {
    await persistTemplates()
  } catch (error) {
    templates.value = previousTemplates
    throw error
  }
}


const handleTemplateSelect = (value: string | number | boolean | null) => {
  selectedTemplateId.value = typeof value === 'string' ? value : ''
  if (!selectedTemplateId.value) return
  const selected = templates.value.find((item) => item.id === selectedTemplateId.value)
  if (!selected) return
  applyTemplateSnapshot(selected)
}

const openSaveTemplateInput = () => {
  pendingTemplateName.value = ''
  showTemplateNameInput.value = true
}

const cancelSaveTemplate = () => {
  pendingTemplateName.value = ''
  showTemplateNameInput.value = false
}

const confirmSaveTemplate = () => {
  const name = pendingTemplateName.value.trim()
  if (!name) return
  const template: ImportApplyTemplate = {
    id: `tpl_${Date.now()}`,
    name,
    ...buildTemplateSnapshot()
  }
  templates.value = [template, ...templates.value]
  selectedTemplateId.value = template.id
  persistTemplates().then(() => {
    cancelSaveTemplate()
    appStore.showSuccess(t('admin.accounts.dataImportTemplateSaved'))
  })
}

const deleteSelectedTemplate = () => {
  if (!selectedTemplateId.value) return
  templates.value = templates.value.filter((item) => item.id !== selectedTemplateId.value)
  selectedTemplateId.value = ''
  persistTemplates().then(() => {
    appStore.showSuccess(t('admin.accounts.dataImportTemplateDeleted'))
  })
}

const addModelMapping = () => {
  modelMappings.value.push({ from: '', to: '' })
}
const removeModelMapping = (index: number) => {
  modelMappings.value.splice(index, 1)
}

const resetForm = () => {
  files.value = []
  dragDepth.value = 0
  result.value = null
  loadTemplates()
  selectedTemplateId.value = ''
  cancelSaveTemplate()
  if (fileInput.value) {
    fileInput.value.value = ''
  }
  resetApplyState()
}

defineExpose({ resetForm })

watch(
  () => props.importer,
  () => {
    resetForm()
  }
)

resetForm()


const openFilePicker = () => {
  fileInput.value?.click()
}

const isImportFile = (sourceFile: File): boolean => {
  const name = sourceFile.name.toLowerCase()
  const mime = sourceFile.type.toLowerCase()
  return (
    name.endsWith('.json') ||
    name.endsWith('.zip') ||
    mime === 'application/json' ||
    mime === 'application/zip' ||
    mime === 'application/x-zip' ||
    mime === 'application/x-zip-compressed' ||
    mime === 'multipart/x-zip'
  )
}

const setSelectedFiles = (sourceFiles: FileList | File[] | null | undefined) => {
  if (importing.value) return
  const incoming = Array.from(sourceFiles || [])
  const picked = incoming.filter(isImportFile)
  if (!picked.length) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }
  if (picked.length < incoming.length) {
    appStore.showWarning(
      t('admin.accounts.dataImportIgnoredFiles', { count: incoming.length - picked.length })
    )
  }
  files.value = picked
  result.value = null
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(target.files)
  target.value = ''
}

const handleDragEnter = () => {
  if (importing.value) return
  dragDepth.value += 1
}

const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

const handleDrop = (event: DragEvent) => {
  dragDepth.value = 0
  if (importing.value) return
  setSelectedFiles(event.dataTransfer?.files)
}

const handleCancel = () => {
  if (importing.value) return
  emit('cancel')
}

/**
 * 构造导入应用块（Apply）的 payload。
 *
 * 关键约束（design 第 3.2 节约束 5/7）：未勾选的字段必须从对象里整体省略，而不是
 * 发 null/0/[] 默认值——后端按指针非 nil 判定是否应用，发默认值会误触发"显式
 * 清空"语义。proxy_id 特殊：勾选了但用户没在 selector 里选实际代理时，apply
 * 里也不放 proxy_id（避免误传 0 触发"显式不绑代理"）。
 *
 * 返回 undefined 时上层会让 axios 不发 apply 字段，POST body 里完全没有 apply 键。
 */
const buildApplyPayload = (): AdminDataImportApply | undefined => {
  const apply: AdminDataImportApply = {}

  if (enableTags.value) {
    // 显式 [] 也是合法的"清空"语义——后端会清空所有导入账号的 tags
    apply.tags = [...applyTags.value]
  }

  if (enableGroups.value) {
    apply.group_ids = sanitizeGroupIds(applyGroupIds.value)
  }

  if (enableProxy.value && applyProxyId.value !== null) {
    apply.proxy_id = applyProxyId.value
  }

  if (enableConcurrency.value && Number.isFinite(applyConcurrency.value)) {
    apply.concurrency = applyConcurrency.value
  }

  if (enablePriority.value && Number.isFinite(applyPriority.value)) {
    apply.priority = applyPriority.value
  }

  if (enableModelRestriction.value) {
    // 合并白名单和映射（combined 模式）——用户可能在模板里同时填了两者，
    // 之前按 mode 二选一会导致只有一侧生效（#issue: 模板白名单和映射都有值但导入账号只有一个生效）
    apply.model_mapping = buildModelMappingObject(
      'combined',
      allowedModels.value,
      modelMappings.value
    ) ?? {}
  }

  return Object.keys(apply).length > 0 ? apply : undefined
}

const handleImport = async () => {
  if (files.value.length === 0) {
    appStore.showError(t('admin.accounts.dataImportSelectFile'))
    return
  }

  importing.value = true
  try {
    const payloads = await parseImportFiles(files.value)
    const dataPayload = mergeDataPayloads(payloads)

    const apply = buildApplyPayload()
    await persistSelectedTemplateSnapshot()

    const res = await props.importer({
      data: dataPayload,
      skip_default_group_bind: true,
      apply
    })

    result.value = res

    const msgParams: Record<string, unknown> = {
      account_created: res.account_created,
      account_failed: res.account_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
    }
    if (res.account_failed > 0 || res.proxy_failed > 0) {
      appStore.showError(t('admin.accounts.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.accounts.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.accounts.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.accounts.dataImportFailed'))
    }
  } finally {
    importing.value = false
  }
}
</script>
