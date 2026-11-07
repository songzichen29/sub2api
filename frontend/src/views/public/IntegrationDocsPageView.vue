<template>
  <IntegrationDocsView v-if="embedded" embedded />
  <AppLayout v-else>
    <iframe
      :src="frameUrl"
      :title="t('docs.title')"
      allow="clipboard-write"
      class="docs-frame block w-full border-0 bg-white dark:bg-dark-950"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { setLocale } from '@/i18n'
import { buildEmbeddedUrl, detectTheme } from '@/utils/embedded-url'
import IntegrationDocsView from './IntegrationDocsView.vue'

const route = useRoute()
const { t, locale } = useI18n()
const embedded = computed(() => route.query.ui_mode === 'embedded' || window.self !== window.top)
const theme = ref(detectTheme())
const frameUrl = computed(() => {
  const url = new URL('/docs', window.location.origin)
  url.hash = route.hash
  return buildEmbeddedUrl(url.toString(), undefined, undefined, theme.value, locale.value)
})

watch(() => [embedded.value, route.query.theme, route.query.lang], () => {
  if (!embedded.value) return
  const requestedTheme = route.query.theme
  if (requestedTheme === 'light' || requestedTheme === 'dark') {
    document.documentElement.classList.toggle('dark', requestedTheme === 'dark')
  }
  const requestedLocale = route.query.lang
  if ((requestedLocale === 'zh' || requestedLocale === 'en') && requestedLocale !== locale.value) {
    void setLocale(requestedLocale)
  }
}, { immediate: true })

let themeObserver: MutationObserver | undefined
onMounted(() => {
  if (embedded.value) return
  themeObserver = new MutationObserver(() => { theme.value = detectTheme() })
  themeObserver.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onBeforeUnmount(() => themeObserver?.disconnect())
</script>

<style scoped>
.docs-frame { height: calc(100dvh - 6rem); }
@media (min-width: 768px) {
  .docs-frame { height: calc(100dvh - 7rem); }
}
</style>
