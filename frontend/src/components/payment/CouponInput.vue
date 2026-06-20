<template>
  <div class="space-y-2">
    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ label }}
    </label>
    <div class="flex gap-2">
      <input
        :value="modelValue"
        class="input flex-1"
        :placeholder="placeholder"
        autocomplete="off"
        @input="handleInput"
      />
      <button
        type="button"
        class="btn btn-secondary px-4"
        :disabled="loading || !modelValue.trim()"
        @click="$emit('apply')"
      >
        {{ loading ? loadingText : applyText }}
      </button>
    </div>
    <p v-if="message" class="text-xs" :class="error ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">
      {{ message }}
    </p>
  </div>
</template>

<script setup lang="ts">
withDefaults(defineProps<{
  modelValue: string
  loading?: boolean
  error?: boolean
  message?: string
  label?: string
  placeholder?: string
  applyText?: string
  loadingText?: string
}>(), {
  loading: false,
  error: false,
  message: '',
  label: '优惠券',
  placeholder: '输入优惠券码',
  applyText: '应用',
  loadingText: '验证中',
})

const emit = defineEmits<{
  'update:modelValue': [value: string]
  apply: []
}>()

function handleInput(event: Event) {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>
