import type { QQChatRuntimeStatus } from '@/api/qqcommunity'

export function runtimeMetricsFresh(status?: QQChatRuntimeStatus, now = Date.now()): boolean {
  const seen = Date.parse(status?.last_seen_at || '')
  const day = new Date(now + 8 * 3600000).toISOString().slice(0, 10)
  return Boolean(status?.enabled && status.connected && status.metrics_available && status.day === day &&
    Number.isFinite(seen) && now - seen < 45000 && now >= seen - 5000)
}
