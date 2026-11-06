<template>
  <div class="lottery-wheel" :class="{ 'is-spinning': spinning }">
    <svg class="wheel-art" viewBox="0 0 600 600" role="img" :aria-label="t('lotteryPortal.wheelLabel')">
      <circle cx="300" cy="305" r="278" fill="#c8b691" opacity=".3" />
      <circle cx="300" cy="300" r="278" fill="#194f43" stroke="#e9d2a0" stroke-width="5" />
      <circle cx="300" cy="300" r="256" fill="#f6ecd8" stroke="#efd9aa" stroke-width="4" />
      <g class="wheel-lights" aria-hidden="true">
        <circle v-for="n in 48" :key="n" :cx="rimPoint(n).x" :cy="rimPoint(n).y" r="2.4" fill="#f5dfad" />
      </g>
      <g class="wheel-rotor" :style="{ transform: `rotate(${rotation}deg)` }" @transitionend="$emit('settled')">
        <g v-for="(prize, index) in prizes" :key="index">
          <circle v-if="prizes.length === 1" cx="300" cy="300" r="251" :fill="sectorColor(prize, index)" />
          <path v-else :d="sector(index)" :fill="sectorColor(prize, index)" stroke="#fcf7eb" stroke-width="2" />
          <g :transform="`translate(300 300) rotate(${index * step})`" fill="#244e40" text-anchor="middle">
            <template v-if="prize.amount > 0">
              <text :y="dense ? -186 : -169" :font-size="dense ? 22 : 42" :textLength="amountWidth(prize.amount)" lengthAdjust="spacingAndGlyphs" font-weight="600" class="wheel-amount">{{ money(prize.amount) }}</text>
              <text :y="dense ? -162 : -138" :font-size="dense ? 13 : 17">{{ shortName(prize.name) }}</text>
            </template>
            <text v-else :y="dense ? -181 : -159" :font-size="dense ? 15 : 24" font-weight="500">{{ shortName(prize.name) }}</text>
          </g>
        </g>
        <circle v-if="!prizes.length" cx="300" cy="300" r="251" fill="#e6e8df" />
      </g>
      <circle cx="300" cy="300" r="76" fill="#f5e7c9" stroke="#fff9ec" stroke-width="5" />
      <circle cx="300" cy="300" r="67" fill="none" stroke="#c4a66b" stroke-width="1" />
      <path d="M282 23 Q282 5 300 5 Q318 5 318 23 Q315 41 300 64 Q285 41 282 23 Z" fill="#edd4a2" stroke="#fff8e8" stroke-width="3" />
      <circle cx="300" cy="22" r="5.5" fill="#194f43" />
    </svg>
    <button type="button" class="wheel-trigger" :disabled="disabled" :aria-label="actionLabel || t(spinning ? 'lotteryPortal.spinning' : 'lotteryPortal.spin')" @click="$emit('spin')">
      <svg viewBox="0 0 32 32" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
        <path d="M5 14h22v-5H5zM7 14v14h18V14M16 9v19M16 9C9 9 7 6 9 3c3-3 7 3 7 6Zm0 0c7 0 9-3 7-6-3-3-7 3-7 6Z" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
      <span>{{ t(spinning ? 'lotteryPortal.spinning' : 'lotteryPortal.wheelCenter') }}</span>
    </button>
  </div>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { QQLotteryPrize } from '@/api/qqcommunity'
const props = defineProps<{ prizes: QQLotteryPrize[]; rotation: number; disabled?: boolean; spinning?: boolean; actionLabel?: string }>()
defineEmits<{ settled: []; spin: [] }>()
const { t } = useI18n()
const step = computed(() => 360 / Math.max(props.prizes.length, 1))
const dense = computed(() => props.prizes.length > 8)
const maxAmount = computed(() => Math.max(0, ...props.prizes.map(prize => prize.amount)))
function rimPoint(n: number) {
  const angle = n * Math.PI / 24
  return { x: 300 + 267 * Math.cos(angle), y: 300 + 267 * Math.sin(angle) }
}
function point(angle: number) {
  const radians = angle * Math.PI / 180
  return `${300 + 251 * Math.cos(radians)} ${300 + 251 * Math.sin(radians)}`
}
function sector(index: number) {
  const start = -90 - step.value / 2 + index * step.value
  return `M300 300 L${point(start)} A251 251 0 ${step.value > 180 ? 1 : 0} 1 ${point(start + step.value)} Z`
}
function sectorColor(prize: QQLotteryPrize, index: number) {
  if (!prize.amount) return '#b5cec0'
  if (prize.amount === maxAmount.value) return '#f4dfad'
  return index % 2 ? '#faf3e2' : '#d8e8dd'
}
function money(amount: number) {
  return '$' + new Intl.NumberFormat('en-US', { maximumFractionDigits: 8 }).format(amount)
}
function amountWidth(amount: number) {
  const available = Math.max(32, 2 * 165 * Math.sin(Math.PI / Math.max(props.prizes.length, 2)) - 12)
  const natural = money(amount).length * (dense.value ? 13 : 24)
  return natural > available ? available : undefined
}
function shortName(name: string) {
  const limit = props.prizes.length > 12 ? 3 : props.prizes.length > 6 ? 5 : 8
  const chars = Array.from(name)
  return chars.length > limit ? chars.slice(0, limit).join('') + '…' : name
}
</script>
<style scoped>
.lottery-wheel { position: relative; width: min(100%, 540px); margin-inline: auto; }
.wheel-art { display: block; width: 100%; height: auto; overflow: hidden; font-family: inherit; }
.wheel-rotor { transform-origin: 300px 300px; transition: transform 3.6s cubic-bezier(.12,.72,.12,1); }
.wheel-amount { font-variant-numeric: tabular-nums; letter-spacing: -1px; }
.wheel-trigger { position: absolute; inset: 38.5%; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 6px; border: 0; border-radius: 50%; background: transparent; color: #345243; font-family: inherit; font-size: clamp(12px, 1.15vw, 15px); font-weight: 600; cursor: pointer; transition: background-color 150ms; }
.wheel-trigger svg { width: 30%; height: 30%; }
.wheel-trigger:hover:enabled { background: #fff9e76b; }
.wheel-trigger:focus-visible { outline: 3px solid #0f766e; outline-offset: 4px; }
.wheel-trigger:disabled { cursor: default; }
.is-spinning .wheel-lights { animation: wheel-lights 700ms ease-in-out infinite alternate; }
@keyframes wheel-lights { to { opacity: .4; } }
@media (prefers-reduced-motion: reduce) { .wheel-rotor { transition-duration: 0.01s; } .is-spinning .wheel-lights { animation: none; } }
</style>
