<template>
  <div ref="pageRoot" class="integration-docs min-h-screen bg-white text-gray-900 dark:bg-dark-950 dark:text-dark-100" :class="{ 'docs-embedded': embedded }">
    <header v-if="!embedded" class="sticky top-0 z-30 border-b border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-950">
      <div class="flex h-16 items-center justify-between gap-3 px-4 sm:px-6">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3" :aria-label="t('docs.home')">
          <img :src="siteLogo || '/logo.svg'" :alt="siteName" class="h-8 w-8 shrink-0 object-contain" />
          <span class="truncate text-base font-semibold">{{ siteName }}</span>
          <span class="hidden shrink-0 border-l border-gray-200 pl-3 text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400 sm:block">{{ t('docs.title') }}</span>
        </RouterLink>
        <nav class="flex shrink-0 items-center gap-1 sm:gap-3" :aria-label="t('docs.mainNav')">
          <RouterLink to="/home" class="doc-header-link hidden sm:flex">{{ t('docs.home') }}</RouterLink>
          <LocaleSwitcher />
          <button type="button" class="doc-icon-button" :aria-label="isDark ? t('nav.lightMode') : t('nav.darkMode')" @click="toggleTheme">
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <RouterLink :to="authStore.isAuthenticated ? '/keys' : '/login?redirect=/keys'" class="rounded-md bg-primary-600 px-3 py-2 text-sm font-medium text-white hover:bg-primary-700">
            {{ authStore.isAuthenticated ? t('docs.keys') : t('docs.signIn') }}
          </RouterLink>
        </nav>
      </div>
    </header>

    <div class="docs-layout">
      <aside class="docs-sidebar hidden lg:block">
        <label for="docs-search" class="mb-2 block text-sm font-medium">{{ t('docs.search') }}</label>
        <div class="doc-search">
          <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
          <input id="docs-search" ref="desktopSearch" v-model="searchQuery" type="search" :placeholder="t('docs.searchHint')" :aria-label="t('docs.searchLabel')" @keydown.esc="searchQuery = ''" />
        </div>
        <p class="mb-3 mt-8 text-sm font-semibold text-gray-900 dark:text-dark-100">{{ t('docs.menu') }}</p>
        <nav :aria-label="t('docs.docsNav')" class="space-y-1">
          <a v-for="section in filteredSections" :key="section.id" :href="'#' + section.id" class="doc-side-link" :class="{ 'is-active': activeSection === section.id }" :aria-current="activeSection === section.id ? 'location' : undefined" @click.prevent="goToSection(section.id)">
            {{ section.title }}
          </a>
          <p v-if="!filteredSections.length" class="text-sm leading-6 text-gray-500">{{ t('docs.noResults') }}</p>
        </nav>
        <a v-if="externalDocs" :href="externalDocs" target="_blank" rel="noopener noreferrer" class="mt-6 flex items-center gap-2 border-t border-gray-200 pt-5 text-sm text-gray-600 hover:text-primary-700 dark:border-dark-800 dark:text-dark-300">
          <Icon name="externalLink" size="sm" />{{ t('nav.externalDocs') }}
        </a>
      </aside>

      <main class="min-w-0 px-4 py-6 sm:px-8 lg:px-10 lg:py-8">
        <div class="mb-7 border-b border-gray-200 pb-6 dark:border-dark-800">
          <h1 class="text-2xl font-semibold tracking-tight sm:text-3xl">{{ t('docs.title') }}</h1>
          <p class="mt-2 text-sm leading-7 text-gray-600 dark:text-dark-300">{{ t('docs.description') }}</p>
        </div>
        <div class="mb-6 space-y-3 lg:hidden">
          <div class="doc-search">
            <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
            <input ref="mobileSearch" v-model="searchQuery" type="search" :placeholder="t('docs.searchHint')" :aria-label="t('docs.searchLabel')" @keydown.esc="searchQuery = ''" />
          </div>
          <details ref="mobileMenu" class="border-b border-gray-200 pb-3 dark:border-dark-800">
            <summary class="cursor-pointer text-sm font-medium">{{ t('docs.menu') }}</summary>
            <nav :aria-label="t('docs.docsNav')" class="mt-3 grid gap-1 sm:grid-cols-2">
              <a v-for="section in filteredSections" :key="section.id" :href="'#' + section.id" class="doc-side-link" @click.prevent="goToSection(section.id)">{{ section.title }}</a>
            </nav>
          </details>
        </div>

        <div v-if="!filteredSections.length" role="status" class="py-14 text-sm text-gray-500 dark:text-dark-400">
          {{ t('docs.noResults') }}
          <button type="button" class="ml-3 text-primary-600 underline dark:text-primary-400" @click="searchQuery = ''">{{ t('common.clear') }}</button>
        </div>

        <section
          v-for="section in filteredSections"
          :id="section.id"
          :key="section.id"
          data-doc-section
          tabindex="-1"
          class="doc-section"
          :aria-labelledby="section.id + '-title'"
        >
          <div class="doc-section-layout" :class="{ 'has-examples': section.examples?.length }">
            <div class="min-w-0">
              <h2 :id="section.id + '-title'" class="mb-3 text-xl font-semibold">{{ section.title }}</h2>
              <p class="doc-copy">{{ section.body }}</p>
              <ol v-if="section.steps" class="mt-5 list-decimal space-y-3 pl-5 text-sm leading-7 text-gray-700 marker:text-gray-400 dark:text-dark-200">
                <li v-for="step in section.steps" :key="step" class="pl-1">{{ step }}</li>
              </ol>
              <p v-if="section.note" class="mt-5 border-l-2 border-primary-500 pl-4 text-sm leading-7 text-gray-600 dark:text-dark-300">{{ section.note }}</p>
              <div v-if="section.links" class="mt-5 flex flex-wrap gap-x-6 gap-y-3">
                <template v-for="link in section.links" :key="link.to">
                  <a v-if="link.to.startsWith('#')" :href="link.to" class="doc-text-link" @click.prevent="goToSection(link.to.slice(1))">{{ link.label }}<Icon name="arrowRight" size="sm" /></a>
                  <a v-else-if="embedded" :href="link.to" target="_top" class="doc-text-link">{{ link.label }}<Icon name="arrowRight" size="sm" /></a>
                  <RouterLink v-else :to="link.to" class="doc-text-link">{{ link.label }}<Icon name="arrowRight" size="sm" /></RouterLink>
                </template>
              </div>
            </div>

            <div v-if="section.examples?.length" class="min-w-0">
              <div v-if="section.examples.length > 1" class="mb-3 flex flex-wrap gap-x-5 gap-y-2 border-b border-gray-200 dark:border-dark-800">
                <button v-for="(example, index) in section.examples" :key="example.label" type="button" class="doc-example-button" :class="{ 'is-active': exampleIndex(section) === index }" :aria-pressed="exampleIndex(section) === index" @click="selectedExamples[section.id] = index">{{ example.label }}</button>
              </div>
              <DocsCodeBlock :key="section.id + exampleIndex(section)" :label="currentExample(section)!.label" :code="currentExample(section)!.code" />
              <p v-if="currentExample(section)?.hint" class="mt-3 text-xs leading-6 text-gray-500 dark:text-dark-400">{{ currentExample(section)!.hint }}</p>
              <dl v-if="section.id === 'overview'" class="mt-6 space-y-3 border-t border-gray-200 pt-4 dark:border-dark-800">
                <div v-for="endpoint in endpointLabels" :key="endpoint.label">
                  <dt class="text-xs text-gray-500 dark:text-dark-400">{{ endpoint.label }}</dt>
                  <dd class="mt-1 flex items-center gap-2">
                    <code class="min-w-0 break-all text-sm">{{ endpoint.url }}</code>
                    <button type="button" class="doc-icon-button shrink-0" :aria-label="t('docs.copyLabel', { label: endpoint.label })" @click="copyToClipboard(endpoint.url)"><Icon name="copy" size="sm" /></button>
                  </dd>
                </div>
              </dl>
            </div>
          </div>

          <div v-if="section.rows" class="mt-5 overflow-x-auto rounded-lg border border-gray-200 dark:border-dark-800" tabindex="0" :aria-label="section.title">
            <table class="w-full text-left text-sm">
              <thead class="border-b border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-800 dark:bg-dark-900 dark:text-dark-400">
                <tr><th class="px-4 py-3 font-medium">{{ t('docs.method') }}</th><th class="px-4 py-3 font-medium">{{ t('docs.path') }}</th><th class="px-4 py-3 font-medium">{{ t('docs.purpose') }}</th></tr>
              </thead>
              <tbody class="divide-y divide-gray-200 dark:divide-dark-800">
                <tr v-for="row in section.rows" :key="row.path">
                  <td class="px-4 py-3 font-mono text-xs font-semibold text-primary-700 dark:text-primary-300">{{ row.method }}</td>
                  <td class="whitespace-nowrap px-4 py-3 font-mono text-xs">{{ row.path }}</td>
                  <td class="px-4 py-3 text-gray-600 dark:text-dark-300">{{ row.purpose }}</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="section.questions" class="mt-5 divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-800 dark:border-dark-800">
            <details v-for="question in section.questions" :key="question.title" class="py-4" :open="!!searchQuery.trim()">
              <summary class="cursor-pointer text-sm font-medium">{{ question.title }}</summary>
              <p class="mt-3 text-sm leading-7 text-gray-600 dark:text-dark-300">{{ question.body }}</p>
            </details>
          </div>
        </section>
        <footer class="pt-5 text-xs leading-6 text-gray-500 dark:text-dark-400">{{ t('docs.footer') }}</footer>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import DocsCodeBlock from '@/components/docs/DocsCodeBlock.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'
import { createIntegrationDocs, filterDocsSections, resolveDocsEndpoints, type DocsSection } from './integrationDocs'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()
const pageRoot = ref<HTMLElement>()
const desktopSearch = ref<HTMLInputElement>()
const mobileSearch = ref<HTMLInputElement>()
const mobileMenu = ref<HTMLDetailsElement>()
const searchQuery = ref('')
const activeSection = ref('overview')
const selectedExamples = ref<Record<string, number>>({})
const isDark = ref(document.documentElement.classList.contains('dark'))

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const externalDocs = computed(() => {
  const url = sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
  return url && new URL(url).pathname !== '/docs' ? url : ''
})
const endpoints = computed(() => resolveDocsEndpoints(appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || '', window.location.origin))
const endpointLabels = computed(() => [
  { label: t('docs.currentEndpoint'), url: endpoints.value.openai },
  { label: t('docs.rootEndpoint'), url: endpoints.value.root },
])
const sections = computed(() => createIntegrationDocs(t, endpoints.value))
const filteredSections = computed(() => filterDocsSections(sections.value, searchQuery.value))

function exampleIndex(section: DocsSection): number {
  return selectedExamples.value[section.id] || 0
}

function currentExample(section: DocsSection) {
  return section.examples?.[exampleIndex(section)]
}

async function goToSection(id: string) {
  searchQuery.value = ''
  if (mobileMenu.value) mobileMenu.value.open = false
  await nextTick()
  await router.push({ path: '/docs', query: route.query, hash: '#' + id })
  activeSection.value = id
  const element = document.getElementById(id)
  element?.scrollIntoView({ block: 'start' })
  element?.focus({ preventScroll: true })
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function onKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    const input = window.innerWidth >= 1024 ? desktopSearch.value : mobileSearch.value
    input?.focus()
  }
}

let scrollFrame = 0
function onScroll() {
  if (scrollFrame) return
  scrollFrame = window.requestAnimationFrame(() => {
    const elements = pageRoot.value?.querySelectorAll<HTMLElement>('[data-doc-section]')
    let current = elements?.[0]?.id
    elements?.forEach(element => {
      if (element.getBoundingClientRect().top <= (props.embedded ? 24 : 110)) current = element.id
    })
    if (current) activeSection.value = current
    scrollFrame = 0
  })
}

onMounted(async () => {
  void appStore.fetchPublicSettings()
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('scroll', onScroll, { passive: true })
  const hash = router.currentRoute.value.hash.slice(1)
  if (sections.value.some(section => section.id === hash)) {
    activeSection.value = hash
    // Initial navigation completes before the app mounts, so its scroll hook
    // cannot yet find these sections. Resolve direct chapter links after render.
    await nextTick()
    const element = document.getElementById(hash)
    element?.scrollIntoView({ block: 'start' })
    element?.focus({ preventScroll: true })
  }
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', onScroll)
  window.cancelAnimationFrame(scrollFrame)
})
</script>

<style scoped>
.docs-layout { display: grid; grid-template-columns: minmax(0, 1fr); }
.docs-sidebar {
  @apply sticky top-16 h-[calc(100vh-4rem)] overflow-y-auto border-r border-gray-200 bg-gray-50 px-5 py-7 dark:border-dark-800 dark:bg-dark-900;
}
.doc-search {
  @apply flex items-center gap-2 rounded-md border border-gray-300 bg-white px-3 py-2.5 focus-within:border-primary-500 focus-within:ring-1 focus-within:ring-primary-500 dark:border-dark-700 dark:bg-dark-950;
}
.doc-search input { @apply min-w-0 flex-1 border-0 bg-transparent p-0 text-sm outline-none placeholder:text-gray-400 focus:ring-0 dark:placeholder:text-dark-500; }
.doc-header-link { @apply items-center gap-2 px-2 py-2 text-sm text-gray-600 hover:text-primary-700 dark:text-dark-300 dark:hover:text-primary-300; }
.doc-icon-button { @apply inline-flex items-center justify-center rounded-md p-2 text-gray-500 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white; }
.doc-side-link { @apply block border-l-2 border-transparent px-3 py-2 text-sm text-gray-600 hover:bg-gray-100 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white; }
.doc-side-link.is-active { @apply border-primary-600 bg-primary-50 font-medium text-primary-700 dark:border-primary-400 dark:bg-primary-500/10 dark:text-primary-300; }
.doc-section { @apply scroll-mt-24 border-b border-gray-200 py-8 outline-none dark:border-dark-800; }
.doc-section:first-of-type { @apply pt-0; }
.doc-section:focus-visible { @apply outline outline-2 outline-offset-4 outline-primary-500; }
.docs-embedded .docs-sidebar { top: 0; height: 100dvh; }
.docs-embedded .doc-section { scroll-margin-top: 16px; }
.doc-section-layout { display: grid; gap: 24px; }
.doc-copy { @apply text-sm leading-7 text-gray-700 dark:text-dark-200; }
.doc-text-link { @apply inline-flex items-center gap-2 text-sm font-medium text-primary-700 hover:underline dark:text-primary-300; }
.doc-example-button { @apply border-b-2 border-transparent pb-2 text-xs text-gray-500 hover:text-gray-900 dark:text-dark-400 dark:hover:text-white; }
.doc-example-button.is-active { @apply border-primary-600 font-medium text-primary-700 dark:border-primary-400 dark:text-primary-300; }
@media (min-width: 1024px) { .docs-layout { grid-template-columns: 248px minmax(0, 1fr); } }
@media (min-width: 1280px) {
  .doc-section-layout.has-examples { grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.1fr); gap: 32px; }
}
</style>
