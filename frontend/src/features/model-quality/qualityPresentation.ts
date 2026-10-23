import type { QualityBenchmark } from '@/api/modelQuality'

export interface QualitySelection {
  planId: string
  batchId?: string
  groupId: number
  groupName: string
  planName?: string
  runId?: string
  benchmark?: QualityBenchmark
}

export function qualityBenchmarkLabel(benchmark: string, t: (key: string) => string): string {
  const key = `modelQuality.benchmarks.${benchmark}`
  const label = t(key)
  return label === key ? benchmark : label
}

export function qualityVerdict(run?: { status?: string; verdict: string }): string {
  if (!run) return 'empty'
  if (run.status === 'cancelled') return 'cancelled'
  return ['normal', 'degraded', 'error', 'unknown', 'pending'].includes(run.verdict) ? run.verdict : 'unknown'
}

export function qualityColor(run?: { status?: string; verdict: string }): string {
  switch (qualityVerdict(run)) {
    case 'normal': return 'bg-emerald-500'
    case 'degraded': return 'bg-yellow-400'
    case 'pending': return 'bg-gray-300 dark:bg-dark-500 animate-pulse'
    default: return 'bg-gray-400 dark:bg-dark-500'
  }
}

export function qualityTime(value: string): string {
  return new Date(value).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}
