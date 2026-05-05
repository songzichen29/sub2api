/**
 * 账号标签前端规范化工具，行为与后端 service.NormalizeAccountTags 严格一致。
 *
 * 后端实现位置：backend/internal/service/account_tags.go。
 * 任何规则改动必须前后端同步——后端 unit test 在 account_service_tags_test.go，
 * 本工具的 unit test 在 utils/__tests__/accountTags.spec.ts。
 *
 * 规则：
 *   - trim：去除首尾空白
 *   - 统一小写（仅影响 ASCII 字母；CJK 不变）
 *   - 单标签长度 ≤ ACCOUNT_TAG_MAX_LENGTH（按 rune 计）
 *   - 单账号数量 ≤ ACCOUNT_TAG_MAX_COUNT
 *   - 字符集：CJK 统一表意文字 + ASCII 字母数字 + `-` + `_`
 *   - 去重（保留首次出现）→ 字典序排序
 */

export const ACCOUNT_TAG_MAX_LENGTH = 30
export const ACCOUNT_TAG_MAX_COUNT = 20

export type AccountTagErrorCode =
  | 'INVALID_ACCOUNT_TAG_LENGTH'
  | 'INVALID_ACCOUNT_TAG_CHARSET'
  | 'TOO_MANY_ACCOUNT_TAGS'

export interface AccountTagError {
  code: AccountTagErrorCode
  /** 触发错误的标签（已规范化为小写）；TOO_MANY_ACCOUNT_TAGS 时为空字符串。 */
  tag: string
}

/**
 * 字符集校验正则。要求：
 *   - 至少 1 个字符
 *   - 仅由 [Han 表意文字 / a-z / 0-9 / - / _] 组成
 *
 * 使用 Unicode property `\p{Script=Han}` 覆盖与后端 `unicode.Is(unicode.Han, r)`
 * 等价的字符范围（含 CJK 扩展区域）。`u` 标志启用 Unicode 模式。
 */
const ACCOUNT_TAG_VALID_REGEX = /^[\p{Script=Han}a-z0-9_-]+$/u

/**
 * 计算字符串的 rune 数（Unicode code point 数）。
 *
 * JS 的 String.length 计的是 UTF-16 code unit，对 surrogate pair 表示的字符
 * 会算成 2。我们要和 Go 的 utf8.RuneCount 行为一致——按 code point 数计。
 * `[...s]` 由 String iterator 拆分，等价于按 code point 计数。
 */
export function countAccountTagRunes(s: string): number {
  return [...s].length
}

/**
 * 单个原始输入规范化（trim + lower）。空白串返回 null 表示"忽略"。
 * 不做长度/字符集校验，调用方拿到非 null 结果后应再调用 validateAccountTag。
 */
export function normalizeAccountTag(raw: string): string | null {
  const tag = raw.trim().toLowerCase()
  return tag === '' ? null : tag
}

/** 校验单个已规范化的标签。合法返回 null，违规返回错误对象。 */
export function validateAccountTag(tag: string): AccountTagError | null {
  if (countAccountTagRunes(tag) > ACCOUNT_TAG_MAX_LENGTH) {
    return { code: 'INVALID_ACCOUNT_TAG_LENGTH', tag }
  }
  if (!ACCOUNT_TAG_VALID_REGEX.test(tag)) {
    return { code: 'INVALID_ACCOUNT_TAG_CHARSET', tag }
  }
  return null
}

export interface NormalizeAccountTagsResult {
  /** 规范化后的合法标签数组（可能为空数组，永远非 null）。 */
  tags: string[]
  /** 任一标签违规时的错误对象；全部合法时为 null。错误时 tags 为空数组。 */
  error: AccountTagError | null
}

/**
 * 批量规范化 + 校验 + 去重 + 排序。
 *
 * 与后端 NormalizeAccountTags 行为一致：任一标签违反长度/字符集规则时
 * 整体失败（返回 error，tags 为 []），不做兜底跳过。数量超限同样整体失败。
 */
export function normalizeAccountTags(input: readonly string[] | null | undefined): NormalizeAccountTagsResult {
  if (!input || input.length === 0) {
    return { tags: [], error: null }
  }

  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of input) {
    const tag = normalizeAccountTag(raw)
    if (tag === null) continue
    const err = validateAccountTag(tag)
    if (err) return { tags: [], error: err }
    if (seen.has(tag)) continue
    seen.add(tag)
    out.push(tag)
  }

  if (out.length > ACCOUNT_TAG_MAX_COUNT) {
    return { tags: [], error: { code: 'TOO_MANY_ACCOUNT_TAGS', tag: '' } }
  }

  out.sort()
  return { tags: out, error: null }
}
