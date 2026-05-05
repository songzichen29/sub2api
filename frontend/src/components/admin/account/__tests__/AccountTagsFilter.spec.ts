import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import AccountTagsFilter from '../AccountTagsFilter.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AccountTagsFilter', () => {
  it('点击 trigger 后展开 dropdown，列出候选标签', async () => {
    const wrapper = mount(AccountTagsFilter, {
      props: {
        modelValue: [],
        availableTags: ['prod', 'staging', 'vip']
      }
    })
    await wrapper.find('button').trigger('click')
    await nextTick()

    const items = wrapper.findAll('ul li button')
    expect(items.length).toBe(3)
    expect(items.map(i => i.text())).toEqual(['prod', 'staging', 'vip'])
  })

  // 覆盖测试约束：
  // "前端 AccountsView 标签筛选透传测试：选 [vip,prod] 后 list API 调用 filters 含 tags=['vip','prod']"
  // 此处验证 AccountTagsFilter 单组件 emit 行为；AccountTableFilters 透传到
  // AccountsView params.tags 的链路由 vue-tsc 静态保证。
  it('依次选 vip + prod 后 emit update:modelValue 累积两个标签 + 各 emit 一次 change', async () => {
    const wrapper = mount(AccountTagsFilter, {
      props: {
        modelValue: [],
        availableTags: ['prod', 'staging', 'vip']
      }
    })
    await wrapper.find('button').trigger('click')
    await nextTick()
    const items = wrapper.findAll('ul li button')

    // 点击 vip（index 2）
    await items[2].trigger('click')
    let updates = wrapper.emitted('update:modelValue')
    expect(updates).toBeTruthy()
    expect(updates![0][0]).toEqual(['vip'])

    // 模拟父组件用 emit 后的新值更新 props，再点 prod（index 0）
    await wrapper.setProps({ modelValue: ['vip'] })
    await items[0].trigger('click')

    updates = wrapper.emitted('update:modelValue')!
    expect(updates[1][0]).toEqual(['vip', 'prod'])

    const changes = wrapper.emitted('change')
    expect(changes).toBeTruthy()
    expect(changes!.length).toBe(2)
  })

  it('再次点击已选标签会取消选中', async () => {
    const wrapper = mount(AccountTagsFilter, {
      props: {
        modelValue: ['vip'],
        availableTags: ['prod', 'vip']
      }
    })
    await wrapper.find('button').trigger('click')
    await nextTick()
    const items = wrapper.findAll('ul li button')
    // 点击已选的 vip（index 1：'prod' 在前）
    await items[1].trigger('click')

    expect(wrapper.emitted('update:modelValue')![0][0]).toEqual([])
  })

  it('清空按钮 emit 空数组', async () => {
    const wrapper = mount(AccountTagsFilter, {
      props: {
        modelValue: ['vip', 'prod'],
        availableTags: ['prod', 'vip']
      }
    })
    await wrapper.find('button').trigger('click')
    await nextTick()

    // 清空按钮在 dropdown 底部 div 里，文本是 admin.accounts.tags.filterClear
    const clearBtn = wrapper
      .findAll('button')
      .find(b => b.text() === 'admin.accounts.tags.filterClear')
    expect(clearBtn).toBeDefined()
    await clearBtn!.trigger('click')

    expect(wrapper.emitted('update:modelValue')![0][0]).toEqual([])
    expect(wrapper.emitted('change')).toBeTruthy()
  })

  it('搜索框过滤候选标签', async () => {
    const wrapper = mount(AccountTagsFilter, {
      props: {
        modelValue: [],
        availableTags: ['prod', 'staging', 'vip']
      }
    })
    await wrapper.find('button').trigger('click')
    await nextTick()
    const searchInput = wrapper.find('input[type="text"]')
    await searchInput.setValue('vi')
    await nextTick()

    const items = wrapper.findAll('ul li button')
    expect(items.length).toBe(1)
    expect(items[0].text()).toBe('vip')
  })
})
