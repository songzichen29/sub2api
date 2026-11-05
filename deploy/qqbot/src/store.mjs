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
    this.db.exec("UPDATE turns SET status='interrupted' WHERE status='running'");
    // A crash between generation and the QQ receipt must not restore an
    // assistant answer whose delivery is unknown into a model session.
    this.db.exec(`DELETE FROM conversations WHERE scope IN (
      SELECT t.scope FROM turns t LEFT JOIN delivery d ON d.id=t.id
      WHERE t.rowid=(SELECT MAX(latest.rowid) FROM turns latest WHERE latest.scope=t.scope)
      AND (t.status<>'completed' OR (t.reply<>'[跳过]' AND COALESCE(d.status,'')<>'sent'))
    )`);
  }
  turn(id) { return this.db.prepare('SELECT * FROM turns WHERE id=?').get(id); }
  begin(id, scope, deliveryLimits = []) { this.db.prepare("INSERT INTO turns VALUES(?,?,'running',NULL,?,?)").run(id, scope, Date.now(), JSON.stringify(deliveryLimits)); }
  finish(id, status, reply = '') { this.db.prepare('UPDATE turns SET status=?,reply=? WHERE id=?').run(status, reply, id); }
  cacheImages(scope, event, images) {
    for (const [i, image] of images.entries()) this.db.prepare('INSERT OR IGNORE INTO cached_images VALUES(?,?,?,?,?,?)').run(scope,event,i,image.mimeType,Buffer.from(image.data,'base64'),Date.now());
  }
  cachedImages(scope, event, before=Date.now()) {
    const rows = event ? this.db.prepare('SELECT * FROM cached_images WHERE scope=? AND event_id=? ORDER BY ordinal').all(scope,event)
      : this.db.prepare('SELECT * FROM cached_images WHERE scope=? AND event_id=(SELECT event_id FROM cached_images WHERE scope=? AND at<=? ORDER BY at DESC LIMIT 1) ORDER BY ordinal').all(scope,scope,before);
    return rows.map(r => ({ mimeType:r.mime, data:Buffer.from(r.data).toString('base64') }));
  }
  collectMeme(scope, image, tags) {
    const data=Buffer.from(image.data,'base64'), hash=opaqueKey(data);
    this.db.prepare('INSERT INTO memes VALUES(?,?,?,?,?,?) ON CONFLICT(scope,hash) DO UPDATE SET tags=excluded.tags').run(scope,hash,image.mimeType,data,tags,Date.now());
    return hash;
  }
  findMeme(scope, keyword='') {
    let rows=this.db.prepare('SELECT * FROM memes WHERE scope=? AND tags LIKE ? ORDER BY RANDOM() LIMIT 1').all(scope,'%'+keyword.replaceAll('%','').replaceAll('_','')+'%');
    if (!rows.length && /^(?:随机|随便|表情包?)?$/u.test(keyword)) rows=this.db.prepare('SELECT * FROM memes WHERE scope=? ORDER BY RANDOM() LIMIT 1').all(scope);
    return rows[0] ? { mimeType:rows[0].mime,data:Buffer.from(rows[0].data).toString('base64'),description:rows[0].tags } : undefined;
  }
  addMedia(id, image) {
    const ordinal=this.db.prepare('SELECT COUNT(*) AS n FROM outgoing_media WHERE id=?').get(id).n;
    this.db.prepare('INSERT INTO outgoing_media VALUES(?,?,?,?,?)').run(id,ordinal,image.mimeType,Buffer.from(image.data,'base64'),image.description || '表情包');
  }
  media(id) { return this.db.prepare('SELECT * FROM outgoing_media WHERE id=? ORDER BY ordinal').all(id).map(r => ({mimeType:r.mime,data:Buffer.from(r.data).toString('base64'),description:r.label})); }
  session(scope) { return this.db.prepare('SELECT id FROM sessions WHERE scope=?').get(scope)?.id; }
  setSession(scope, id) { this.db.prepare('INSERT INTO sessions VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET id=excluded.id').run(scope, id); }
  clear(scope) {
    this.db.prepare('DELETE FROM sessions WHERE scope=?').run(scope);
    this.db.prepare('DELETE FROM memory WHERE scope=?').run(scope);
    this.db.prepare('INSERT INTO resets VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET after_seq=excluded.after_seq').run(scope, this.latest(scope));
    this.db.prepare('DELETE FROM summaries WHERE scope=?').run(scope);
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
    if (context && turn?.reply && turn.reply !== '[跳过]') this.observe(context.scope, { event_id: 'reply:' + id, sender: bot, name: '机器人', role: 'assistant', text: turn.reply + (this.media(id).length ? '\n[已发送表情包图片]' : '') });
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
      if (!Number.isSafeInteger(item.limit) || item.limit <= 0 || (current?.used || 0) >= item.limit || (item.cooldown && current && now - current.updated < item.cooldown)) throw new Error('rate_limited');
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
    if(!Array.isArray(ids) || ids.length>20 || ids.some(value=>typeof value!=='string' || !/^-?\d{1,24}$/u.test(value))) throw new Error('invalid_receipts');
    for(const value of ids) this.db.prepare('INSERT OR IGNORE INTO delivery_receipts VALUES(?,?)').run(id,value);
  }
}
