import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import AccountTagsInput from '../AccountTagsInput.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

/**
 * 工具：拿到组件内的 input 元素。组件根容器 div 不是 input 本身。
 */
function getInput(wrapper: ReturnType<typeof mount>) {
  return wrapper.find('input[type="text"]')
}

describe('AccountTagsInput', () => {
  // 覆盖 design checklist 退出条件之一："输入 'VIP ' 回车 emit ['vip']"
  // 验证：1) 大写规范化为小写  2) 回车键提交  3) emit update:modelValue 含新标签
  it("输入 'VIP' 回车后 emit update:modelValue=['vip']（小写规范化）", async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('VIP')
    await input.trigger('keydown', { key: 'Enter' })

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted![0]).toEqual([['vip']])
  })

  it('输入 "  Vip  " 含两端空格，回车后规范化为 "vip"', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('  Vip  ')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')![0]).toEqual([['vip']])
  })

  it('空格键也作为提交分隔符', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('vip')
    await input.trigger('keydown', { key: ' ' })

    expect(wrapper.emitted('update:modelValue')![0]).toEqual([['vip']])
  })

  // 覆盖 design checklist 退出条件之二："超长输入 emit invalid"
  // 31 字符（超过 ACCOUNT_TAG_MAX_LENGTH=30）触发 INVALID_ACCOUNT_TAG_LENGTH。
  it('超长（>30 字符）输入触发 emit invalid，且不修改 modelValue', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('a'.repeat(31))
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    const invalid = wrapper.emitted('invalid')
    expect(invalid).toBeTruthy()
    expect(invalid![0][0]).toMatchObject({ code: 'INVALID_ACCOUNT_TAG_LENGTH' })
  })

  it('非法字符（如 "vip!"）触发 INVALID_ACCOUNT_TAG_CHARSET', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('vip!')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('invalid')![0][0]).toMatchObject({ code: 'INVALID_ACCOUNT_TAG_CHARSET' })
  })

  it('CJK 中文标签合法直接通过', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: [] }
    })
    const input = getInput(wrapper)
    await input.setValue('测试')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')![0]).toEqual([['测试']])
  })

  // 覆盖 design checklist："点 chip × 后 emit 移除"
  it('点 chip × 按钮后 emit update:modelValue 不含该 tag', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: ['prod', 'vip'] }
    })
    // 找到第一个 chip 的删除按钮（aria-label 形如 admin.accounts.tags.remove）
    const removeButtons = wrapper.findAll('button[aria-label]')
    expect(removeButtons.length).toBe(2)
    await removeButtons[0].trigger('click')

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toEqual(['vip'])
  })

  it('已存在的标签重复输入不重复入列，仅清空 draft', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: ['vip'] }
    })
    const input = getInput(wrapper)
    await input.setValue('VIP') // 大写但规范化后等价于已存在的 'vip'
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect((input.element as HTMLInputElement).value).toBe('')
  })

  it('Backspace 在 draft 为空时移除最后一个 chip', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: ['alpha', 'vip'] }
    })
    const input = getInput(wrapper)
    await input.trigger('keydown', { key: 'Backspace' })

    const emitted = wrapper.emitted('update:modelValue')
    expect(emitted).toBeTruthy()
    // 移除最后一个 'vip'
    expect(emitted![0][0]).toEqual(['alpha'])
  })

  it('数量上限（20）超过时新增触发 TOO_MANY_ACCOUNT_TAGS', async () => {
    const initial = Array.from({ length: 20 }, (_, i) => `t${i}`)
    const wrapper = mount(AccountTagsInput, {
      props: { modelValue: initial }
    })
    const input = getInput(wrapper)
    await input.setValue('extra')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.emitted('invalid')![0][0]).toMatchObject({ code: 'TOO_MANY_ACCOUNT_TAGS' })
  })

  it('点 dropdown 候选项加入 modelValue', async () => {
    const wrapper = mount(AccountTagsInput, {
      props: {
        modelValue: [],
        suggestions: ['production', 'staging', 'vip']
      }
    })
    const input = getInput(wrapper)
    await input.trigger('focus')
    await input.setValue('pro')
    await nextTick()

    const items = wrapper.findAll('ul li')
    expect(items.length).toBe(1)
    expect(items[0].text()).toBe('production')
    await items[0].trigger('click')

    expect(wrapper.emitted('update:modelValue')![0]).toEqual([['production']])
  })
})
