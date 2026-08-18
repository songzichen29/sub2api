import { describe, expect, it } from 'vitest'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import en from '../locales/en'
import zh from '../locales/zh'

const sourceRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../')

function flattenLeafKeys(value: unknown, prefix = ''): Set<string> {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return new Set(prefix ? [prefix] : [])
  }

  const keys = new Set<string>()
  for (const [key, child] of Object.entries(value as Record<string, unknown>)) {
    for (const leaf of flattenLeafKeys(child, prefix ? `${prefix}.${key}` : key)) {
      keys.add(leaf)
    }
  }
  return keys
}

function collectSourceFiles(directory: string): string[] {
  const files: string[] = []
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const entryPath = path.join(directory, entry.name)
    if (entry.isDirectory()) {
      if (entry.name !== 'node_modules') files.push(...collectSourceFiles(entryPath))
      continue
    }
    if (!/\.(?:vue|ts|tsx)$/.test(entry.name)) continue
    if (entry.name.includes('.spec.') || entry.name.includes('.test.')) continue
    if (entryPath.includes(`${path.sep}__tests__${path.sep}`)) continue
    files.push(entryPath)
  }
  return files
}

function collectLiteralTranslationKeys(): Map<string, string[]> {
  const usages = new Map<string, string[]>()
  const callPattern = /(?:\bt|\$t|i18n\.t)\s*\(\s*(['"])([^'"\\]+)\1/g

  for (const file of collectSourceFiles(sourceRoot)) {
    const source = fs.readFileSync(file, 'utf8')
    for (const match of source.matchAll(callPattern)) {
      const key = match[2]
      const locations = usages.get(key) ?? []
      locations.push(path.relative(sourceRoot, file))
      usages.set(key, locations)
    }
  }
  return usages
}

describe('page locale key coverage', () => {
  it('defines every literal translation key used by page code in both locales', () => {
    const enKeys = flattenLeafKeys(en)
    const zhKeys = flattenLeafKeys(zh)
    const missing = [...collectLiteralTranslationKeys().entries()]
      // Dynamic namespaces such as `t('payment.status.' + status)` cannot be
      // resolved statically; their fixed prefix ends with a dot.
      .filter(([key]) => !key.endsWith('.') && (!enKeys.has(key) || !zhKeys.has(key)))
      .map(([key, files]) => ({ key, files: [...new Set(files)].sort() }))
      .sort((a, b) => a.key.localeCompare(b.key))

    expect(missing).toEqual([])
  })
})
