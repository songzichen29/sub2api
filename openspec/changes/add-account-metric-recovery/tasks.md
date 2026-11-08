# Tasks

## 1. Metrics and Persistence

- [x] 1.1 Add PostgreSQL and MySQL monitor/event migrations with a disabled default; verify static definitions, UTF-8 without BOM, and existing migration tests. Remove the unsupported literal TEXT default from the MySQL reason column. Native execution remains in 4.1.
- [x] 1.2 Add account-level rolling success/cache/TTFT/duration aggregation from Ops errors and usage rows; verify deduplication, event filtering, null metrics, and sample counts with repository tests.

## 2. Rules and Gateway Admission

- [x] 2.1 Implement validated per-account pause/recovery rules and pure breach/recovery evaluation; verify threshold boundaries, insufficient samples, invalid values, and recovery hysteresis with unit tests.
- [x] 2.2 Implement revisioned configuration, pause/recovery state, audit events, and background evaluation; verify stale writes and state transitions with repository/service tests. Real database locking remains in 4.1.
- [x] 2.3 Add opt-in membership admission and shared persisted trial limits; verify known protected accounts fail closed, ordinary accounts do no admission SQL, failed refreshes retain protection, and local configuration writes cannot be overwritten by stale refreshes. Document and test eventual cross-instance enablement; native concurrent locking/budget execution remains in 4.1.

## 3. Administration

- [x] 3.1 Add admin read/configure/release/history endpoints behind the existing admin route middleware; direct handler tests cover reading, saving, and malformed inputs. Revision-conflict behavior is covered at repository level; handler conflict mapping and end-to-end authentication/middleware coverage remain unverified.
- [x] 3.2 Add account metric configuration, status, evidence, and release controls to the existing account UI; verify Vue tests, TypeScript, and lint for affected files.

## 4. Integration Verification

- [ ] 4.1 Wire the service and repository for both database modes; verify server generation/build and focused PostgreSQL/MySQL integration tests. Wiring, generated server code, and migration/unit coverage are complete; live dual-database integration remains environment-dependent.
- [x] 4.2 Run focused backend and frontend suites plus whitespace/diff checks; record any environment-dependent test gaps.

## Fix verification — 2026-11-08

- Passed the default suites with `go test ./migrations ./internal/repository ./internal/service ./internal/handler/admin ./cmd/server -count=1`.
- Passed the final focused rerun with `go test ./internal/service ./internal/repository ./migrations -run 'AccountMetric|AccountPerformance|ProvideAccountMetric' -count=1`, including opt-in isolation, failed refresh retention, local enable/disable ordering, concurrent refresh/write protection, and the explicit cross-instance propagation boundary.
- Executed the production aggregate SQL against SQLite fixtures using both placeholder formats. Verified failed usage placeholders, retried successes, internal request exclusion, billed image successes, and computed sample counts/rates/timings. This does not prove native dialect or row-lock behavior.
- Regenerated Wire and passed `go build ./cmd/server` using an isolated temporary output, removed after verification.
- `TestAccountMetricNativeDatabases/mysql` and `/postgres` both explicitly skipped: `SUB2API_TEST_METRIC_MYSQL_DSN` and `SUB2API_TEST_METRIC_POSTGRES_DSN` were not configured. Their isolated local-database migration/idempotence, concurrent trial admission, budget, and revision/generation tests are implemented but not executed. Task 4.1 remains open.
- The Windows environment reports `CGO_ENABLED=0` and no available GCC/Clang; race-detector tests were not run. The passing default suites do not establish `-race` or `-tags=unit` coverage.
- Frontend code was unchanged by this correction; prior focused Vue, TypeScript, and lint results were not rerun or counted as new verification.
- `gofmt`, `git diff --check`, additional untracked-file whitespace checks, and raw-byte checks of both new migrations passed; both SQL files begin with `43 52 45 41 54 45 20 54` (`CREATE T`), not a BOM.
- No commit, push, deployment, or production database operation was performed for this fix.

## Workflow follow-up

- Archive the change after implementation and review requirements are satisfied.
