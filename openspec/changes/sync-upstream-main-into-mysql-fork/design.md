## Context

Current local evidence:

- Current branch: `merge/upstream-main-into-pgsql-mysql-20260426` tracking `origin/merge/upstream-main-into-pgsql-mysql-20260426`.
- Remotes: `origin` is the fork, `upstream` is `https://github.com/Wei-Shaw/sub2api.git`.
- Latest fetched upstream head: `upstream/main` at `4a5665da chore: sync VERSION to 0.1.137 [skip ci]`.
- Merge base between current branch and `upstream/main`: `e34ad2b19424844cbd1bcffb599e5b0002ad3c50`.
- Divergence from merge base: current branch has about 338 commits, upstream has about 80 commits.
- The fork already contains substantial MySQL migration work: `github.com/go-sql-driver/mysql`, MySQL testcontainers, `sql.Open("mysql", ...)`, MySQL DSN generation, `backup_mysql_dumper.go`, MySQL-oriented setup/config tests, and docs under `docs/plan-pgsql-to-mysql-replacement.md`.
- Upstream changes since the merge base include gateway fixes/features, OpenAI quota reset, thinking protocol normalization, scheduler outbox dedup/cleanup fixes, channel monitor jitter, cyber policy handling, billing fallback pricing, frontend admin UI changes, and new PostgreSQL-style migrations numbered around 151-153.

A merge-tree preview shows conflicts in cross-cutting files including generated wire/ent code, OpenAI gateway handlers/services/tests, content moderation, scheduler outbox repository, API key middleware, settings DTO/view logic, migration tests, and frontend admin views/i18n. These conflicts should be treated as expected integration work rather than resolved by blindly favoring either side.

## Goals / Non-Goals

**Goals:**

- Bring the fork close to the latest `upstream/main` behavior while preserving the fork's MySQL database direction.
- Keep MySQL as the authoritative runtime database target for this fork during conflict resolution.
- Prevent PostgreSQL-only upstream migrations, SQL fragments, drivers, Docker services, or docs from re-entering active runtime paths.
- Resolve generated-code conflicts through source/schema/wire definitions first, then regenerate artifacts.
- Provide a repeatable merge workflow with clear rollback points and verification gates.

**Non-Goals:**

- Do not restore PostgreSQL as a supported runtime database for the fork.
- Do not complete the entire PostgreSQL-to-MySQL migration if unrelated to absorbing the current upstream delta.
- Do not implement a production data migration from PostgreSQL to MySQL in this sync change.
- Do not mechanically translate every historical upstream PostgreSQL migration; prefer the fork's MySQL baseline strategy.
- Do not change public API contracts unless required by upstream features being accepted.

## Decisions

### Decision 1: Merge into an isolated sync branch, not directly into the long-lived fork branch

Create a temporary integration branch from the current fork head, for example `codex/sync-upstream-main-into-mysql-fork`, and perform the upstream merge there. Keep the existing branch and a timestamped backup ref intact.

Rationale: the merge preview shows many conflicts across backend, frontend, and generated files. Isolating the merge keeps rollback simple and allows partial validation before replacing the current branch.

Alternative considered: merge directly into `merge/upstream-main-into-pgsql-mysql-20260426`. This is faster but makes rollback and review harder if conflict resolution accidentally reintroduces PostgreSQL assumptions.

### Decision 2: Accept upstream business logic, but adapt database-facing changes to MySQL

For non-database gateway/service/frontend changes, prefer upstream behavior unless the fork already has a deliberate local override. For database-facing changes, re-express upstream intent in MySQL terms instead of accepting PostgreSQL SQL or migrations verbatim.

Examples:

- Upstream scheduler outbox fixes should be kept, but index/migration SQL must match MySQL syntax and the fork's migration runner.
- Upstream repository query fixes should be kept, but placeholders, JSON functions, locking semantics, array handling, and `RETURNING` assumptions must be audited.
- Upstream migration numbers may conflict with fork migrations; renumber or fold them into the MySQL baseline/increment sequence rather than preserving PostgreSQL migration files as active scripts.

Alternative considered: favor `upstream/main` for all conflicts and then repair MySQL afterward. This increases the chance of a compiling but PostgreSQL-regressed tree.

### Decision 3: Treat generated artifacts as rebuild outputs

Files under generated wire and ent output should not be manually reconciled as the source of truth. Resolve source definitions first:

- Wire sources: `backend/cmd/server/wire.go`, `backend/internal/service/wire.go`, `backend/internal/handler/wire.go`.
- Ent schemas: `backend/ent/schema/**/*.go`.
- Go module dependencies: `backend/go.mod` / `backend/go.sum`.

Then regenerate `wire_gen.go` and `backend/ent/**` through the repository's existing generation commands.

Alternative considered: hand-edit generated files to clear conflicts. This is brittle and can hide schema/provider conflicts until later.

### Decision 4: Separate migration acceptance from migration execution

Any upstream migration that targets PostgreSQL should be reviewed for intent and converted to a MySQL-compatible migration or incorporated into the fork's MySQL baseline. Active migration files must be executable by the MySQL runner and idempotent under repeated startup.

Alternative considered: keep upstream PostgreSQL migrations in `backend/migrations/` and rely on runtime filtering. This increases ambiguity and risks accidental execution unless the runner already enforces dialect scoping.

### Decision 5: Verify by risk area, not only by full test suite

The sync should run targeted checks before broad checks:

1. Go compile/tests for changed backend packages.
2. Migration runner and schema smoke tests against MySQL paths.
3. Gateway/OpenAI/content moderation/scheduler tests touched by upstream conflicts.
4. Frontend typecheck/build and focused view tests.
5. Optional full backend/frontend suites after targeted failures are resolved.

Alternative considered: run the full suite only after all conflicts are resolved. Full-suite failures will be harder to attribute without earlier risk-area gates.

## Risks / Trade-offs

- [Risk] PostgreSQL-only SQL re-enters repository or migration runtime paths → Mitigation: run focused `rg` scans for `postgres`, `pq.`, `dialect.Postgres`, `ILIKE`, `RETURNING`, `::`, `jsonb`, `pg_`, `$1`-style SQL in active DB code after conflict resolution.
- [Risk] Generated code conflicts hide source-level incompatibility → Mitigation: discard/regenerate generated outputs after source conflicts are resolved.
- [Risk] Upstream migration numbers collide with fork MySQL migration sequence → Mitigation: assign MySQL-specific migration numbers/names and document any upstream intent that is folded into baseline.
- [Risk] Scheduler outbox concurrency semantics differ between PostgreSQL and MySQL → Mitigation: preserve upstream behavioral tests, add/adjust MySQL-specific tests for dedup, cleanup, and lock behavior.
- [Risk] Frontend/admin contracts drift from backend DTOs → Mitigation: resolve DTO/API/frontend files as one group and run TypeScript tests/build.
- [Risk] Full merge becomes too large to review → Mitigation: commit in staged groups if implementation proceeds: merge scaffold, backend DB adaptations, backend business conflicts, frontend conflicts, generated artifacts, verification fixes.

## Migration Plan

1. Fetch `upstream` and `origin`, record current `HEAD`, upstream head, and merge base.
2. Create a backup branch or tag for current fork head.
3. Create an isolated sync branch from current fork head.
4. Merge `upstream/main` without auto-committing if possible, then resolve conflicts by groups:
   - generated/wire/ent source prerequisites;
   - migrations and MySQL runner/schema;
   - repository SQL and integration tests;
   - gateway/service/handler conflicts;
   - frontend API/types/i18n/views;
   - Docker/CI/deploy docs.
5. Regenerate wire and ent outputs after source conflicts are resolved.
6. Run targeted verification gates and fix regressions.
7. Review the final diff for PostgreSQL regressions and unintended public API changes.
8. Merge or fast-forward the long-lived fork branch only after verification passes or known gaps are explicitly documented.

Rollback strategy:

- Abort an in-progress merge with `git merge --abort` while still resolving conflicts.
- Reset the isolated sync branch to the recorded pre-merge head if resolution becomes inconsistent.
- Keep the long-lived fork branch untouched until the sync branch is validated.

## Open Questions

- Should the final sync branch keep all upstream commits as a merge commit, or squash local conflict-resolution commits after the merge?
- What MySQL version/collation should be treated as the verification baseline if not already fixed by the deployment configuration?
- Should upstream PostgreSQL migration files remain in the repository as archived reference files, or be converted/renumbered into active MySQL migrations only?
- Which verification environment is available for MySQL integration tests: local MySQL, Docker Compose, or testcontainers only?
