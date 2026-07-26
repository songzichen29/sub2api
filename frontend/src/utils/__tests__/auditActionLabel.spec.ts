import { describe, expect, it } from 'vitest'

import enAudit from '@/i18n/locales/en/admin/audit'
import zhAudit from '@/i18n/locales/zh/admin/audit'
import { formatAuditActionLabel } from '@/utils/auditActionLabel'

type MessageTree = Record<string, unknown>

function resolvePath(tree: MessageTree, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (typeof current !== 'object' || current === null) return undefined
    return (current as MessageTree)[segment]
  }, tree)
}

function formatter(locale: MessageTree) {
  const messages = { admin: locale }
  return (action: string) =>
    formatAuditActionLabel(
      action,
      (key) => String(resolvePath(messages, key)),
      (key) => resolvePath(messages, key) !== undefined,
      (key) => resolvePath(messages, key)
    )
}

function leafKeys(value: unknown, prefix = ''): string[] {
  if (typeof value !== 'object' || value === null) return [prefix]
  return Object.entries(value).flatMap(([key, child]) => leafKeys(child, prefix ? `${prefix}.${key}` : key))
}

describe('formatAuditActionLabel', () => {
  const en = formatter(enAudit)
  const zh = formatter(zhAudit)

  it('uses exact translations for explicit and sensitive-read actions', () => {
    expect(en('auth.login')).toBe('Sign in')
    expect(zh('auth.login.2fa')).toBe('两步验证登录')
    expect(en('admin.proxies.export')).toBe('Export proxies')
    expect(zh('admin.redeem_codes.export')).toBe('导出兑换码')
    expect(zh('admin.data_management.s3_config.read')).toBe('查看数据管理 S3 配置')
  })

  it('localizes route-derived action segments without mixed-language output', () => {
    expect(en('admin.accounts.clear_error.create')).toBe('Accounts / Clear error / Create')
    expect(zh('admin.accounts.clear_error.create')).toBe('账号 / 清除错误 / 新建')
    expect(en('admin.ops.system_logs.cleanup.create')).toBe('Operations / System logs / Cleanup / Create')
    expect(zh('admin.ops.system_logs.cleanup.create')).toBe('运维 / 系统日志 / 清理 / 新建')
  })

  it('keeps English and Chinese audit locale keys in parity', () => {
    expect(leafKeys(enAudit).sort()).toEqual(leafKeys(zhAudit).sort())
  })
})
