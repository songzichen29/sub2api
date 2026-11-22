<template>
  <BaseDialog :show="show" :title="title" width="narrow" :close-on-escape="!pending" :show-close-button="!pending" @close="handleCancel">
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ message }}</p>
      <slot></slot>
    </div>

    <template #footer>
      <div class="flex justify-end space-x-3">
        <button
          @click="handleCancel"
          type="button"
          :disabled="pending"
          class="btn btn-secondary"
        >
          {{ cancelText }}
        </button>
        <button
          @click="handleConfirm"
          type="button"
          :disabled="pending"
          :aria-busy="pending"
          :class="['btn', danger ? 'btn-danger' : 'btn-primary']"
        >
          {{ pending ? t('common.processing') : confirmText }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'

const { t } = useI18n()

interface Props {
  show: boolean
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  loading?: boolean
  onConfirm?: () => unknown
}

interface Emits {
  (e: 'confirm'): void
  (e: 'cancel'): void
}

const props = withDefaults(defineProps<Props>(), {
  danger: false,
  loading: false
})

const confirmText = computed(() => props.confirmText || t('common.confirm'))
const cancelText = computed(() => props.cancelText || t('common.cancel'))
const confirming = ref(false)
const pending = computed(() => props.loading || confirming.value)

const emit = defineEmits<Emits>()

const handleConfirm = async () => {
  if (pending.value) return
  confirming.value = true
  try {
    // @confirm 作为回调传入，等待其返回的 Promise 后再解除确认锁。
    await props.onConfirm?.()
  } finally {
    confirming.value = false
  }
}

const handleCancel = () => {
  if (pending.value) return
  emit('cancel')
}
</script>
