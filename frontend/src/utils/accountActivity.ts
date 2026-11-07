export type QQRewardKind = 'growth' | 'lottery' | 'invite' | 'checkin' | 'streak' | 'ranking' | 'redemption' | 'website_checkin'

export interface QQRewardNote {
  kind: QQRewardKind
  number: string
  detail: string
}

// Match only receipts written by the QQ reward jobs. Operator notes must stay verbatim.
export function parseQQRewardNote(notes?: string): QQRewardNote | null {
  const website = notes?.match(/^Website checkin reward #(\d+)$/i)
  if (website) return { kind: 'website_checkin', number: website[1]!, detail: '' }
  const match = notes?.match(/^QQ (growth|lottery|invite|checkin|streak|ranking|redemption) reward #(\d+)(?:: (.*))?$/i)
  if (!match) return null
  return { kind: match[1]!.toLowerCase() as QQRewardKind, number: match[2]!, detail: match[3] || '' }
}

export function parseQQCampaignNote(notes?: string): string | null {
  return notes?.match(/^QQ campaign ([a-f0-9]{24}|[a-f0-9]{32})$/i)?.[1] || null
}
