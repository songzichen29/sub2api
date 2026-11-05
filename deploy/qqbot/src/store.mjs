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
    this.db.exec("UPDATE turns SET status='interrupted' WHERE status='running'");
  }
  turn(id) { return this.db.prepare('SELECT * FROM turns WHERE id=?').get(id); }
  begin(id, scope, deliveryLimits = []) { this.db.prepare("INSERT INTO turns VALUES(?,?,'running',NULL,?,?)").run(id, scope, Date.now(), JSON.stringify(deliveryLimits)); }
  finish(id, status, reply = '') { this.db.prepare('UPDATE turns SET status=?,reply=? WHERE id=?').run(status, reply, id); }
  session(scope) { return this.db.prepare('SELECT id FROM sessions WHERE scope=?').get(scope)?.id; }
  setSession(scope, id) { this.db.prepare('INSERT INTO sessions VALUES(?,?) ON CONFLICT(scope) DO UPDATE SET id=excluded.id').run(scope, id); }
  clear(scope) {
    this.db.prepare('DELETE FROM sessions WHERE scope=?').run(scope);
    this.db.prepare('DELETE FROM memory WHERE scope=?').run(scope);
  }
  recent(scope) { return this.db.prepare('SELECT entry FROM memory WHERE scope=? ORDER BY id DESC LIMIT 8').all(scope).reverse().map(r => JSON.parse(r.entry)); }
  remember(scope, value) {
    const entry = JSON.stringify(value);
    this.db.prepare('INSERT INTO memory(scope,entry) VALUES(?,?)').run(scope, entry);
    // Every scope gets its own file; private text never enters another group's raw log.
    appendFileSync(join(this.rawDirectory, opaqueKey(scope) + '.jsonl'), entry + '\n', { mode: 0o600 });
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
  finishDelivery(id, status) { this.db.prepare('UPDATE delivery SET status=?,updated=? WHERE id=?').run(status, Date.now(), id); }
}
