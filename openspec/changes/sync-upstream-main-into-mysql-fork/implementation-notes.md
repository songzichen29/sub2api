# Implementation Notes: sync-upstream-main-into-mysql-fork

## Baseline

- Recorded at: 2026-06-17 00:26:52 +08:00
- Current branch before sync: `merge/upstream-main-into-pgsql-mysql-20260426`
- Current HEAD before sync: `b6e0d0f7e6f2abcb392370684118213e585aeb86`
- Upstream head: `4a5665da5b2c6b83c4597844ea6e573746c821b1`
- Merge base: `e34ad2b19424844cbd1bcffb599e5b0002ad3c50`
- Divergence (`HEAD...upstream/main`): `338 80` (`left=current branch`, `right=upstream/main`)
- Backup branch: `backup/pre-sync-upstream-main-into-mysql-fork-20260617-002606`
- Isolated integration branch: `codex/sync-upstream-main-into-mysql-fork`

## Merge Preview and Conflict Evidence

- Preview command: `git merge-tree HEAD upstream/main`
- Full preview: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/merge-tree-preview.txt`
- Conflict summary: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/merge-conflicts.txt`
- Upstream diff summary: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/upstream-diff-summary.txt`

## Accepted Upstream Feature Groups

- OpenAI / gateway: accepted upstream cyber policy/session-block behavior, thinking protocol filtering, responses fallback fixes, `response.failed` usage/failover handling, image failover, websocket probe/quota behavior, usage-recording tests, and related API compatibility tests.
- Content moderation / risk control: accepted model-filter support, cyber-policy exclusion handling, email/log side effects, admin handler updates, and focused handler/service tests.
- Scheduler / async: accepted scheduler outbox deduplication, cleanup, race fixes, snapshot cleanup tests, and lock behavior while adapting persistence to MySQL.
- Settings / auth / billing / quota: accepted OAuth/default subscription/login agreement/public settings/default platform quota/OpenAI quota reset/affiliate/standalone import updates plus billing fallback and token-refresh candidate changes.
- Channel monitor / operations: accepted channel monitor jitter and template changes plus ops/user-error logging additions.
- Frontend admin/user UI: accepted admin account, risk control, settings, usage, i18n, API type, OpenAI quota reset, and SettingsView test updates.
- Deploy/package metadata: accepted Dockerfile/package/version updates while keeping MySQL as the deployment target.

## MySQL Adaptations

- Added active MySQL migrations:
  - `backend/migrations/mysql/031_channel_monitor_jitter.sql`
  - `backend/migrations/mysql/032_account_autopause_expiry_index.sql`
  - `backend/migrations/mysql/033_scheduler_outbox_dedup_key.sql`
  - `backend/migrations/mysql/034_scheduler_outbox_pending_dedup_key_index.sql`
- Preserved upstream root PostgreSQL migration files `backend/migrations/151-153*.sql` as historical/reference artifacts only. Runtime migration execution uses `migrations.MySQLFS` through `backend/internal/repository/ent.go`.
- Adapted repository SQL to MySQL placeholders/functions/locking, including `JSON_EXTRACT` / `JSON_UNQUOTE` / `JSON_CONTAINS`, `SELECT ... FOR UPDATE`, `GET_LOCK` / `RELEASE_LOCK`, `ON DUPLICATE KEY UPDATE`, and MySQL-compatible cleanup deletes.
- Kept MySQL runtime/config/deploy paths active: MySQL driver/config, MySQL backup naming, Docker Compose `mysql:8.0`, `mysqladmin ping`, port `3306`, and `mysql_data` volumes.
- Updated deploy docs and migration README examples away from PostgreSQL commands (`psql`, `pg_dump`, `jsonb_set`) toward MySQL equivalents.

## Regeneration and Formatting

- Regenerated Ent outputs with `go generate ./ent` after resolving `backend/ent/schema` changes.
- Regenerated Wire outputs with `go generate ./cmd/server` after resolving provider/source files.
- Ran `gofmt` over changed backend Go files.
- Ran focused frontend ESLint on changed Settings/RiskControl API/view/test files.

## PostgreSQL Regression Audit

- Final audit file: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/postgres-regression-audit-summary-final.md`
- Active runtime/deploy scans found no active `lib/pq`, `dialect.Postgres`, `sql.Open("postgres", ...)`, PostgreSQL service/port guidance, `pg_dump`, or `psql` regressions.
- Remaining PostgreSQL-looking terms are documented as reference-only migrations, `integration && pglegacy` tests, generator compatibility metadata, adaptation comments, or false positives.

## Verification

- Backend targeted packages passed:
  - `cd backend; go test ./migrations ./internal/config ./internal/setup ./internal/repository ./internal/server/middleware ./internal/service ./internal/handler -count=1`
  - Evidence: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/go-test-backend-targeted-final.txt`
- Handler regression fixed and passed:
  - `cd backend; go test ./internal/handler -run TestOpenAIResponsesWebSocket_ContentModerationBlocksFirstFrame -count=1 -v`
  - `cd backend; go test ./internal/handler -count=1`
  - Evidence: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/go-test-handler-focused-final.txt`, `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/go-test-handler-final.txt`
- Frontend checks passed:
  - `pnpm --dir frontend run typecheck`
  - `pnpm --dir frontend exec eslint src/api/admin/riskControl.ts src/api/admin/settings.ts src/views/admin/SettingsView.vue src/views/admin/__tests__/SettingsView.spec.ts`
  - `pnpm --dir frontend run test:run -- src/views/admin/__tests__/SettingsView.spec.ts`
  - Evidence: `frontend-typecheck-final.txt`, `frontend-focused-eslint-after-fixes.txt`, `frontend-settings-vitest-final.txt`
- Diff whitespace check passed:
  - `git diff HEAD --check` exit `0`; output only line-ending normalization warnings.
  - Evidence: `git-diff-check-final.txt`

## Verification Gap / Residual Risk

- MySQL integration smoke was attempted with the closest repository integration test command:
  - `cd backend; go test -tags integration ./internal/repository -run 'TestEnqueueSchedulerOutbox|TestSchedulerOutbox|TestUserRepoSuite' -count=1 -v`
- Local Docker was unavailable, so the repository skipped testcontainer-backed integration tests: `docker is not available; skipping integration tests (start Docker to enable)`.
- Residual risk: empty-schema/repeated MySQL migration idempotency still needs a Docker/MySQL-capable environment before production promotion.

## Final Handoff

- Reviewed final diff stat: `openspec/changes/sync-upstream-main-into-mysql-fork/analysis/final-diff-stat.txt`
- Long-lived fork branch was not merged or fast-forwarded. Task 7.4 remains intentionally open for human review after this integration branch is inspected.
