<template>
  <div ref="container" class="relative w-full overflow-hidden bg-white" :style="fill ? { height: '100%' } : { aspectRatio: '3 / 2' }">
    <iframe
      v-if="width > 0 && height > 0"
      class="absolute left-0 top-0 origin-top-left border-0"
      :style="frameStyle"
      :srcdoc="documentHTML"
      :title="title"
      sandbox="allow-scripts"
      scrolling="no"
      referrerpolicy="no-referrer"
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { qualityPreviewDocument } from './qualityPreview'

const props = defineProps<{ html: string; title: string; fill?: boolean }>()
const container = ref<HTMLElement>()
const width = ref(0)
const height = ref(0)
const frameStyle = computed(() => {
  const scale = Math.min(width.value / 960, height.value / 640)
  return {
    width: '960px', height: '640px',
    transform: `translate(${(width.value - 960 * scale) / 2}px, ${(height.value - 640 * scale) / 2}px) scale(${scale})`,
  }
})
const nonce = document.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''
const documentHTML = computed(() => qualityPreviewDocument(props.html, nonce))
let observer: ResizeObserver | undefined
onMounted(() => {
  if (!container.value) return
  width.value = container.value.clientWidth
  height.value = container.value.clientHeight
  observer = new ResizeObserver(entries => {
    width.value = entries[0]?.contentRect.width || 0
    height.value = entries[0]?.contentRect.height || 0
  })
  observer.observe(container.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>
