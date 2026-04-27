<template>
  <div class="min-h-screen bg-gray-50 dark:bg-dark-950">
    <div class="relative min-h-screen">
      <AppHeader :show-mobile-sidebar-toggle="false" />

      <main class="px-4 py-5 md:px-6 lg:px-8 lg:py-6">
        <slot />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import AppHeader from './AppHeader.vue'
</script>
