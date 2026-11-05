import { scopeKey } from './policy.mjs';

export const bytes = value => Buffer.byteLength(typeof value === 'string' ? value : JSON.stringify(value));
export function conversationScope(input) {
  scopeKey(input);
  return input.group ? `${input.bot}:group:${input.chat}` : `${input.bot}:private:${input.sender}`;
}

// Raw records have no message-count or age limit. Only the model's finite
// input capacity determines when old context is folded into a durable summary.
export async function prepareMemory(store, scope, through, budget, summarize) {
  let summary = store.summary(scope);
  let tail = store.history(scope, summary.through_seq, through);
  const target = Math.floor(budget * 0.72);
  while (bytes(tail) + bytes(summary.text) > target && tail.length > 1) {
    const chunk = [];
    let size = bytes(summary.text) + 256;
    for (const row of tail) {
      if (chunk.length && (size + bytes(row) > target * 0.65 || chunk.length === tail.length - 1)) break;
      if (bytes(row) > target * 0.65) {
        // A single long utterance is summarized in pieces; the full original
        // remains in SQLite, and its cursor advances only after every piece.
        let text = summary.text;
        const pieceSize = Math.max(256, Math.floor(target / 8));
        for (let offset = 0; offset < row.text.length; offset += pieceSize) {
          text = await summarize(text, [{ ...row, text: row.text.slice(offset, offset + pieceSize) }]);
          if (!text?.trim() || bytes(text) > target * 0.6) throw new Error('invalid_memory_summary');
        }
        store.setSummary(scope, row.seq, text);
        summary = store.summary(scope);
        tail = store.history(scope, summary.through_seq, through);
        break;
      }
      chunk.push(row); size += bytes(row);
    }
    if (!chunk.length) continue;
    const text = await summarize(summary.text, chunk);
    if (!text?.trim() || bytes(text) > target * 0.6) throw new Error('invalid_memory_summary');
    store.setSummary(scope, chunk.at(-1).seq, text);
    summary = store.summary(scope);
    tail = store.history(scope, summary.through_seq, through);
  }
  return { summary: summary.text, summary_through: summary.through_seq, messages: tail };
}

export function retrieveMemory(store, scope, query, through, budget) {
  const ignored = new Set(['什么', '一下', '现在', '之前', '我们', '你们', '可以', '那个', '这个', '记得', '聊天']);
  const terms = new Set();
  for (const part of query.match(/[a-zA-Z0-9_]{3,}|[\p{Script=Han}]{2,}/gu) || []) {
    if (/^[\p{Script=Han}]+$/u.test(part)) {
      for (let n = 0; n < part.length - 1; n++) {
        const word = part.slice(n, n + 2);
        if (!ignored.has(word)) terms.add(word);
      }
    } else terms.add(part);
  }
  const words = [...terms].slice(0, 24);
  if (!words.length || through <= store.resetAt(scope)) return [];
  const escape = word => '%' + word.replace(/[\\%_]/gu, '\\$&') + '%';
  const rows = store.db.prepare(`SELECT * FROM history WHERE scope=? AND seq>? AND seq<=? AND role='user' AND (${words.map(() => "text LIKE ? ESCAPE '\\'").join(' OR ')}) ORDER BY seq DESC`).iterate(scope, store.resetAt(scope), through, ...words.map(escape));
  const output = []; let size = 0;
  for (const row of rows) {
    if (size + bytes(row) > budget) continue;
    output.push(row); size += bytes(row);
    if (size > budget * 0.9) break;
  }
  return output.reverse();
}

export function sensitiveText(text) {
  return /(?:[a-z\d._%+-]+@[a-z\d.-]+\.[a-z]{2,}|\bsk-[a-z\d_-]{8,}|(?:密码|口令|验证码|密钥|api[_ -]?key)\s*[:：=]|^(?:\/?(?:绑定|确认|解绑)(?:\s|$)|[a-z\d_-]{24}$))/iu.test(text);
}

export function migrateMemory(store, config) {
  if (store.db.prepare('SELECT name FROM migrations WHERE name=?').get('shared-context-v1')) return;
  for (const row of store.db.prepare('SELECT id,scope,entry FROM memory ORDER BY id').all()) {
    const [bot, kind, chat, sender] = row.scope.split(':');
    if (bot !== config.bot_id || (kind === 'group' ? !config.groups.includes(chat) : !config.private_users.includes(chat))) continue;
    const value = JSON.parse(row.entry);
    if (typeof value.user !== 'string' || sensitiveText(value.user)) continue;
    const scope = conversationScope({ bot, chat, sender: sender || chat, group: kind === 'group', message_id: 'migration' });
    store.observe(scope, { event_id: 'legacy-user:' + row.id, sender: value.sender || sender || chat, text: value.user, at: value.at });
    const delivered = store.db.prepare("SELECT 1 FROM turns t JOIN delivery d ON d.id=t.id WHERE t.scope=? AND t.reply=? AND d.status='sent'").get(row.scope, value.assistant);
    if (delivered && value.assistant && value.assistant !== '[跳过]') store.observe(scope, { event_id: 'legacy-reply:' + row.id, sender: bot, name: '机器人', role: 'assistant', text: value.assistant, at: value.at });
  }
  store.db.prepare('INSERT INTO migrations VALUES(?)').run('shared-context-v1');
}
