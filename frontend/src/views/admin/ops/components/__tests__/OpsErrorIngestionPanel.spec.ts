import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsErrorIngestionPanel from '../OpsErrorIngestionPanel.vue'
import zhLocale from '@/i18n/locales/zh'
import enLocale from '@/i18n/locales/en'

const getErrorLogIngestionHealth = vi.fn()
const getIngressRejectHealth = vi.fn()
const listIngressRejections = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getErrorLogIngestionHealth: (...args: any[]) => getErrorLogIngestionHealth(...args),
    getIngressRejectHealth: (...args: any[]) => getIngressRejectHealth(...args),
    listIngressRejections: (...args: any[]) => listIngressRejections(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

describe('OpsErrorIngestionPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    getErrorLogIngestionHealth.mockResolvedValue({
      queue_depth: 1,
      queue_capacity: 256,
      queue_bytes: 512,
      queue_bytes_capacity: 1024,
      enqueued_count: 12,
      processed_count: 11,
      persisted_count: 9,
      skipped_count: 1,
      dropped_count: 1,
      write_failed_count: 1,
      sanitized_count: 0,
      workers_started: true,
      accepting: true,
      last_error: 'database unavailable',
      last_error_at: '2026-07-27T01:00:00Z',
    })
    getIngressRejectHealth.mockResolvedValue({
      cardinality: 2,
      capacity: 8192,
      pending_batches: 0,
      pending_rows: 0,
      overflowed_count: 0,
      dropped_count: 0,
      flushed_request_count: 4,
      flush_failure_count: 0,
      accepting: true,
    })
    listIngressRejections.mockResolvedValue({
      items: [{
        id: 1,
        bucket_start: '2026-07-27T01:00:00Z',
        reject_reason: 'invalid_api_key',
        route_family: 'messages',
        protocol: 'anthropic',
        client_ip: '127.0.0.1',
        request_count: 4,
        first_seen: '2026-07-27T01:00:00Z',
        last_seen: '2026-07-27T01:01:00Z',
      }],
      total: 1,
      page: 1,
      page_size: 10,
    })
  })

  it('展示真实落库健康指标和入口拒绝列表', async () => {
    const wrapper = mount(OpsErrorIngestionPanel, {
      props: { timeRange: '1h', refreshToken: 1 },
    })
    await flushPromises()

    expect(getErrorLogIngestionHealth).toHaveBeenCalledOnce()
    expect(getIngressRejectHealth).toHaveBeenCalledOnce()
    expect(listIngressRejections).toHaveBeenCalledWith({ page: 1, page_size: 10, time_range: '1h' })
    expect(wrapper.text()).toContain('admin.ops.errorIngestion.persisted 9')
    expect(wrapper.text()).toContain('admin.ops.errorIngestion.failed 1')
    expect(wrapper.text()).toContain('database unavailable')
    expect(wrapper.text()).toContain('invalid_api_key')
    expect(wrapper.text()).toContain('messages / anthropic')
    expect(wrapper.get('[data-testid="ingestion-state"]').text()).toContain('warning')
  })

  it('中英文都包含采集状态和全部入口拒绝原因翻译', () => {
    const reasonKeys = [
      'query_api_key_deprecated', 'api_key_required', 'invalid_api_key',
      'invalid_auth_rate_limited', 'api_key_auth_overloaded', 'api_key_disabled',
      'ip_restricted', 'user_inactive', 'group_deleted', 'group_disabled',
      'group_not_allowed', 'group_unassigned', 'other',
    ]
    for (const locale of [zhLocale, enLocale]) {
      const messages = locale.admin.ops.errorIngestion
      expect(messages.title).toBeTruthy()
      expect(messages.errorPipeline).toBeTruthy()
      expect(messages.rejectPipeline).toBeTruthy()
      for (const key of reasonKeys) {
        expect(messages.reasons[key as keyof typeof messages.reasons]).toBeTruthy()
      }
    }
  })
})
