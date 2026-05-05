import { describe, expect, it } from 'vitest'
import {
  ACCOUNT_TAG_MAX_COUNT,
  ACCOUNT_TAG_MAX_LENGTH,
  countAccountTagRunes,
  normalizeAccountTag,
  normalizeAccountTags,
  validateAccountTag
} from '../accountTags'

describe('accountTags utils', () => {
  describe('countAccountTagRunes', () => {
    it('ASCII 字符串按字符数计', () => {
      expect(countAccountTagRunes('abc')).toBe(3)
    })
    it('CJK 字符按 code point 数计而非 UTF-16 unit 数', () => {
      // 4 个汉字 = 4 个 code point。String.length 也是 4（基本平面汉字）。
      expect(countAccountTagRunes('测试标签')).toBe(4)
    })
    it('Surrogate pair（emoji 等）按 1 计而非 2', () => {
      // 🎉 是 U+1F389，UTF-16 占 2 unit；按 rune 应为 1
      expect(countAccountTagRunes('🎉')).toBe(1)
    })
  })

  describe('normalizeAccountTag', () => {
    it('trim 首尾空白并转小写', () => {
      expect(normalizeAccountTag('  VIP  ')).toBe('vip')
    })
    it('空白字符串返回 null', () => {
      expect(normalizeAccountTag('   ')).toBeNull()
      expect(normalizeAccountTag('')).toBeNull()
    })
    it('CJK 字符规范化后保持原样（小写化对中文无影响）', () => {
      expect(normalizeAccountTag(' 测试 ')).toBe('测试')
    })
  })

  describe('validateAccountTag', () => {
    it('合法 ASCII 标签返回 null', () => {
      expect(validateAccountTag('vip-prod_2024')).toBeNull()
    })
    it('合法 CJK 标签返回 null', () => {
      expect(validateAccountTag('生产环境')).toBeNull()
    })
    it('超长返回 INVALID_ACCOUNT_TAG_LENGTH', () => {
      const tag = 'a'.repeat(ACCOUNT_TAG_MAX_LENGTH + 1)
      expect(validateAccountTag(tag)).toEqual({ code: 'INVALID_ACCOUNT_TAG_LENGTH', tag })
    })
    it('恰好 30 字符是合法的（边界值）', () => {
      const tag = 'a'.repeat(ACCOUNT_TAG_MAX_LENGTH)
      expect(validateAccountTag(tag)).toBeNull()
    })
    it('非法字符（如 !）返回 INVALID_ACCOUNT_TAG_CHARSET', () => {
      expect(validateAccountTag('vip!')).toEqual({ code: 'INVALID_ACCOUNT_TAG_CHARSET', tag: 'vip!' })
    })
    it('空格在标签中是非法字符（应在外层 trim 后调用）', () => {
      expect(validateAccountTag('vip prod')).toEqual({
        code: 'INVALID_ACCOUNT_TAG_CHARSET',
        tag: 'vip prod'
      })
    })
  })

  describe('normalizeAccountTags', () => {
    it('空输入返回空数组无错误', () => {
      expect(normalizeAccountTags(null)).toEqual({ tags: [], error: null })
      expect(normalizeAccountTags(undefined)).toEqual({ tags: [], error: null })
      expect(normalizeAccountTags([])).toEqual({ tags: [], error: null })
    })

    it('混合大小写 + 空白 + 重复后规范化、去重、字典序排序', () => {
      expect(normalizeAccountTags(['VIP', 'prod', '  vip ', 'Prod', 'alpha'])).toEqual({
        tags: ['alpha', 'prod', 'vip'],
        error: null
      })
    })

    it('空字符串元素被跳过不报错', () => {
      expect(normalizeAccountTags(['', 'vip', '   '])).toEqual({
        tags: ['vip'],
        error: null
      })
    })

    it('任一标签超长整体失败，tags 为空', () => {
      const result = normalizeAccountTags(['vip', 'a'.repeat(31)])
      expect(result.tags).toEqual([])
      expect(result.error?.code).toBe('INVALID_ACCOUNT_TAG_LENGTH')
    })

    it('任一标签字符集非法整体失败', () => {
      const result = normalizeAccountTags(['vip', 'has space'])
      expect(result.tags).toEqual([])
      expect(result.error?.code).toBe('INVALID_ACCOUNT_TAG_CHARSET')
    })

    it('数量超过上限整体失败', () => {
      const input = Array.from({ length: ACCOUNT_TAG_MAX_COUNT + 1 }, (_, i) => `t${i}`)
      const result = normalizeAccountTags(input)
      expect(result.tags).toEqual([])
      expect(result.error?.code).toBe('TOO_MANY_ACCOUNT_TAGS')
    })

    it('恰好 20 个标签是合法的（边界值）', () => {
      const input = Array.from({ length: ACCOUNT_TAG_MAX_COUNT }, (_, i) => `t${String(i).padStart(2, '0')}`)
      const result = normalizeAccountTags(input)
      expect(result.tags.length).toBe(ACCOUNT_TAG_MAX_COUNT)
      expect(result.error).toBeNull()
    })
  })
})
