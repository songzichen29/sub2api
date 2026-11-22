<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useTemplateRef, nextTick, useId } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const tooltipId = useId()

const props = withDefaults(defineProps<{
  content?: string
  trigger?: 'hover' | 'click'
  widthClass?: string
}>(), {
  trigger: 'hover',
  widthClass: 'w-64',
})

const show = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipRef = useTemplateRef<HTMLElement>('tooltip')
const tooltipStyle = ref({ top: '0px', left: '0px' })
const above = ref(true)

function openTooltip() {
  show.value = true
  nextTick(updatePosition)
}

function closeTooltip() {
  show.value = false
}

function onEnter() {
  if (props.trigger !== 'hover') return
  openTooltip()
}

function isInside(container: HTMLElement | null, target: EventTarget | null): boolean {
  return target instanceof Node && !!container?.contains(target)
}

// 悬停模式下指针在触发图标与提示框之间往返时保持打开，便于选中提示里的文字。
function onLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(tooltipRef.value, event.relatedTarget)) return
  if (isInside(triggerRef.value, document.activeElement)) return
  closeTooltip()
}

function onTooltipLeave(event: MouseEvent) {
  if (props.trigger !== 'hover') return
  if (isInside(triggerRef.value, event.relatedTarget)) return
  closeTooltip()
}

function onClick(event: Event) {
  event.stopPropagation()
  if (props.trigger === 'hover') {
    openTooltip()
    return
  }
  if (show.value) {
    closeTooltip()
    return
  }
  openTooltip()
}

function onDocumentClick(event: MouseEvent) {
  if (!show.value) return
  const target = event.target as Node | null
  if (!target) return
  if (triggerRef.value?.contains(target) || tooltipRef.value?.contains(target)) return
  closeTooltip()
}

function onDocumentKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !show.value) return
  event.preventDefault()
  event.stopPropagation()
  closeTooltip()
}

function onBlur(event: FocusEvent) {
  if (isInside(tooltipRef.value, event.relatedTarget) || isInside(triggerRef.value, event.relatedTarget)) return
  closeTooltip()
}

function onViewportChange() {
  if (!show.value) return
  updatePosition()
}

function updatePosition() {
  const el = triggerRef.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const width = tooltipRef.value!.offsetWidth
  const height = tooltipRef.value!.offsetHeight
  const center = rect.left + rect.width / 2
  above.value = rect.top >= height + 8
  tooltipStyle.value = {
    top: `${above.value ? rect.top - height - 8 : Math.min(rect.bottom + 8, window.innerHeight - height - 8)}px`,
    left: `${Math.max(width / 2 + 8, Math.min(window.innerWidth - width / 2 - 8, center))}px`,
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick, true)
  document.addEventListener('keydown', onDocumentKeydown, true)
  window.addEventListener('resize', onViewportChange)
  window.addEventListener('scroll', onViewportChange, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClick, true)
  document.removeEventListener('keydown', onDocumentKeydown, true)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<template>
  <div
    ref="trigger"
    class="group relative ml-1 inline-flex items-center rounded align-middle focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-600"
    role="button"
    tabindex="0"
    :aria-label="t('common.info')"
    :aria-expanded="show"
    :aria-controls="tooltipId"
    :aria-describedby="show ? tooltipId : undefined"
    @mouseenter="onEnter"
    @mouseleave="onLeave"
    @click="onClick"
    @keydown.enter.prevent="onClick"
    @keydown.space.prevent="onClick"
    @focus="onEnter"
    @blur="onBlur"
  >
    <!-- Trigger Icon -->
    <slot name="trigger">
      <svg
        class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        stroke-width="2"
      >
        <path
          stroke-linecap="round"
          stroke-linejoin="round"
          d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
        />
      </svg>
    </slot>

    <!-- Teleport to body to escape modal overflow clipping -->
    <Teleport to="body">
      <!-- before: 伪元素向下延伸一段透明区域，盖住提示框与触发图标之间的空隙，让指针能连续移入提示框。 -->
      <div
        ref="tooltip"
        v-show="show"
        role="tooltip"
        :id="tooltipId"
        :class="[
          'fixed z-[99999] max-w-[calc(100vw-16px)] -translate-x-1/2 rounded-md bg-gray-900 p-3 text-sm leading-relaxed text-white shadow-sm ring-1 ring-white/10 selection:bg-primary-200 selection:text-gray-900 before:absolute before:inset-x-0 before:h-3 dark:bg-gray-800 dark:selection:bg-primary-200 dark:selection:text-gray-900',
          above ? 'before:top-full' : 'before:bottom-full',
          props.widthClass,
        ]"
        :style="tooltipStyle"
        @mouseleave="onTooltipLeave"
        @focusout="onBlur"
      >
        <button
          v-if="props.trigger === 'click'"
          type="button"
          class="absolute right-1.5 top-1.5 rounded p-1 text-gray-300 transition-colors hover:bg-white/10 hover:text-white"
          :aria-label="t('common.close')"
          @click.stop="closeTooltip"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <div class="max-h-[calc(100dvh-56px)] overflow-auto break-words"><slot>{{ content }}</slot></div>
        <div class="absolute left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900 dark:bg-gray-800" :class="above ? '-bottom-1' : '-top-1'"></div>
      </div>
    </Teleport>
  </div>
</template>
