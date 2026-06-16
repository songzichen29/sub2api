# PostgreSQL Regression Audit Summary (final)

Active runtime/deploy target remains MySQL.

Clean / replaced:
- deploy, Dockerfile, and .github: no `postgres`, `pg_isready`, `pg_dump`, `psql`, `postgres_data`, or port `5432` hits.
- active DB setup uses `sql.Open("mysql", ...)` and `migrations.MySQLFS`.
- repository SQL hotspots use MySQL placeholders, JSON functions, `GET_LOCK`/`RELEASE_LOCK`, `SELECT ... FOR UPDATE`, and `ON DUPLICATE KEY UPDATE` where needed.
- upstream PostgreSQL migration intents were added as MySQL migrations under `backend/migrations/mysql/031-034`.

Intentionally retained / reference-only:
- root `backend/migrations/*.sql` PostgreSQL files remain embedded as historical/reference migrations; production migration execution uses `migrations.MySQLFS` (`backend/internal/repository/ent.go`).
- `integration && pglegacy` migration/auth identity tests retain PostgreSQL catalog assertions as legacy reference tests only.
- Ent schema/generated metadata may mention non-MySQL dialects for generator compatibility; runtime opens Ent with `dialect.MySQL`.
- Comments that explicitly describe MySQL differences from former PostgreSQL behavior are retained where they explain the adaptation.

Remaining grep noise:
- `pq.` local variable names in platform quota validation tests are not `lib/pq` usage.
- `$` hits in shell scripts, USD pricing comments, and regexp replacement strings are not SQL placeholders.
- `psql` substring false positives occur in words like `PruneRollupSQL`.
