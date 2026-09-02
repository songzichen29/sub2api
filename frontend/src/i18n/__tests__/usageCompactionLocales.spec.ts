import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('usage compaction locale keys', () => {
  it('contains English labels for the filter and usage badge', () => {
    expect(en.usage.compactionFilter).toBe('Request Kind')
    expect(en.usage.allCompactionTypes).toBe('All Requests')
    expect(en.usage.compactionOnly).toBe('Compaction Only')
    expect(en.usage.nativeCompactionV2).toBe('Compaction')
  })

  it('contains Chinese labels for the filter and usage badge', () => {
    expect(zh.usage.compactionFilter).toBe('请求类别')
    expect(zh.usage.allCompactionTypes).toBe('全部请求')
    expect(zh.usage.compactionOnly).toBe('仅原生压缩')
    expect(zh.usage.nativeCompactionV2).toBe('压缩')
  })
})
