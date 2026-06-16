## 1. Baseline and Branch Safety

- [x] 1.1 Record current branch, current `HEAD`, `upstream/main`, merge base, and `git rev-list --left-right --count HEAD...upstream/main` in the implementation notes or commit message.
- [x] 1.2 Create a timestamped backup branch or tag for the current fork head before any real merge.
- [x] 1.3 Create an isolated integration branch from the current fork head for the upstream sync.
- [x] 1.4 Run `git merge-tree HEAD upstream/main` or an equivalent no-commit merge preview and save the conflict file list for review.

## 2. Database and MySQL Guardrails

- [x] 2.1 Review upstream changes under `backend/internal/repository`, `backend/migrations`, `backend/ent/schema`, `backend/internal/config`, `backend/internal/setup`, `deploy`, `Dockerfile`, and `.github` for PostgreSQL assumptions.
- [x] 2.2 For every upstream migration accepted into the fork, convert or fold its intent into the active MySQL migration/baseline strategy instead of accepting PostgreSQL-only SQL verbatim.
- [x] 2.3 Resolve migration runner and migration test conflicts so placeholders, lock strategy, schema metadata, and idempotency remain compatible with MySQL.
- [x] 2.4 Resolve repository SQL conflicts by preserving upstream business intent while adapting placeholders, JSON functions, locking semantics, array handling, and return-value behavior to MySQL.
- [x] 2.5 Keep MySQL driver/config/setup/backup paths active and prevent `lib/pq`, `dialect.Postgres`, `sql.Open("postgres", ...)`, `pg_dump`, or `psql` from re-entering runtime paths.

## 3. Conflict Resolution by Functional Area

- [x] 3.1 Resolve generated-code prerequisites first: Go module dependencies, wire source files, Ent schema files, and provider definitions.
- [x] 3.2 Resolve gateway/OpenAI conflicts, including chat completions, messages, responses fallback, image failover, quota/probe behavior, cyber policy, and usage recording tests.
- [x] 3.3 Resolve content moderation conflicts across handler, repository, service, settings DTOs, and tests.
- [x] 3.4 Resolve scheduler outbox conflicts, preserving upstream dedup/cleanup/race fixes with MySQL-compatible persistence semantics.
- [x] 3.5 Resolve API key middleware, settings view, auth/oauth, billing fallback pricing, token refresh, rate limit, and channel monitor jitter changes.
- [x] 3.6 Resolve frontend conflicts in admin accounts, risk control, settings, usage views, API clients, shared types, i18n, and related tests.
- [x] 3.7 Resolve Docker, deploy, workflow, and release metadata conflicts without reverting default deployment guidance back to PostgreSQL.

## 4. Regeneration and Formatting

- [x] 4.1 Regenerate Ent outputs from resolved `backend/ent/schema/**/*.go` sources using the repository's existing generation command.
- [x] 4.2 Regenerate Wire outputs from resolved provider/source files using the repository's existing generation command.
- [x] 4.3 Run Go formatting on changed backend Go files.
- [x] 4.4 Run frontend formatting or lint-fix only where the repository already defines the command and only for changed files or focused checks.

## 5. PostgreSQL Regression Audit

- [x] 5.1 Run focused searches in active runtime/deploy paths for PostgreSQL regressions: `lib/pq`, `pq.`, `dialect.Postgres`, `sql.Open("postgres"`, `postgres://`, `DATABASE_URL` examples using PostgreSQL, `pg_dump`, `psql`, `pg_`, `jsonb`, `ILIKE`, `RETURNING`, `::`, `ANY(`, and `$1`-style hand SQL.
- [x] 5.2 For each remaining PostgreSQL term, either replace it, mark it as archived/reference-only documentation, or document why it is intentionally retained.
- [x] 5.3 Check migration filenames and numbering for collisions between upstream additions and fork MySQL migrations.
- [x] 5.4 Check Docker/deploy docs and workflow files for service names, health checks, ports, volumes, and packages that contradict the MySQL target.

## 6. Verification

- [x] 6.1 Run targeted backend tests for database config/setup/repository/migration packages touched by the merge.
- [x] 6.2 Run targeted backend tests for gateway/OpenAI/content moderation/scheduler/API key/settings packages touched by conflict resolution.
- [x] 6.3 Run a MySQL migration smoke test or the closest available testcontainer/integration test for empty-schema initialization and repeated migration idempotency.
- [x] 6.4 Run frontend typecheck/build and focused tests for changed admin views, API clients, i18n, and shared types.
- [x] 6.5 If any verification cannot run locally, document the command, reason, and residual risk in the final implementation notes.

## 7. Final Review and Handoff

- [x] 7.1 Review `git diff --stat` and high-risk diffs for unintended PostgreSQL rollback, generated-only noise, or public API drift.
- [x] 7.2 Ensure the final branch contains a clear merge/conflict-resolution history suitable for review.
- [x] 7.3 Summarize accepted upstream feature groups, MySQL adaptations, skipped/changed upstream migration handling, verification results, and known risks.
- [ ] 7.4 Only after successful review, merge or fast-forward the long-lived fork branch from the isolated integration branch.
