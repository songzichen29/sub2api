<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.dataImportTitle')"
    width="wide"
    close-on-click-outside
    @close="handleClose"
  >
    <AccountDataImportForm
      ref="formRef"
      show-actions
      form-id="import-data-form"
      :proxies="proxies"
      :groups="groups"
      :available-tags="availableTags"
      :templates-loader="loadTemplates"
      :templates-saver="saveTemplates"
      :importer="adminAPI.accounts.importData"
      @cancel="handleClose"
      @imported="emit('imported')"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AccountDataImportForm from '@/components/admin/account/AccountDataImportForm.vue'
import { adminAPI } from '@/api/admin'
import type { AccountImportApplyTemplate } from '@/api/admin/settings'
import type { AdminGroup, Proxy as ProxyConfig } from '@/types'

interface Props {
  show: boolean
  proxies?: ProxyConfig[]
  groups?: AdminGroup[]
  availableTags?: string[]
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = withDefaults(defineProps<Props>(), {
  proxies: () => [],
  groups: () => [],
  availableTags: () => []
})
const emit = defineEmits<Emits>()
const { t } = useI18n()

const formRef = ref<InstanceType<typeof AccountDataImportForm> | null>(null)

const loadTemplates = async (): Promise<AccountImportApplyTemplate[]> => {
  return adminAPI.settings?.getAccountImportTemplates?.() ?? []
}

const saveTemplates = async (templates: AccountImportApplyTemplate[]): Promise<AccountImportApplyTemplate[]> => {
  return adminAPI.settings?.updateAccountImportTemplates?.(templates) ?? templates
}

watch(
  () => props.show,
  (open) => {
    if (open) {
      formRef.value?.resetForm()
    }
  }
)

const handleClose = () => {
  emit('close')
}
</script>
