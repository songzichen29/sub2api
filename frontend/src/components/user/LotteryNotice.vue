<template>
  <section v-if="data?.enabled || error" class="card space-y-2 p-4 text-sm" aria-live="polite">
    <template v-if="data?.enabled">
      <p class="font-medium">{{ t('lotteryPortal.remainingText', { count: data.state.remaining }) }}</p>
      <p v-if="data.state.recharge_remaining !== undefined && data.state.bonus_remaining !== undefined">{{ t('lotteryPortal.remainingSources', { recharge: data.state.recharge_remaining, activity: data.state.bonus_remaining }) }}</p>
      <p v-if="data.pending_recharge">{{ t(attempts >= 12 ? 'lotteryPortal.pendingLater' : 'lotteryPortal.pending') }}</p>
      <p v-else>{{ t('lotteryPortal.next', { amount: data.state.next_recharge.toFixed(2), count: data.chances_per_step }) }}</p>
      <p v-if="!data.bound_qq">{{ t('lotteryPortal.unbound', { count: data.state.remaining }) }}</p>
    </template>
    <p v-if="error" role="alert">{{ t('lotteryPortal.loadFailed') }}</p>
    <div class="flex flex-wrap items-center gap-4">
      <RouterLink to="/lottery" class="text-primary-600 hover:underline dark:text-primary-400">{{ t('lotteryPortal.view') }}</RouterLink>
      <button type="button" class="text-gray-600 hover:underline dark:text-gray-300" :disabled="busy" @click="refresh">{{ t('lotteryPortal.refresh') }}</button>
    </div>
  </section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { lotteryAPI, type UserLottery } from '@/api/lottery'
const props = defineProps<{ refreshKey?: number | string }>()
const { t } = useI18n()
const data = ref<UserLottery | null>(null)
const error = ref(false)
const busy = ref(false)
const attempts = ref(0)
let timer: ReturnType<typeof setTimeout> | undefined
let generation = 0
async function load(id: number) {
  busy.value = true
  try {
    const value = await lotteryAPI.overview()
    if (id !== generation) return
    data.value = value; error.value = false
    if (value.enabled && value.pending_recharge && attempts.value < 12) {
      timer = setTimeout(() => { attempts.value++; void load(id) }, 5000)
    }
  } catch { if (id === generation) error.value = true }
  finally { if (id === generation) busy.value = false }
}
function refresh() { clearTimeout(timer); attempts.value = 0; void load(++generation) }
watch(() => props.refreshKey, refresh, { immediate: true })
onBeforeUnmount(() => { generation++; clearTimeout(timer) })
</script>
