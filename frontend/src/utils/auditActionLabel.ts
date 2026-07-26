export type AuditTranslate = (key: string) => string
export type AuditTranslationExists = (key: string) => boolean
export type AuditTranslationMessage = (key: string) => unknown

export function formatAuditActionLabel(
  action: string,
  t: AuditTranslate,
  te: AuditTranslationExists,
  tm: AuditTranslationMessage
): string {
  const normalized = action.startsWith('admin.') ? action.slice('admin.'.length) : action
  const actionKey = normalized.replace(/(^|\.)2fa(?=\.|$)/g, '$1twoFactor')
  const key = `admin.audit.actions.${actionKey}`
  const nestedLabelKey = `${key}._label`
  if (te(nestedLabelKey)) {
    return t(nestedLabelKey)
  }
  if (te(key) && typeof tm(key) === 'string') {
    return t(key)
  }

  return (
    normalized
      .split('.')
      .filter(Boolean)
      .map((segment) => {
        const segmentKey = `admin.audit.actionSegments.${segment === '2fa' ? 'twoFactor' : segment}`
        return te(segmentKey) ? t(segmentKey) : segment.replace(/[_-]+/g, ' ')
      })
      .join(' / ') || action
  )
}
