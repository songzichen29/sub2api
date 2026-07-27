import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import OpsErrorDetailsModal from '../OpsErrorDetailsModal.vue'

const listRequestErrors = vi.fn()
const listUpstreamErrors = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listRequestErrors: (...args: any[]) => listRequestErrors(...args),
    listUpstreamErrors: (...args: any[]) => listUpstreamErrors(...args),
  },
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const BaseDialogStub = defineComponent({
  props: { show: Boolean, title: String, width: String },
  emits: ['close'],
  template: '<div v-if="show"><slot /></div>',
})

const SelectStub = defineComponent({
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />',
})

const ErrorTableStub = defineComponent({
  name: 'OpsErrorLogTable',
  props: ['rows', 'total', 'loading', 'page', 'pageSize'],
  template: '<div class="error-table-stub" />',
})

describe('OpsErrorDetailsModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    listRequestErrors.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
    listUpstreamErrors.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 10 })
  })

  it('打开请求错误明细时默认查询全部记录，业务限制不会被隐藏', async () => {
    const wrapper = mount(OpsErrorDetailsModal, {
      props: {
        show: false,
        timeRange: '1h',
        platform: 'anthropic',
        groupId: 8,
        errorType: 'request',
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Select: SelectStub,
          OpsErrorLogTable: ErrorTableStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(listRequestErrors).toHaveBeenCalledWith(expect.objectContaining({
      time_range: '1h',
      platform: 'anthropic',
      group_id: 8,
      view: 'all',
    }))
  })
})
