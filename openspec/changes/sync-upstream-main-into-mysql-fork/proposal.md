## Why

The fork needs to absorb the latest `upstream/main` changes without regressing the fork's ongoing PostgreSQL-to-MySQL conversion. The current branch is substantially diverged from upstream, and a direct merge has many conflict hotspots where upstream PostgreSQL-oriented migrations, repository SQL, and generated code can overwrite or invalidate MySQL-specific work.

## What Changes

- Define a controlled upstream-sync process from `upstream/main` into the MySQL fork branch.
- Preserve MySQL as the fork's primary database direction while selectively adapting upstream changes that still assume PostgreSQL.
- Classify merge conflicts by risk: generated code, database/migrations, repository SQL, gateway/service behavior, frontend/API contracts, CI/deploy.
- Establish verification gates for compilation, targeted backend tests, migration smoke checks, and frontend build/test checks.
- Document fallback and rollback points before applying a real merge.

## Capabilities

### New Capabilities
- `mysql-fork-upstream-sync`: Controlled synchronization of upstream changes into a fork that is migrating from PostgreSQL to MySQL, including conflict triage, database dialect adaptation, and verification requirements.

### Modified Capabilities
- None.

## Impact

- Affected branches/remotes: current fork branch `merge/upstream-main-into-pgsql-mysql-20260426`, `origin`, and `upstream/main`.
- Affected backend areas: `backend/internal/repository`, `backend/migrations`, `backend/ent`, `backend/internal/service`, `backend/internal/handler`, `backend/internal/config`, setup and tests.
- Affected frontend areas: admin account/risk/settings/usage views, i18n, API DTO/types.
- Affected operations: Dockerfiles, compose/deploy assets, GitHub workflows, release metadata.
- Database impact: upstream PostgreSQL-specific SQL or migrations must not be accepted verbatim when they conflict with MySQL runner/schema strategy.
