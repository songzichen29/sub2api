export function quotaLimits(input, config, now = Date.now()) {
  const day = new Date(now + 8 * 3600000).toISOString().slice(0, 10);
  const hour = Math.floor(now / 3600000);
  const policy = (config.group_policies || []).find(p => p.group_id === input.chat) || {};
  const calls = [
    { key: `chat:all:${day}`, limit: config.total_daily_chat_limit },
    { key: `chat:member:${input.sender}:${day}`, limit: config.daily_chat_limit, cooldown: (config.cooldown_seconds ?? 5) * 1000 },
    { key: `chat:scope:${input.chat}:${input.sender}:${hour}`, limit: config.user_hourly_limit ?? 60 }
  ];
  if (input.group) calls.push({ key: `chat:group:${input.chat}:${day}`, limit: policy.disable_chat ? 0 : (policy.chat_daily_limit ?? config.total_daily_chat_limit) });
  const delivery = [];
  // A scheduled group topic has no member principal and spends no user's quota.
  if (input.proactive) calls.splice(1, 2);
  if (input.ambient) {
    calls.push({ key: `ambient:group:${input.chat}:${day}`, limit: config.ambient_daily_calls });
    if (input.images?.length) calls.push({ key: `ambient_media:group:${input.chat}:${day}`, limit: config.ambient_media_daily_calls });
    delivery.push(
      { key: `ambient_reply:group:${input.chat}:${day}`, limit: config.ambient_daily_replies, cooldown: (config.cooldown_seconds ?? 5) * 1000 },
      { key: `ambient_reply:group:${input.chat}:${hour}`, limit: config.ambient_hourly_replies }
    );
  }
  return { calls, delivery };
}
