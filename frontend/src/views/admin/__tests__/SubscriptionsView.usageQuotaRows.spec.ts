import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('admin subscriptions usage column', () => {
  it('renders weekly and monthly quota usage rows', () => {
    const source = readFileSync(resolve('src/views/admin/SubscriptionsView.vue'), 'utf8')

    expect(source).toContain('getOverdraftLimit(row) || row.group?.weekly_limit_usd')
    expect(source).toContain(": t('admin.subscriptions.weekly')")
    expect(source).toContain('getOverdraftDisplayUsed(row) ?? row.weekly_usage_usd')
    expect(source).toContain('!getOverdraftLimit(row) && row.group?.monthly_limit_usd')
    expect(source).toContain('getProgressClass(row.monthly_usage_usd, row.group.monthly_limit_usd)')
  })
})
