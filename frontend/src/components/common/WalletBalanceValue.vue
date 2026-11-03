<template>
  <div class="relative" aria-label="钱包余额">
    <svg class="h-44 w-44" viewBox="0 0 160 160" aria-hidden="true">
      <circle cx="80" cy="80" r="68" fill="none" stroke="#2dd4bf" stroke-width="10" />
    </svg>
    <div class="absolute inset-0 flex items-center justify-center">
      <span class="text-2xl font-bold tabular-nums text-teal-500">{{ amount }}</span>
    </div>
  </div>
</template>
<script setup lang="ts">
defineProps<{ amount: string }>()
</script>
