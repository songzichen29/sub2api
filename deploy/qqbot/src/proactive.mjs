import { beijingTime } from './time.mjs';
import { opaqueKey } from './policy.mjs';

// Five minutes permits serial model generation for several groups, but never
// replays old slots on restart. A claimed slot is never generated twice.
export const slotLifetime = 5 * 60000;
export function proactiveAllowed(c, chat) {
  return c.enabled && !c.chat_disabled && c.proactive_enabled &&
    c.groups.includes(chat) && (c.proactive_groups || []).includes(chat) &&
    !(c.group_policies || []).some(p => p.group_id === chat && (p.disable_chat || p.disable_notifications));
}

export function nextProactive(c, store, now, startedAt) {
  if (!c.proactive_enabled || !c.enabled || c.chat_disabled) return;
  const day = beijingTime(now).slice(0, 10);
  const startupMinute = Math.floor(startedAt / 60000) * 60000;
  for (const slot of [...new Set(c.proactive_times || [])].sort()) {
    if (!/^([01]\d|2[0-3]):[0-5]\d$/u.test(slot)) continue;
    const at = Date.parse(`${day}T${slot}:00+08:00`);
    if (at < startupMinute || at > now || now - at >= slotLifetime) continue;
    for (const chat of [...new Set(c.proactive_groups || [])]) {
      if (!proactiveAllowed(c, chat)) continue;
      const scope = `${c.bot_id}:group:${chat}`;
      const activity = store.groupActivity(scope);
      const busy = activity && now - activity.at < (c.proactive_idle_minutes ?? 10) * 60000;
      const id = opaqueKey(`${c.bot_id}:${chat}:${day}:${slot}`);
      if (!store.claimProactive(id, scope, at, activity?.event_id || '', busy ? 'skipped_busy' : 'claimed', now)) continue;
      if (busy) { console.info(JSON.stringify({ event: 'proactive_skipped', chat, slot, reason: 'group_active' })); continue; }
      return { id, chat, scope, at, slot };
    }
  }
}

export function canDeliverProactive(c, store, job, now) {
  const chat = job.scope.split(':')[2];
  return proactiveAllowed(c, chat) && (c.proactive_times || []).includes(beijingTime(job.slot_at).slice(11, 16)) &&
    now >= job.slot_at && now - job.slot_at < slotLifetime &&
    (store.groupActivity(job.scope)?.event_id || '') === job.activity;
}
