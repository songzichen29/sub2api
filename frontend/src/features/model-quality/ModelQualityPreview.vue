<template>
  <div ref="container" class="relative w-full overflow-hidden bg-white" :style="fill ? { height: '100%' } : { aspectRatio: '3 / 2' }">
    <iframe
      v-if="width > 0 && height > 0"
      ref="frame"
      class="absolute left-0 top-0 origin-top-left border-0"
      :style="frameStyle"
      :srcdoc="documentHTML"
      :title="title"
      sandbox="allow-scripts"
      scrolling="no"
      referrerpolicy="no-referrer"
      allow="camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'"
    />
    <p v-if="scriptFailed" role="alert" class="absolute inset-x-0 bottom-0 border-t border-red-200 bg-white/95 px-3 py-2 text-xs leading-5 text-red-700">{{ t('modelQuality.previewScriptFailed') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { qualityPreviewDocument } from './qualityPreview'

const props = defineProps<{ html: string; title: string; fill?: boolean }>()
const { t } = useI18n()
const container = ref<HTMLElement>()
const frame = ref<HTMLIFrameElement>()
const scriptFailed = ref(false)
const documentVersion = ref(0)
watch(() => props.html, () => {
  scriptFailed.value = false
  documentVersion.value++
}, { immediate: true, flush: 'sync' })
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
const documentHTML = computed(() => qualityPreviewDocument(props.html, nonce, documentVersion.value))
function handleMessage(event: MessageEvent) {
  // iframe 使用不透明来源；同时核对窗口与文档版本，排除其他作品和旧文档的迟到消息。
  if (!frame.value || event.source !== frame.value.contentWindow) return
  if (event.data?.type !== 'sub2api-quality-preview-error' || event.data?.documentVersion !== documentVersion.value) return
  scriptFailed.value = true
}
let observer: ResizeObserver | undefined
onMounted(() => {
  window.addEventListener('message', handleMessage)
  if (!container.value) return
  width.value = container.value.clientWidth
  height.value = container.value.clientHeight
  observer = new ResizeObserver(entries => {
    width.value = entries[0]?.contentRect.width || 0
    height.value = entries[0]?.contentRect.height || 0
  })
  observer.observe(container.value)
})
onBeforeUnmount(() => {
  window.removeEventListener('message', handleMessage)
  observer?.disconnect()
})
</script>
