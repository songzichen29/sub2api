import { onUnmounted, ref, watch, type Ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { AccountPerformanceStatus } from '@/types'

export function useAccountMetricRecoveryList(accounts: Ref<{ id: number }[]>) {
  const statuses = ref<Record<string, AccountPerformanceStatus>>({})
  const loading = ref(false)
  const error = ref(false)
  let controller: AbortController | undefined
  let sequence = 0
  async function refresh() {
    controller?.abort()
    const current = ++sequence
    const request = new AbortController()
    controller = request
    const ids = [...new Set(accounts.value.map(account => account.id))]
    statuses.value = {}
    error.value = false
    loading.value = ids.length > 0
    try {
      const next: Record<string, AccountPerformanceStatus> = {}
      // Bound backend work even when a saved page size exceeds 100.
      for (let offset = 0; offset < ids.length; offset += 100) {
        const result = await adminAPI.accounts.getBatchMetricRecovery(ids.slice(offset, offset + 100), request.signal)
        if (current !== sequence) return
        Object.assign(next, result.accounts)
      }
      if (current === sequence) statuses.value = next
    } catch {
      if (current === sequence && !request.signal.aborted) error.value = true
    } finally {
      if (current === sequence) loading.value = false
    }
  }
  watch(accounts, () => { void refresh() })
  onUnmounted(() => { ++sequence; controller?.abort() })
  return { statuses, loading, error, refresh }
}
