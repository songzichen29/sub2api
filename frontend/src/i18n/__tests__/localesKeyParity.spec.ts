import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

function flattenLeafKeys(value: unknown, prefix = ''): string[] {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return prefix ? [prefix] : []
  }

  return Object.entries(value as Record<string, unknown>).flatMap(([key, child]) =>
    flattenLeafKeys(child, prefix ? `${prefix}.${key}` : key)
  )
}

describe('locale key parity', () => {
  it('keeps English and Chinese locale leaf keys in sync', () => {
    expect(flattenLeafKeys(en).sort()).toEqual(flattenLeafKeys(zh).sort())
  })
})
