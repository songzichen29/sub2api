# Implementation and verification — 2026-11-08

## Administration

- API-key accounts accept 1–20 ordered keys for one upstream and the same model,
  billing, proxy and scheduling identity. No SQL migration is needed: the pool
  uses the existing credentials JSON field on MySQL/PostgreSQL.
- Create/edit use masked rows with add, move, remove and multiline paste.
- Ordinary account DTOs expose only `api_key_count` and credential-presence flags;
  account lists show the count. Existing explicit administrator backup exports
  retain their existing secret-export behavior.
- Leaving the edit input empty preserves the complete stored pool. Entering a
  replacement requires the complete desired list. A single replacement removes
  the old pool. Bulk JSON updates handle this replacement explicitly.
- The model-list preview, account test, standard HTTP and TLS/proxy transports
  carry the same request-local pool. Shared account snapshots are not mutated.

## Retry behavior and boundaries

- Each independent request starts at the primary. Authentication/quota rejection
  advances to the next key; earlier keys are preferred again on the next request.
  There is no background model request or persistent "active key" state.
- All-key exhaustion normally uses a recoverable 15-second cooldown. Explicit
  administrator error-code/temp-unschedulable rules retain their precedence.
- One-key quota snapshots cannot disable an entire pool. Local account billing
  caps, administrator disables and non-quota admission policies remain unchanged.
- A successful response/stream is never replayed; transport errors of ambiguous
  acceptance and non-replayable upload bodies are not replayed either.
- OpenAI Responses/Grok realtime retry rejected WS handshakes only. Established
  sessions and preferred continuation connections keep their original credential.
  New independent pool acquisitions re-evaluate the primary.
- Explicit missing-resource responses can locate resources through later keys.
  Gemini batch upload/create stays on one key; a definitive rejection restarts
  that pair with the next key. Batch lookups/cancel/download/cleanup can locate
  resources owned by fallback keys without storing secrets in task records.
- Ambiguous 5xx on asynchronous create endpoints (video, Seedance, batch, file,
  explicit cache) does not restart a potentially accepted billable job.
- This is transport-level failover. A provider returning HTTP 200 and only later
  emitting an in-band error is not automatically replayed.

## Form parity inventory

New creation controls reuse edit semantics and existing translations:

| Configuration | Scope |
| --- | --- |
| Account scheduling threshold override | OpenAI, Anthropic, Grok |
| 5-hour/7-day pause switches and thresholds | OpenAI |
| Automatic credit reset and thresholds | OpenAI OAuth |
| Subscription plan override | OpenAI OAuth; preserves detected plan if untouched |
| Hosted image tool inherit/enable/disable/block | OpenAI |
| Client-tool cache switch | Grok OAuth |
| Media eligibility auto/enable/disable | Grok OAuth |
| Automatic upstream rate synchronization | API-key accounts; enables probing |

All ordinary and bulk OAuth create calls use one policy serializer. Codex session
and PAT imports pass the same policy credentials/extra. Closing/reopening resets
new controls. Runtime diagnostics, existing-account probes, browser-session login,
and shared OpenCode/Ollama refresh operations remain in edit because they require
a saved account/session and their dedicated backend coordination APIs.

## Checks

- Frontend account/create/edit/model-selector/i18n regression: 183 tests passed.
- `pnpm exec vue-tsc --noEmit`: passed.
- ESLint for the new components, serializers and their tests: passed.
- `pnpm run build`: passed (existing bundle-size/Browserslist warnings).
- `go build -tags embed ./...`: passed after the final source changes.
- Focused Go tests passed in service, repository, admin handler and DTO packages,
  including real HTTP transport, TLS path, batch ownership, pool exhaustion,
  credential redaction, bulk replacement and primary recovery.
- Broad `-tags=unit` compilation is blocked by existing unrelated tests:
  `admin_proxy_credentials_update_test.go` expects pointer fields now typed as
  strings, and old backup tests reference removed `saveRecordWithArchive`.
- No real upstream secrets were used and no billable production requests were
  generated. Live provider/browser acceptance and deployment are not asserted.
- `git diff --check`: passed. Existing unrelated worktrees/directories were kept.
- One AOCI maintenance checkpoint returned `stopped/blocked`, `aligned=false`,
  with 1,245 targets and unrelated historical orphan entries. It issued no
  authoring candidates; `pending_transactions=0`, `recovery_pending=false`,
  `semantic_generated=false`. Per repository policy, no whole-repository cleanup
  or index write was attempted. The index remains explicitly unaligned.

Work branch: `codex/account-key-failover-form-parity-20261108`.
Production branch remains `merge/upstream-main-into-pgsql-mysql-20260426`.
