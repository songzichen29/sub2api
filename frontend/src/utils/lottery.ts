// Defense in depth for links restored from older or externally edited settings.
export function safeJoinURL(value: string): boolean {
  try { const url = new URL(value); return url.protocol === 'https:' && ['qun.qq.com', 'qm.qq.com'].includes(url.host) && !url.username && !url.password }
  catch { return false }
}
