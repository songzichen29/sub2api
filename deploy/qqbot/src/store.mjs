import { DatabaseSync } from 'node:sqlite';
import { mkdirSync, appendFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { opaqueKey } from './policy.mjs';

export class Store {
  constructor(path, rawDirectory) {
    mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    mkdirSync(rawDirectory, { recursive: true, mode: 0o700 });
    this.rawDirectory = rawDirectory;
    this.db = new DatabaseSync(path);
    this.db.exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
      CREATE TABLE IF NOT EXISTS sessions(scope TEXT PRIMARY KEY,id TEXT NOT NULL);
      CREATE TABLE IF NOT EXISTS turns(id TEXT PRIMARY KEY,scope TEXT NOT NULL,status TEXT NOT NULL,reply TEXT,created INTEGER NOT NULL,delivery_limits TEXT NOT NULL DEFAULT '[]');
      CREATE TABLE IF NOT EXISTS memory(id INTEGER PRIMARY KEY,scope TEXT NOT NULL,entry TEXT NOT NULL);
      CREATE INDEX IF NOT EXISTS memory_scope ON memory(scope,id);
      CREATE TABLE IF NOT EXISTS delivery(id TEXT PRIMARY KEY,status TEXT NOT NULL,updated INTEGER NOT NULL);
      CREATE TABLE IF NOT EXISTS meters(key TEXT PRIMARY KEY,used INTEGER NOT NULL,updated INTEGER NOT NULL);`);
	this.db.exec(`
      CREATE TABLE IF NOT EXISTS history(seq INTEGER PRIMARY KEY,scope TEXT NOT NULL,event_id TEXT NOT NULL,sender TEXT NOT NULL,name TEXT NOT NULL,role TEXT NOT NULL,text TEXT NOT NULL,at TEXT NOT NULL,UNIQUE(scope,event_id));
      CREATE INDEX IF NOT EXISTS history_scope ON history(scope,seq);
      CREATE TABLE IF NOT EXISTS summaries(scope TEXT PRIMARY KEY,through_seq INTEGER NOT NULL,text TEXT NOT NULL);
      CREATE TABLE IF NOT EXISTS conversations(scope TEXT PRIMARY KEY,session_id TEXT NOT NULL,cursor INTEGER NOT NULL,bytes INTEGER NOT NULL);
      CREATE TABLE IF NOT EXISTS resets(scope TEXT PRIMARY KEY,after_seq INTEGER NOT NULL);
      CREATE TABLE IF NOT EXISTS turn_context(id TEXT PRIMARY KEY,scope TEXT NOT NULL,event_id TEXT NOT NULL,sender TEXT NOT NULL);
      CREATE TABLE IF NOT EXISTS migrations(name TEXT PRIMARY KEY);
      CREATE TABLE IF NOT EXISTS runtime_settings(key TEXT PRIMARY KEY,value TEXT NOT NULL);`);
	this.db.exec(`
      CREATE TABLE IF NOT EXISTS cached_images(scope TEXT NOT NULL,event_id TEXT NOT NULL,ordinal INTEGER NOT NULL,mime TEXT NOT NULL,data BLOB NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(scope,event_id,ordinal));
      CREATE TABLE IF NOT EXISTS memes(scope TEXT NOT NULL,hash TEXT NOT NULL,mime TEXT NOT NULL,data BLOB NOT NULL,tags TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(scope,hash));
      CREATE TABLE IF NOT EXISTS outgoing_media(id TEXT NOT NULL,ordinal INTEGER NOT NULL,mime TEXT NOT NULL,data BLOB NOT NULL,label TEXT NOT NULL,PRIMARY KEY(id,ordinal));`);
    this.db.exec('CREATE TABLE IF NOT EXISTS delivery_receipts(id TEXT NOT NULL,message_id TEXT NOT NULL,PRIMARY KEY(id,message_id));');
    this.db.exec(`CREATE TABLE IF NOT EXISTS group_activity(scope TEXT PRIMARY KEY,event_id TEXT NOT NULL,at INTEGER NOT NULL);
      CREATE TABLE IF NOT EXISTS proactive_slots(id TEXT PRIMARY KEY,scope TEXT NOT NULL,slot_at INTEGER NOT NULL,activity TEXT NOT NULL,status TEXT NOT NULL,turn_id TEXT,updated INTEGER NOT NULL);
      CREATE UNIQUE INDEX IF NOT EXISTS proactive_turn ON proactive_slots(turn_id);`);
    this.db.exec(`CREATE TABLE IF NOT EXISTS pending_conversations(id TEXT PRIMARY KEY,scope TEXT NOT NULL,session_id TEXT NOT NULL,cursor INTEGER NOT NULL,bytes INTEGER NOT NULL);
      CREATE TABLE IF NOT EXISTS diagnostics(id TEXT PRIMARY KEY,scope TEXT NOT NULL,sender TEXT NOT NULL,message_id TEXT NOT NULL,status TEXT NOT NULL,reason TEXT NOT NULL,created INTEGER NOT NULL,updated INTEGER NOT NULL,deadline INTEGER NOT NULL);
      CREATE INDEX IF NOT EXISTS diagnostics_created ON diagnostics(created);
      CREATE TABLE IF NOT EXISTS meme_search_cache(scope TEXT NOT NULL,keyword TEXT NOT NULL,mime TEXT NOT NULL,data BLOB NOT NULL,description TEXT NOT NULL,at INTEGER NOT NULL,PRIMARY KEY(scope,keyword));`);
    // Additive, idempotent SQLite migration: preserve previously saved assets.
    if (!this.db.prepare('PRAGMA table_info(cached_images)').all().some(c => c.name === 'sender')) this.db.exec("ALTER TABLE cached_images ADD COLUMN sender TEXT NOT NULL DEFAULT ''");
    if (!this.db.prepare('PRAGMA table_info(memes)').all().some(c => c.name === 'manual_tags')) {
      this.db.exec("ALTER TABLE memes ADD COLUMN manual_tags TEXT NOT NULL DEFAULT ''; UPDATE memes SET manual_tags=tags");
    }
    this.db.exec("UPDATE turns SET status='interrupted' WHERE status='running'");
    this.db.exec("UPDATE diagnostics SET status='interrupted',reason='runtime_restarted' WHERE status IN ('queued','running'); DELETE FROM pending_conversations");
    this.db.exec("UPDATE delivery SET status='unconfirmed' WHERE status='sending'; UPDATE diagnostics SET status='unconfirmed',reason='runtime_restarted' WHERE status='sending'");
    // A crash between generation and the QQ receipt must not restore an
    // assistant answer whose delivery is unknown into a model session.
    this.db.exec(`DELETE FROM conversations WHERE scope IN (
      SELECT t.scope FROM turns t LEFT JOIN delivery d ON d.id=t.id
      WHERE t.rowid=(SELECT MAX(latest.rowid) FROM turns latest WHERE latest.scope=t.scope)
      AND (t.status<>'completed' OR (t.reply<>'[跳过]' AND COALESCE(d.status,'')<>'sent'))
    )`);
  }
  turn(id) { return this.db.prepare('SELECT * FROM turns WHERE id=?').get(id); }
  noteActivity(scope, event, at = Date.now()) {
    this.db.prepare('INSERT INTO group_activity VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET event_id=excluded.event_id,at=excluded.at WHERE group_activity.event_id<>excluded.event_id').run(scope, event, at);
  }
  groupActivity(scope) {
    const activity = this.db.prepare('SELECT event_id,at FROM group_activity WHERE scope=?').get(scope);
    if (activity) return activity;
    const prior = this.db.prepare("SELECT event_id,at FROM history WHERE scope=? AND role='user' ORDER BY seq DESC LIMIT 1").get(scope);
    const at = Date.parse(prior?.at);
    return Number.isFinite(at) ? { event_id: prior.event_id.replace(/^msg:/u, ''), at } : undefined;
  }
  claimProactive(id, scope, at, activity, status, now) {
    return this.db.prepare('INSERT OR IGNORE INTO proactive_slots VALUES(?,?,?,?,?,NULL,?)').run(id, scope, at, activity, status, now).changes > 0;
  }
  finishProactive(id, status, turn = null) { this.db.prepare('UPDATE proactive_slots SET status=?,turn_id=COALESCE(?,turn_id),updated=? WHERE id=?').run(status, turn, Date.now(), id); }
  proactiveTurn(id) { return this.db.prepare('SELECT * FROM proactive_slots WHERE turn_id=?').get(id); }
  begin(id, scope, deliveryLimits = []) { this.db.prepare("INSERT INTO turns VALUES(?,?,'running',NULL,?,?)").run(id, scope, Date.now(), JSON.stringify(deliveryLimits)); }
  finish(id, status, reply = '') { this.db.prepare('UPDATE turns SET status=?,reply=? WHERE id=?').run(status, reply, id); }
  cacheImages(scope, event, images, sender = '') {
    this.pruneMedia();
    for (const [i, image] of images.entries()) this.db.prepare('INSERT OR IGNORE INTO cached_images(scope,event_id,ordinal,mime,data,at,sender) VALUES(?,?,?,?,?,?,?)').run(scope,event,(image.source_index || i + 1)-1,image.mimeType,Buffer.from(image.data,'base64'),Date.now(),sender);
    this.pruneMedia();
  }
  cachedImages(scope, event, before=Date.now()) {
    const rows = event ? this.db.prepare('SELECT * FROM cached_images WHERE scope=? AND event_id=? ORDER BY ordinal').all(scope,event)
      : this.db.prepare('SELECT * FROM cached_images WHERE scope=? AND event_id=(SELECT event_id FROM cached_images WHERE scope=? AND at<=? ORDER BY at DESC LIMIT 1) ORDER BY ordinal').all(scope,scope,before);
    return rows.map(r => ({ mimeType:r.mime, data:Buffer.from(r.data).toString('base64'), source_index:r.ordinal+1 }));
  }
  recentSenderImages(scope, sender, before = Date.now()) {
    const row = this.db.prepare('SELECT event_id FROM cached_images WHERE scope=? AND sender=? AND at<=? AND at>=? ORDER BY at DESC,rowid DESC LIMIT 1').get(scope,sender,before,before-10*60000);
    return row ? this.cachedImages(scope,row.event_id) : [];
  }
  collectMeme(scope, image, tags, { automatic = false } = {}) {
    const data=Buffer.from(image.data,'base64'), hash=opaqueKey(data);
    const existing=this.db.prepare('SELECT hash FROM memes WHERE scope=? AND hash=?').get(scope,hash);
    if (!existing && this.db.prepare('SELECT COALESCE(SUM(length(data)),0) AS size FROM memes').get().size + data.length > 1024*1024*1024) {
      if (automatic) return undefined;
      throw new Error('media_storage_full');
    }
    this.db.prepare(`INSERT INTO memes(scope,hash,mime,data,tags,at,manual_tags) VALUES(?,?,?,?,?,?,?) ON CONFLICT(scope,hash) DO UPDATE SET
      tags=CASE WHEN excluded.manual_tags='' AND memes.manual_tags<>'' THEN memes.tags ELSE excluded.tags END,
      manual_tags=CASE WHEN excluded.manual_tags='' THEN memes.manual_tags ELSE excluded.manual_tags END`).run(scope,hash,image.mimeType,data,tags,Date.now(),automatic?'':tags);
    return hash;
  }
  findMeme(scope, keyword='') {
    let rows=this.db.prepare('SELECT * FROM memes WHERE scope=? AND tags LIKE ? ORDER BY RANDOM() LIMIT 1').all(scope,'%'+keyword.replaceAll('%','').replaceAll('_','')+'%');
    if (!rows.length && /^(?:随机|随便|表情包?)?$/u.test(keyword)) rows=this.db.prepare('SELECT * FROM memes WHERE scope=? ORDER BY RANDOM() LIMIT 1').all(scope);
    return rows[0] ? { mimeType:rows[0].mime,data:Buffer.from(rows[0].data).toString('base64'),description:rows[0].tags } : undefined;
  }
  cachedMeme(scope, keyword) {
    const row=this.db.prepare('SELECT * FROM meme_search_cache WHERE scope=? AND keyword=? AND at>?').get(scope,keyword,Date.now()-86400000);
    return row ? {mimeType:row.mime,data:Buffer.from(row.data).toString('base64'),description:row.description} : undefined;
  }
  cacheMeme(scope, keyword, image) {
    this.db.prepare('INSERT INTO meme_search_cache VALUES(?,?,?,?,?,?) ON CONFLICT(scope,keyword) DO UPDATE SET mime=excluded.mime,data=excluded.data,description=excluded.description,at=excluded.at').run(scope,keyword,image.mimeType,Buffer.from(image.data,'base64'),image.description || keyword,Date.now());
    this.pruneMedia();
  }
  pruneMedia(now = Date.now(), maxBytes = 128*1024*1024) {
    this.db.prepare('DELETE FROM cached_images WHERE at<?').run(now-86400000);
    this.db.prepare('DELETE FROM meme_search_cache WHERE at<?').run(now-86400000);
    // Keep in-flight/ambiguous sends for inspection; clean only old terminal media.
    this.db.prepare("DELETE FROM outgoing_media WHERE id IN (SELECT t.id FROM turns t LEFT JOIN delivery d ON d.id=t.id WHERE t.created<? AND COALESCE(d.status,'')<>'sending' AND t.status<>'running')").run(now-7*86400000);
    for (const table of ['cached_images','meme_search_cache']) {
      this.db.exec(`DELETE FROM ${table} WHERE rowid IN (SELECT rowid FROM (SELECT rowid,SUM(length(data)) OVER (ORDER BY at DESC,rowid DESC) AS total FROM ${table}) WHERE total>${maxBytes})`);
    }
    this.db.prepare('DELETE FROM diagnostics WHERE updated<?').run(now-30*86400000);
  }
  addMedia(id, image) {
    this.pruneMedia();
    const data=Buffer.from(image.data,'base64'), capacity=256*1024*1024;
    let size=this.db.prepare('SELECT COALESCE(SUM(length(data)),0) AS n FROM outgoing_media').get().n;
    if(size+data.length>capacity) {
      const terminal=this.db.prepare("SELECT m.rowid,length(m.data) AS n FROM outgoing_media m JOIN turns t ON t.id=m.id LEFT JOIN delivery d ON d.id=m.id WHERE t.status IN ('failed','interrupted') OR d.status IN ('sent','unconfirmed','partial') ORDER BY t.created,m.rowid").all();
      for(const row of terminal) {if(size+data.length<=capacity)break;this.db.prepare('DELETE FROM outgoing_media WHERE rowid=?').run(row.rowid);size-=row.n;}
    }
    if(size+data.length>capacity)throw new Error('media_storage_full');
    const ordinal=this.db.prepare('SELECT COUNT(*) AS n FROM outgoing_media WHERE id=?').get(id).n;
    this.db.prepare('INSERT INTO outgoing_media VALUES(?,?,?,?,?)').run(id,ordinal,image.mimeType,data,image.description || '表情包');
  }
  media(id) { return this.db.prepare('SELECT * FROM outgoing_media WHERE id=? ORDER BY ordinal').all(id).map(r => ({mimeType:r.mime,data:Buffer.from(r.data).toString('base64'),description:r.label})); }
  session(scope) { return this.db.prepare('SELECT id FROM sessions WHERE scope=?').get(scope)?.id; }
  setSession(scope, id) { this.db.prepare('INSERT INTO sessions VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET id=excluded.id').run(scope, id); }
  clear(scope) {
    this.db.prepare('DELETE FROM sessions WHERE scope=?').run(scope);
    this.db.prepare('DELETE FROM memory WHERE scope=?').run(scope);
    this.db.prepare('INSERT INTO resets VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET after_seq=excluded.after_seq').run(scope, this.latest(scope));
    this.db.prepare('DELETE FROM summaries WHERE scope=?').run(scope);
    this.db.prepare('DELETE FROM pending_conversations WHERE scope=?').run(scope);
    this.db.prepare('DELETE FROM cached_images WHERE scope=?').run(scope);
    this.dropConversation(scope);
  }
  recent(scope) { return this.db.prepare('SELECT entry FROM memory WHERE scope=? ORDER BY id').all(scope).map(r => JSON.parse(r.entry)); }
  remember(scope, value) {
    const entry = JSON.stringify(value);
    this.db.prepare('INSERT INTO memory(scope,entry) VALUES(?,?)').run(scope, entry);
    // Every scope gets its own file; private text never enters another group's raw log.
    appendFileSync(join(this.rawDirectory, opaqueKey(scope) + '.jsonl'), entry + '\n', { mode: 0o600 });
  }
  observe(scope, { event_id, sender, name = '', role = 'user', text, at = new Date().toISOString() }) {
    const result = this.db.prepare('INSERT INTO history(scope,event_id,sender,name,role,text,at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(scope,event_id) DO NOTHING').run(scope, event_id, sender, name, role, text, at);
    if (result.changes) appendFileSync(join(this.rawDirectory, opaqueKey(scope) + '.jsonl'), JSON.stringify({ event_id, sender, name, role, text, at }) + '\n', { mode: 0o600 });
    return this.db.prepare('SELECT seq FROM history WHERE scope=? AND event_id=?').get(scope, event_id).seq;
  }
  resetAt(scope) { return this.db.prepare('SELECT after_seq FROM resets WHERE scope=?').get(scope)?.after_seq || 0; }
  latest(scope) { return this.db.prepare('SELECT MAX(seq) AS seq FROM history WHERE scope=?').get(scope)?.seq || 0; }
  history(scope, after = 0, through = Number.MAX_SAFE_INTEGER) {
    return this.db.prepare('SELECT * FROM history WHERE scope=? AND seq>? AND seq<=? ORDER BY seq').all(scope, Math.max(after, this.resetAt(scope)), through);
  }
  summary(scope) { return this.db.prepare('SELECT through_seq,text FROM summaries WHERE scope=?').get(scope) || { through_seq: this.resetAt(scope), text: '' }; }
  setSummary(scope, through, text) { this.db.prepare('INSERT INTO summaries VALUES(?,?,?) ON CONFLICT(scope) DO UPDATE SET through_seq=excluded.through_seq,text=excluded.text').run(scope, through, text); }
  conversation(scope) { return this.db.prepare('SELECT session_id,cursor,bytes FROM conversations WHERE scope=?').get(scope); }
  setConversation(scope, id, cursor, bytes) { this.db.prepare('INSERT INTO conversations VALUES(?,?,?,?) ON CONFLICT(scope) DO UPDATE SET session_id=excluded.session_id,cursor=excluded.cursor,bytes=excluded.bytes').run(scope, id, cursor, bytes); }
  stageConversation(id, scope, session, cursor, bytes) {
    this.dropConversation(scope);
    this.db.prepare('DELETE FROM pending_conversations WHERE scope=?').run(scope);
    this.db.prepare('INSERT INTO pending_conversations VALUES(?,?,?,?,?)').run(id,scope,session,cursor,bytes);
  }
  confirmConversation(id) {
    const pending=this.db.prepare('SELECT * FROM pending_conversations WHERE id=?').get(id);
    if (!pending) return;
    const latest=this.db.prepare('SELECT id FROM turns WHERE scope=? ORDER BY rowid DESC LIMIT 1').get(pending.scope);
    if (latest?.id===id && this.delivery(id)?.status==='sent') this.setConversation(pending.scope,pending.session_id,pending.cursor,pending.bytes);
    this.db.prepare('DELETE FROM pending_conversations WHERE id=?').run(id);
  }
  discardPending(id) { this.db.prepare('DELETE FROM pending_conversations WHERE id=?').run(id); }
  ensureScopeRuntime(scope, signature) {
    const key='scope:'+scope;
    if (this.db.prepare('SELECT value FROM runtime_settings WHERE key=?').get(key)?.value===signature) return;
    this.dropConversation(scope);
    this.db.prepare('DELETE FROM pending_conversations WHERE scope=?').run(scope);
    this.db.prepare('INSERT INTO runtime_settings VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value').run(key,signature);
  }
  dropConversation(scope) { this.db.prepare('DELETE FROM conversations WHERE scope=?').run(scope); this.db.prepare('DELETE FROM sessions WHERE scope=?').run(scope); }
  ensureRuntime(signature) {
    const prior = this.db.prepare("SELECT value FROM runtime_settings WHERE key='identity'").get()?.value;
    if (prior === signature) return;
    this.db.exec('BEGIN IMMEDIATE');
    try {
      this.db.exec('DELETE FROM conversations; DELETE FROM sessions;');
      this.db.prepare("INSERT INTO runtime_settings VALUES('identity',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value").run(signature);
      this.db.exec('COMMIT');
    } catch (error) { this.db.exec('ROLLBACK'); throw error; }
  }
  setTurnContext(id, scope, event, sender) { this.db.prepare('INSERT INTO turn_context VALUES(?,?,?,?)').run(id, scope, event, sender); }
  commitReply(id, bot) {
    if (this.db.prepare('SELECT status FROM delivery WHERE id=?').get(id)?.status !== 'sent') return;
    const context = this.db.prepare('SELECT * FROM turn_context WHERE id=?').get(id), turn = this.turn(id);
    if (context && turn && turn.reply !== '[跳过]' && (turn.reply || this.media(id).length)) this.observe(context.scope, { event_id: 'reply:' + id, sender: bot, name: '机器人', role: 'assistant', text: turn.reply + (this.media(id).length ? '\n[已发送表情包图片]' : '') });
  }
  diagnostic(id) { return this.db.prepare('SELECT * FROM diagnostics WHERE id=?').get(id); }
  recordTurn(id, scope, sender, message, deadline) {
    const now=Date.now();
    this.db.prepare("INSERT OR IGNORE INTO diagnostics VALUES(?,?,?,?,'queued','',?,?,?)").run(id,scope,sender,message,now,now,deadline);
  }
  recordStatus(id, status, reason = '') {
    this.db.prepare('UPDATE diagnostics SET status=?,reason=?,updated=? WHERE id=?').run(status,reason.slice(0,100),Date.now(),id);
  }
  runtimeStats(now = Date.now()) {
    // Allow a short receipt-report grace period, then classify abandoned sends
    // as uncertain so their binary assets can eventually expire as well.
    this.db.prepare("UPDATE delivery SET status='unconfirmed',updated=? WHERE status='sending' AND id IN (SELECT id FROM diagnostics WHERE deadline<?)").run(now,now-60000);
    this.pruneMedia(now);
    this.db.prepare("UPDATE diagnostics SET status='expired',reason='delivery_not_claimed',updated=? WHERE status='generated' AND deadline<?").run(now,now);
    this.db.prepare("UPDATE diagnostics SET status='unconfirmed',reason='qq_delivery_unconfirmed',updated=? WHERE status='sending' AND deadline<?").run(now,now);
    this.db.exec("DELETE FROM pending_conversations WHERE id IN (SELECT id FROM diagnostics WHERE status IN ('expired','cancelled','rejected','failed','interrupted','unconfirmed','partial'))");
    const day=new Date(now+8*3600000).toISOString().slice(0,10), start=Date.parse(day+'T00:00:00+08:00');
    const count=(statuses)=>this.db.prepare(`SELECT COUNT(*) AS n FROM diagnostics WHERE created>=? AND status IN (${statuses.map(()=>'?').join(',')})`).get(start,...statuses).n;
    return {day,daily_calls:this.db.prepare('SELECT used FROM meters WHERE key=?').get('chat:all:'+day)?.used || 0,
      daily_sent:count(['sent']),daily_failed:count(['failed','interrupted','expired','rejected','cancelled']),unconfirmed:count(['sending','unconfirmed','partial']),
      recent:this.db.prepare('SELECT id,scope,sender,message_id,status,reason,created,updated FROM diagnostics ORDER BY updated DESC,rowid DESC LIMIT 20').all()};
  }
  delivery(id) { return this.db.prepare('SELECT * FROM delivery WHERE id=?').get(id); }
  referenceInScope(scope, message) {
    return Boolean(this.db.prepare('SELECT 1 FROM delivery_receipts r JOIN turns t ON t.id=r.id WHERE t.scope=? AND r.message_id=?').get(scope,message));
  }
  stats() {
    const count = table => this.db.prepare('SELECT COUNT(*) AS n FROM ' + table).get().n;
    return { sessions: count('conversations'), messages: count('history'), summaries: count('summaries') };
  }
  seedMeters(rows) {
    const put = this.db.prepare('INSERT INTO meters VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET used=MAX(used,excluded.used)');
    for (const row of rows) put.run(row.key, row.used, 0);
  }
  _reserve(limits, now) {
    for (const item of limits) {
      const current = this.db.prepare('SELECT used,updated FROM meters WHERE key=?').get(item.key);
      if (!Number.isSafeInteger(item.limit) || item.limit <= 0 || (current?.used || 0) >= item.limit) throw new Error('rate_limited');
      if (item.cooldown && current && now-current.updated<item.cooldown) throw new Error('cooldown');
    }
    const put = this.db.prepare('INSERT INTO meters VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET used=used+1,updated=excluded.updated');
    for (const item of limits) put.run(item.key, now);
  }
  reserve(limits, now = Date.now()) {
    this.db.exec('BEGIN IMMEDIATE');
    try { this._reserve(limits, now); this.db.exec('COMMIT'); }
    catch (error) { this.db.exec('ROLLBACK'); throw error; }
  }
  claimDelivery(id) {
    this.db.exec('BEGIN IMMEDIATE');
    try {
      if (this.db.prepare('SELECT id FROM delivery WHERE id=?').get(id)) { this.db.exec('COMMIT'); return false; }
      const turn = this.turn(id);
      if (!turn || turn.status !== 'completed') throw new Error('turn_not_completed');
      this._reserve(JSON.parse(turn.delivery_limits), Date.now());
      this.db.prepare("INSERT INTO delivery VALUES(?,'sending',?)").run(id, Date.now());
      this.db.exec('COMMIT'); return true;
    } catch (error) { this.db.exec('ROLLBACK'); throw error; }
  }
  finishDelivery(id, status) {
    const prior = this.db.prepare('SELECT status FROM delivery WHERE id=?').get(id)?.status;
    if (!prior || (prior !== 'sending' && prior !== status)) throw new Error('invalid_delivery_transition');
    this.db.prepare('UPDATE delivery SET status=?,updated=? WHERE id=?').run(status, Date.now(), id);
  }
  recordReceipts(id, ids=[]) {
    if(!Array.isArray(ids) || ids.length>128 || ids.some(value=>typeof value!=='string' || !/^-?\d{1,24}$/u.test(value) || value==='0')) throw new Error('invalid_receipts');
    for(const value of ids) this.db.prepare('INSERT OR IGNORE INTO delivery_receipts VALUES(?,?)').run(id,value);
  }
}
