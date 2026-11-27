<template>
  <div class="lottery-wheel" :class="{ 'is-spinning': spinning, 'is-rolling': rolling, 'is-winner': won }">
    <svg class="wheel-art" viewBox="0 0 600 600" role="img" :aria-label="t('lotteryPortal.wheelLabel')">
      <circle class="wheel-rim" cx="300" cy="300" r="280" />
      <circle class="wheel-border" cx="300" cy="300" r="269" />
      <circle v-if="spinning" class="wheel-orbit" cx="300" cy="300" r="280" />
      <g class="wheel-rotor" :style="{ transform: `rotate(${rotation}deg)` }" @transitionend="$emit('settled')">
        <g v-for="(prize, index) in prizes" :key="index">
          <circle v-if="prizes.length === 1" class="wheel-sector" cx="300" cy="300" r="263" :class="sectorClass(prize, index)" />
          <path v-else class="wheel-sector" :d="sector(index)" :class="sectorClass(prize, index)" />
          <g :transform="`translate(300 300) rotate(${index * step})`" text-anchor="middle">
            <text
              class="wheel-prize-label"
              :class="{ 'is-featured': prize.amount > 0 && index === featuredIndex }"
              :transform="labelTransform(index)"
              :aria-label="prize.name"
              :font-size="fontSize"
              font-weight="600"
            ><tspan
              v-for="(line, lineIndex) in nameLines(prize.name)"
              :key="lineIndex"
              x="0"
              :y="-186 + (lineIndex - (nameLines(prize.name).length - 1) / 2) * (fontSize + 5)"
              dominant-baseline="central"
              :textLength="nameWidth(line)"
              lengthAdjust="spacingAndGlyphs"
            >{{ line }}</tspan></text>
          </g>
        </g>
        <circle v-if="!prizes.length" class="wheel-empty" cx="300" cy="300" r="263" />
      </g>
      <circle class="wheel-hub" cx="300" cy="300" r="86" />
      <path class="wheel-pointer" d="M284 12 H316 Q322 12 320 20 L303 66 Q300 73 297 66 L280 20 Q278 12 284 12 Z" />
      <g v-if="won" class="wheel-confetti" transform="translate(300 300)" aria-hidden="true">
        <g v-for="n in 12" :key="n" :transform="`rotate(${n * 30})`">
          <path d="M0 -265 v-13" :style="{ animationDelay: `${(n % 3) * 60}ms` }" />
        </g>
      </g>
    </svg>
    <button type="button" class="wheel-trigger" :disabled="disabled" :aria-label="actionLabel || t(spinning ? 'lotteryPortal.spinning' : 'lotteryPortal.spin')" @click="$emit('spin')">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
        <path d="M12 4v16M4 12h16M6.5 6.5l11 11M6.5 17.5l11-11" stroke-linecap="round" />
        <circle cx="12" cy="12" r="4" fill="currentColor" stroke="none" />
      </svg>
      <span>{{ t(spinning ? 'lotteryPortal.spinning' : 'lotteryPortal.wheelCenter') }}</span>
    </button>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQLotteryPrize } from '@/api/qqcommunity'
const props = defineProps<{ prizes: QQLotteryPrize[]; rotation: number; disabled?: boolean; spinning?: boolean; rolling?: boolean; won?: boolean; actionLabel?: string }>()
defineEmits<{ settled: []; spin: [] }>()
const { t } = useI18n()
const step = computed(() => 360 / Math.max(props.prizes.length, 1))
const fontSize = computed(() => props.prizes.length > 12 ? 22 : props.prizes.length > 8 ? 26 : 32)
const featuredIndex = computed(() => {
  const maxAmount = Math.max(0, ...props.prizes.map(prize => prize.amount))
  return props.prizes.findIndex(prize => prize.amount > 0 && prize.amount === maxAmount)
})
function point(angle: number) {
  const radians = angle * Math.PI / 180
  return `${300 + 263 * Math.cos(radians)} ${300 + 263 * Math.sin(radians)}`
}
function sector(index: number) {
  const start = -90 - step.value / 2 + index * step.value
  return `M300 300 L${point(start)} A263 263 0 ${step.value > 180 ? 1 : 0} 1 ${point(start + step.value)} Z`
}
function sectorClass(prize: QQLotteryPrize, index: number) {
  if (!prize.amount) return 'is-empty'
  if (index === featuredIndex.value) return 'is-featured'
  return index % 2 ? 'is-light' : 'is-mint'
}
function labelTransform(index: number) {
  const angle = (index * step.value + props.rotation) % 360
  return angle > 90 && angle < 270 ? 'rotate(180 0 -186)' : undefined
}
function nameWidth(line: string) {
  const available = Math.max(32, 2 * 168 * Math.sin(Math.PI / Math.max(props.prizes.length, 2)) - 16)
  const natural = Array.from(line).reduce((width, char) => width + (char.charCodeAt(0) > 255 ? 1 : .6), 0) * fontSize.value
  return natural > available ? available : undefined
}
function nameLines(name: string) {
  const limit = props.prizes.length > 12 ? 3 : props.prizes.length > 8 ? 4 : 6
  const chars = Array.from(name)
  if (chars.length <= limit) return [name]
  const split = Math.min(limit, Math.ceil(chars.length / 2))
  return [chars.slice(0, split).join(''), chars.slice(split, split + limit).join('') + (chars.length > split + limit ? '…' : '')]
}
</script>
<style scoped>
.lottery-wheel { position: relative; width: min(100%, 500px); margin-inline: auto; }
.wheel-art { display: block; width: 100%; height: auto; overflow: hidden; font-family: inherit; }
.wheel-rim { fill: #e8eeeb; stroke: #d7e1dc; stroke-width: 1; }
.wheel-border { fill: #fff; }
.wheel-sector { stroke: #fff; stroke-width: 3; }
.wheel-sector.is-empty, .wheel-empty { fill: #f1f3f0; }
.wheel-sector.is-featured { fill: #165b4b; }
.wheel-sector.is-light { fill: #f5eddc; }
.wheel-sector.is-mint { fill: #d5e8df; }
.wheel-prize-label { fill: #234e41; }
.wheel-prize-label.is-featured { fill: #fff; }
.wheel-rotor { transform-origin: 300px 300px; transition: transform 3.6s cubic-bezier(.12,.72,.12,1); }
.wheel-hub { fill: #fff; stroke: #d7e1dc; stroke-width: 1; }
.wheel-pointer { fill: #165b4b; stroke: #fff; stroke-width: 3; }
.wheel-orbit { fill: none; stroke: #0f766e; stroke-width: 4; stroke-dasharray: 160 1600; transform-origin: 300px 300px; animation: wheel-orbit 1.2s linear infinite; }
.wheel-confetti path { fill: none; stroke: #0f766e; stroke-width: 5; stroke-linecap: round; animation: wheel-burst 800ms ease-out both; }
.wheel-confetti g:nth-child(even) path { stroke: #c29a4e; }
.wheel-trigger { position: absolute; inset: 37%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; border: 0; border-radius: 50%; background: #165b4b; color: #fff; font-family: inherit; font-size: clamp(14px, 1.3vw, 17px); font-weight: 600; cursor: pointer; transition: background-color 150ms; }
.wheel-trigger svg { width: 28%; height: 28%; }
.wheel-trigger:hover:enabled { background: #0f766e; }
.wheel-trigger:active:enabled { background: #134e4a; }
.wheel-trigger:focus-visible { outline: 3px solid #0f766e; outline-offset: 4px; }
.wheel-trigger:disabled { cursor: default; background: #456d60; }
.is-spinning .wheel-trigger { background: #165b4b; }
.is-spinning .wheel-trigger svg { animation: wheel-orbit 1.6s linear infinite; }
.is-rolling .wheel-pointer { transform-origin: 300px 12px; animation: pointer-tick 150ms ease-in-out infinite alternate; }
.is-winner .wheel-trigger { animation: winner-pulse 500ms ease-out; }
.is-winner .wheel-rim { stroke: #0f766e; stroke-width: 3; }
.dark .wheel-rim { fill: #273c35; stroke: #40584e; }
.dark .wheel-border, .dark .wheel-hub { fill: #142d25; }
.dark .wheel-sector { stroke: #142d25; }
.dark .wheel-hub { stroke: #6e8e7e; }
.dark .wheel-pointer { fill: #d5e8df; stroke: #142d25; }
.dark .wheel-orbit, .dark .wheel-confetti path { stroke: #8bd3b6; }
@keyframes wheel-orbit { to { transform: rotate(360deg); } }
@keyframes pointer-tick { to { transform: rotate(-8deg); } }
@keyframes winner-pulse { 45% { transform: scale(1.06); } }
@keyframes wheel-burst { from { opacity: 0; transform: translateY(10px); } 20% { opacity: 1; } to { opacity: 0; transform: translateY(-18px); } }
@media (prefers-reduced-motion: reduce) {
  .wheel-rotor { transition-duration: 0.01s; }
  .wheel-orbit, .wheel-confetti, .is-spinning .wheel-trigger svg, .is-rolling .wheel-pointer, .is-winner .wheel-trigger { animation: none; }
  .wheel-confetti { display: none; }
}
</style>
