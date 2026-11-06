// A bounded FIFO per conversation, with independent conversations allowed to run
// concurrently. Deadlines include queue time; abandoned HTTP work never starts.
export class ConversationScheduler {
  constructor({ concurrency = 4, maxPending = 64 } = {}) {
    this.concurrency = concurrency;
    this.maxPending = maxPending;
    this.activeScopes = new Set();
    this.pending = [];
    this.closed = false;
    this.running = new Set();
  }
  get active() { return this.activeScopes.size; }
  get waiting() { return this.pending.length; }
  has(scope) { return this.activeScopes.has(scope) || this.pending.some(job => job.scope === scope); }
  run(scope, work, signal) {
    if (this.closed) return Promise.reject(new Error('runtime_stopping'));
    if (this.pending.length >= this.maxPending) return Promise.reject(new Error('queue_full'));
    if (signal?.aborted) return Promise.reject(new Error('turn_expired'));
    return new Promise((resolve, reject) => {
      const job = { scope, work, signal, resolve, reject };
      job.cancel = () => {
        const index = this.pending.indexOf(job);
        if (index >= 0) {
          this.pending.splice(index, 1);
          reject(new Error('turn_expired'));
        }
      };
      signal?.addEventListener('abort', job.cancel, { once: true });
      this.pending.push(job);
      this.drain();
    });
  }
  drain() {
    while (!this.closed && this.active < this.concurrency) {
      const index = this.pending.findIndex(job => !this.activeScopes.has(job.scope));
      if (index < 0) return;
      const job = this.pending.splice(index, 1)[0];
      job.signal?.removeEventListener('abort', job.cancel);
      if (job.signal?.aborted) { job.reject(new Error('turn_expired')); continue; }
      this.activeScopes.add(job.scope);
      const execution = Promise.resolve().then(job.work).then(job.resolve, job.reject).finally(() => {
        this.activeScopes.delete(job.scope);
        this.running.delete(execution);
        this.drain();
      });
      this.running.add(execution);
    }
  }
  async close() {
    this.closed = true;
    for (const job of this.pending.splice(0)) {
      job.signal?.removeEventListener('abort', job.cancel);
      job.reject(new Error('runtime_stopping'));
    }
    await Promise.allSettled([...this.running]);
  }
}
