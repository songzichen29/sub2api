import { onBeforeUnmount, onMounted, ref, toValue, type MaybeRefOrGetter } from 'vue'
import { useI18n } from 'vue-i18n'
import * as api from '@/api/modelQuality'
import { extractApiErrorMessage } from '@/utils/apiError'

export function useModelQuality(admin: MaybeRefOrGetter<boolean> = false) {
  const { t } = useI18n()
  const plans = ref<api.QualityPlan[]>([])
  const loading = ref(false)
  const error = ref('')
  let disposed = false
  let timer: ReturnType<typeof setTimeout> | undefined

  async function refresh() {
    if (loading.value || disposed) return
    loading.value = true
    try {
      const rows = await api.list(toValue(admin))
      if (!disposed) { plans.value = rows; error.value = '' }
    } catch (e) {
      if (!disposed) error.value = extractApiErrorMessage(e, t('modelQuality.loadFailed'))
    } finally { loading.value = false }
  }
  async function poll() {
    if (!document.hidden) await refresh()
    if (!disposed) timer = setTimeout(poll, plans.value.some(p => p.recent.some(r => !r.finished_at)) ? 5000 : 30000)
  }
  onMounted(poll)
  onBeforeUnmount(() => { disposed = true; if (timer) clearTimeout(timer) })
  return { plans, loading, error, refresh }
}
