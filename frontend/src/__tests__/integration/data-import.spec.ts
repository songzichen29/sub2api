import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import JSZip from 'jszip'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import type { AccountImportApplyTemplate } from '@/api/admin/settings'
import type { AdminGroup } from '@/types'

const showError = vi.fn()
const showSuccess = vi.fn()
const importDataMock = vi.fn()
const getTemplatesMock = vi.fn()
const updateTemplatesMock = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: (...args: unknown[]) => importDataMock(...args)
    },
    settings: {
      getAccountImportTemplates: (...args: unknown[]) => getTemplatesMock(...args),
      updateAccountImportTemplates: (...args: unknown[]) => updateTemplatesMock(...args)
    }
  }
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

// 子组件全部 stub 成空 div + 透传 props/emits，让我们能用 findComponent + vm.$emit
// 直接模拟"用户在子组件里输入并触发 update:modelValue"。
const stubChild = (name: string, props: string[]) => ({
  name,
  template: `<div data-stub="${name}" />`,
  props,
  emits: ['update:modelValue', 'invalid']
})

const makeGroup = (id: number, name = `group-${id}`): AdminGroup => ({
  id,
  name,
  description: null,
  platform: 'anthropic',
  rate_multiplier: 1,
  rpm_limit: 0,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  daily_limit_reset_price: null,
  allow_daily_overdraft: false,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  allow_messages_dispatch: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: false,
  sort_order: id
})

const makeTemplate = (
  overrides: Partial<AccountImportApplyTemplate> = {}
): AccountImportApplyTemplate => ({
  id: 'tpl-default',
  name: 'default',
  enableTags: false,
  enableGroups: false,
  enableProxy: false,
  enableConcurrency: false,
  enablePriority: false,
  enableModelRestriction: false,
  applyTags: [],
  applyGroupIds: [],
  applyProxyId: null,
  applyConcurrency: 1,
  applyPriority: 1,
  modelRestrictionMode: 'whitelist',
  allowedModels: [],
  modelMappings: [],
  ...overrides
})

const mountModal = (props: Record<string, unknown> = {}) =>
  mount(ImportDataModal, {
    props: { show: true, proxies: [], groups: [], availableTags: [], ...props },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        ProxySelector: stubChild('ProxySelector', ['modelValue', 'proxies', 'disabled']),
        GroupSelector: stubChild('GroupSelector', ['modelValue', 'groups']),
        AccountTagsInput: stubChild('AccountTagsInput', ['modelValue', 'suggestions', 'disabled', 'placeholder']),
        ModelWhitelistSelector: stubChild('ModelWhitelistSelector', ['modelValue']),
        Select: stubChild('Select', ['modelValue', 'options', 'placeholder']),
        Icon: true
      }
    }
  })

const attachJsonFile = async (
  wrapper: ReturnType<typeof mountModal>,
  payload: Record<string, unknown>,
  name = 'data.json'
) => {
  const input = wrapper.find('input[type="file"]')
  const file = new File([JSON.stringify(payload)], name, { type: 'application/json' })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(JSON.stringify(payload))
  })
  Object.defineProperty(input.element, 'files', { value: [file] })
  await input.trigger('change')
}

const attachJsonFiles = async (
  wrapper: ReturnType<typeof mountModal>,
  filesPayload: Array<{ name: string; payload: Record<string, unknown> }>
) => {
  const input = wrapper.find('input[type="file"]')
  const files = filesPayload.map(({ name, payload }) => {
    const file = new File([JSON.stringify(payload)], name, { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(payload))
    })
    return file
  })
  Object.defineProperty(input.element, 'files', { value: files })
  await input.trigger('change')
}

const attachZipFile = async (
  wrapper: ReturnType<typeof mountModal>,
  entries: Array<{ name: string; payload: Record<string, unknown> }>
) => {
  const input = wrapper.find('input[type="file"]')
  const zip = new JSZip()
  for (const { name, payload } of entries) {
    zip.file(name, JSON.stringify(payload))
  }
  zip.file('README.txt', 'ignored')

  const buffer = await zip.generateAsync({ type: 'arraybuffer' })
  const file = new File([buffer], 'bundle.zip', { type: 'application/zip' })
  Object.defineProperty(file, 'arrayBuffer', {
    value: () => Promise.resolve(buffer)
  })
  Object.defineProperty(input.element, 'files', { value: [file] })
  await input.trigger('change')
}

const sampleData = {
  type: 'sub2api-data',
  version: 1,
  proxies: [],
  accounts: [
    {
      name: 'acc-1',
      platform: 'anthropic',
      type: 'oauth',
      credentials: { access_token: 'tok' },
      concurrency: 3,
      priority: 50
    }
  ]
}

describe('ImportDataModal', () => {
  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    importDataMock.mockReset()
    getTemplatesMock.mockReset()
    updateTemplatesMock.mockReset()
    importDataMock.mockResolvedValue({
      account_created: 1,
      account_failed: 0,
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      errors: []
    })
    getTemplatesMock.mockResolvedValue([])
    updateTemplatesMock.mockImplementation(async (templates) => templates)
  })

  it('未选择文件时提示错误', async () => {
    const wrapper = mountModal()
    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时提示解析失败', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = new File(['invalid json'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('invalid json')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
  })

  it('支持选择多个 JSON 文件并合并后导入', async () => {
    const wrapper = mountModal()

    await attachJsonFiles(wrapper, [
      {
        name: 'part-1.json',
        payload: {
          ...sampleData,
          proxies: [
            {
              proxy_key: 'http|127.0.0.1|8080||',
              name: 'px-1',
              protocol: 'http',
              host: '127.0.0.1',
              port: 8080,
              status: 'active'
            }
          ],
          accounts: [
            {
              name: 'acc-1',
              platform: 'anthropic',
              type: 'oauth',
              credentials: { access_token: 'tok-1' },
              concurrency: 3,
              priority: 50
            }
          ]
        }
      },
      {
        name: 'part-2.json',
        payload: {
          ...sampleData,
          proxies: [],
          accounts: [
            {
              name: 'acc-2',
              platform: 'openai',
              type: 'apikey',
              credentials: { api_key: 'tok-2' },
              concurrency: 1,
              priority: 10
            }
          ]
        }
      }
    ])

    await wrapper.find('form').trigger('submit')
    await flushPromises()
    expect(importDataMock).toHaveBeenCalledTimes(1)
    expect(importDataMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          type: 'sub2api-data',
          version: 1,
          proxies: expect.arrayContaining([
            expect.objectContaining({ name: 'px-1' })
          ]),
          accounts: expect.arrayContaining([
            expect.objectContaining({ name: 'acc-1' }),
            expect.objectContaining({ name: 'acc-2' })
          ])
        })
      })
    )
  })

  it('支持选择 ZIP 文件并导入其中所有 JSON 文件', async () => {
    const wrapper = mountModal()

    await attachZipFile(wrapper, [
      {
        name: 'exports/part-1.json',
        payload: {
          ...sampleData,
          proxies: [
            {
              proxy_key: 'http|10.0.0.1|8080||',
              name: 'zip-px-1',
              protocol: 'http',
              host: '10.0.0.1',
              port: 8080,
              status: 'active'
            }
          ],
          accounts: [
            {
              name: 'zip-acc-1',
              platform: 'anthropic',
              type: 'oauth',
              credentials: { access_token: 'zip-tok-1' },
              concurrency: 3,
              priority: 50
            }
          ]
        }
      },
      {
        name: 'part-2.json',
        payload: {
          ...sampleData,
          proxies: [],
          accounts: [
            {
              name: 'zip-acc-2',
              platform: 'openai',
              type: 'apikey',
              credentials: { api_key: 'zip-tok-2' },
              concurrency: 1,
              priority: 10
            }
          ]
        }
      }
    ])

    await wrapper.find('form').trigger('submit')

    await vi.waitFor(() => {
      expect(importDataMock).toHaveBeenCalledTimes(1)
    })
    expect(importDataMock).toHaveBeenCalledWith(
      expect.objectContaining({
        data: expect.objectContaining({
          type: 'sub2api-data',
          version: 1,
          proxies: expect.arrayContaining([
            expect.objectContaining({ name: 'zip-px-1' })
          ]),
          accounts: expect.arrayContaining([
            expect.objectContaining({ name: 'zip-acc-1' }),
            expect.objectContaining({ name: 'zip-acc-2' })
          ])
        })
      })
    )
  })

  // ===== feature 2026-05-06-account-import-apply：折叠面板 + 6 字段 =====

  it('Apply 面板默认折叠（details 不带 open 属性）', () => {
    const wrapper = mountModal()
    const details = wrapper.find('details')
    expect(details.exists()).toBe(true)
    // jsdom 上 details.open 默认 false
    expect((details.element as HTMLDetailsElement).open).toBe(false)
  })

  it('未勾选任何 Apply 字段时提交，payload 不含 apply 键', async () => {
    const wrapper = mountModal()
    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as Record<string, unknown>
    expect(arg.apply).toBeUndefined()
  })

  it('勾选 tags 并填值后，payload.apply 仅含 tags 字段', async () => {
    const wrapper = mountModal()

    await wrapper.find('input#import-apply-tags-enabled').setValue(true)
    await wrapper.findComponent({ name: 'AccountTagsInput' }).vm.$emit('update:modelValue', ['vip'])

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    expect(arg.apply).toEqual({ tags: ['vip'] })
  })

  it('勾选 proxy 但未选具体代理时，payload.apply 不含 proxy_id', async () => {
    const wrapper = mountModal()
    await wrapper.find('input#import-apply-proxy-enabled').setValue(true)
    // 不触发 ProxySelector 的 update:modelValue → applyProxyId 保持初始 null

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    // 启用了 proxy 但 proxyId=null，apply 仍应不含 proxy_id；apply 整体也应为 undefined
    // （没有任何字段被有效启用）
    expect(arg.apply).toBeUndefined()
  })

  it('勾选 priority + concurrency 后 payload.apply 含两数字字段', async () => {
    const wrapper = mountModal()

    await wrapper.find('input#import-apply-concurrency-enabled').setValue(true)
    await wrapper.find('input#import-apply-concurrency').setValue(10)
    await wrapper.find('input#import-apply-priority-enabled').setValue(true)
    await wrapper.find('input#import-apply-priority').setValue(1)

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    expect(arg.apply).toEqual({ concurrency: 10, priority: 1 })
  })

  it('勾选 groups 触发子组件更新后 payload.apply.group_ids 正确', async () => {
    const wrapper = mountModal({ groups: [makeGroup(5), makeGroup(7)] })
    await wrapper.find('input#import-apply-groups-enabled').setValue(true)
    await wrapper.findComponent({ name: 'GroupSelector' }).vm.$emit('update:modelValue', [5, 7])

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    expect(arg.apply).toEqual({ group_ids: [5, 7] })
  })

  it('套用模板时过滤已删除分组，避免导入账号重新显示旧分组', async () => {
    getTemplatesMock.mockResolvedValue([
      makeTemplate({
        id: 'tpl-with-deleted-group',
        name: '旧模板',
        enableGroups: true,
        applyGroupIds: [5, 99, 7, 5]
      })
    ])
    const wrapper = mountModal({ groups: [makeGroup(5), makeGroup(7)] })
    await flushPromises()

    await wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', 'tpl-with-deleted-group')
    await flushPromises()

    expect(wrapper.findComponent({ name: 'GroupSelector' }).props('modelValue')).toEqual([5, 7])

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    expect(arg.apply).toEqual({ group_ids: [5, 7] })
  })

  it('修改已选模板后再次导入会先更新模板内容', async () => {
    getTemplatesMock.mockResolvedValue([
      makeTemplate({
        id: 'tpl-concurrency',
        name: '并发模板',
        enableConcurrency: true,
        applyConcurrency: 2
      })
    ])
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', 'tpl-concurrency')
    await wrapper.find('input#import-apply-concurrency').setValue(8)
    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(updateTemplatesMock).toHaveBeenCalledWith([
      expect.objectContaining({
        id: 'tpl-concurrency',
        name: '并发模板',
        enableConcurrency: true,
        applyConcurrency: 8
      })
    ])
    expect(importDataMock).toHaveBeenCalledWith(
      expect.objectContaining({ apply: { concurrency: 8 } })
    )
    expect(updateTemplatesMock.mock.invocationCallOrder[0]).toBeLessThan(
      importDataMock.mock.invocationCallOrder[0]
    )
  })

  it('已选模板更新失败时不会继续导入账号', async () => {
    getTemplatesMock.mockResolvedValue([makeTemplate()])
    updateTemplatesMock.mockRejectedValue(new Error('template save failed'))
    const wrapper = mountModal()
    await flushPromises()

    await wrapper.findComponent({ name: 'Select' }).vm.$emit('update:modelValue', 'tpl-default')
    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('template save failed')
  })

  it('勾选 model restriction 白名单模式后 payload.apply.model_mapping 为 key=value 形式', async () => {
    const wrapper = mountModal()

    await wrapper.find('input#import-apply-model-enabled').setValue(true)
    // 默认 mode 是 whitelist
    await wrapper.findComponent({ name: 'ModelWhitelistSelector' }).vm.$emit('update:modelValue', ['claude-3-5-sonnet-20241022'])

    await attachJsonFile(wrapper, sampleData)
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importDataMock).toHaveBeenCalledTimes(1)
    const arg = importDataMock.mock.calls[0][0] as { apply?: Record<string, unknown> }
    expect(arg.apply).toEqual({
      model_mapping: { 'claude-3-5-sonnet-20241022': 'claude-3-5-sonnet-20241022' }
    })
  })

  it('关闭弹窗再打开，所有 Apply 字段 enable 状态被重置为 false', async () => {
    const wrapper = mountModal()

    await wrapper.find('input#import-apply-tags-enabled').setValue(true)
    await wrapper.find('input#import-apply-priority-enabled').setValue(true)
    await wrapper.find('input#import-apply-concurrency-enabled').setValue(true)
    expect((wrapper.find('input#import-apply-tags-enabled').element as HTMLInputElement).checked).toBe(true)

    // 关闭弹窗
    await wrapper.setProps({ show: false })
    // 重新打开 → watch(props.show) 应该把 enableXxx reset 为 false
    await wrapper.setProps({ show: true })

    expect((wrapper.find('input#import-apply-tags-enabled').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.find('input#import-apply-priority-enabled').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.find('input#import-apply-concurrency-enabled').element as HTMLInputElement).checked).toBe(false)
  })
})
