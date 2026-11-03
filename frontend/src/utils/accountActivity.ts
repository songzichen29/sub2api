export type QQRewardKind = 'growth' | 'lottery' | 'invite' | 'checkin' | 'streak' | 'ranking' | 'redemption'

export interface QQRewardNote {
  kind: QQRewardKind
  number: string
  detail: string
}

// Match only receipts written by the QQ reward jobs. Operator notes must stay verbatim.
export function parseQQRewardNote(notes?: string): QQRewardNote | null {
  const match = notes?.match(/^QQ (growth|lottery|invite|checkin|streak|ranking|redemption) reward #(\d+)(?:: (.*))?$/i)
  if (!match) return null
  return { kind: match[1]!.toLowerCase() as QQRewardKind, number: match[2]!, detail: match[3] || '' }
}
