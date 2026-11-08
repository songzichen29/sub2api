# Integration validation — 2026-11-08

## Scope and preservation

Integration branch: codex/complete-custom-operations-20261108.
Working directory: D:/data/sub2api-operations-final-20261108.
Base: production 333de3d70600b6688bff6ccc80cc6841d39781c5, also confirmed as
the remote production branch head during this work.

The original 111-file uncommitted draft in
D:/data/sub2api-custom-operations-20261107 is preserved. Its alternative
account_performance implementation, migrations, UI and wiring were not imported.
Production account_metric_recovery remains authoritative. Its source files and
all model_quality source files have no diff from the baseline.

Model quality detection, capability/metadata probing and visual assessment were
explicitly removed from this change at the user's request. No new renderer,
quality task, quality migration or quality UI is introduced.

## Functional entry points

| Capability | Entry point | Behavior |
| --- | --- | --- |
| Per-user group concurrency | Admin → Groups → Create/Edit | Zero disables it. Positive limits reserve global and group capacity atomically, including renewable Live leases. |
| Group fallback | Admin → Groups → Create/Edit | Zero disables it. Active same-platform target, one hop after eligible accounts are exhausted. Capacity errors, infrastructure errors and bound continuations do not borrow another pool. |
| Channel diagnostics | Existing channel-monitor V2 matrix | Output tokens per second excludes TTFT; missing data remains unknown. Expand a row for up to five recent requests within the requested window, capped at 24 hours. |
| QQ image output | Admin → QQ bot → Image settings; image tool output_format; /生图 --gif or --static | Default image normalization is unchanged. Explicit png/jpeg/webp/gif is validated; real GIF bytes and animation are preserved within resource limits. |

Fallback retains source billing, pricing restrictions, profit gates and user
admission. The integration additionally fixes the draft's weaker-destination
case: OAuth-only and privacy requirements apply from both source and target,
without mutating shared group/account snapshots.

Monitor request details remain scoped to visible groups. Raw error details are
admin-only; output TPS follows the existing throughput visibility setting.
Existing rate calculations and health thresholds remain unchanged.

QQ rewards, account binding, lottery, invitation restrictions and the OneBot/DSH
transport are not replaced. Format selection does not add a second image request.
Unsupported GIF output returns a clear failure instead of pretending a static
image is animated.

## Migrations

- PostgreSQL: 254_group_operations_controls.sql; 255_channel_monitor_output_tps.sql.
- MySQL: 113_group_operations_controls.sql; 114_channel_monitor_output_tps.sql.
- Existing migrations are unchanged. No duplicate account metric schema is added.
- All four new SQL files were checked bytewise: UTF-8 without BOM.
- Ent was regenerated from the final schema; no dependency-file changes resulted.

## Passing checks

- Focused Go regression run across service, repository, qqcommunity, admin, DTO,
  migrations and server compilation: account metric admission/recovery, group
  concurrency, Live lease lifecycle, fallback policy/ownership, image handling,
  channel monitoring and profit control.
- Additional qqcommunity/handler regressions: image paths, rewards, lottery,
  check-in, invitation restrictions, user-slot helpers and Live handling.
- Unit-tag handler selection also passes; the service unit-tag package is blocked
  by unrelated stale tests as described below.
- Frontend: 15 files / 95 tests pass, including new group controls, monitor
  components, existing account metric UI/list logic, group editing/copying,
  QQ settings, API contracts and locale completeness.
- QQ runtime: node --test test/capabilities.test.mjs — 3/3 pass.
- pnpm build passes, including locale checks, vue-tsc and Vite.
- go build ./cmd/server passes.
- git diff --check passes.

## Limits and reproducibility

Redis atomicity tests use miniredis with independent Redis clients; they are not
evidence of execution against a real Redis server. A real MySQL group persistence
and authentication-projection regression was added with the existing integration
build tag, but this Windows host has no Docker runtime.

The command:

    go test -tags integration ./internal/repository -run '^TestGroupOperationsPersistenceAndAuthProjection$' -count=1 -v -timeout 5m

fails during compilation of existing tests: ListOAuthRefreshCandidatePage and
SetRateLimitedIfUnchanged no longer exist, and sweepOneExpiredProxy has a changed
signature. Those files are unchanged from production. The unit-tag service suite
similarly references obsolete proxy credential types and saveRecordWithArchive.
These unrelated test repairs were not included, and no full-suite success is claimed.

Frontend builds emit existing bundle-size/dynamic-import/Browserslist warnings.
QQ view tests pass with a jsdom network AggregateError warning from a background
request. No live browser or QQ delivery was verified.

Detailed local logs are under the Windows temporary directory with prefix
sub2api-operations-final-, including backend2.log, boundaries.log, frontend.log,
compat.log, build.log, go-build.log, integration.log and unit.log.

## AOCI

The MCP checkpoint identifies D:/data/sub2api as its runtime repository, rather
than this isolated worktree. It reports 1203 semantic changes and 12 orphans
in that repository. No full overview, semantic maintenance, historical cleanup
or index rewrite was performed, and this change does not claim AOCI alignment.

## Release status

The user subsequently authorized production merge and deployment. The four
implementation commits through cd76199af were fast-forwarded and pushed to
merge/upstream-main-into-pgsql-mysql-20260426 on November 8, 2026.

Release preflight used temporary MySQL and Redis containers with the exact
production engine images, an internal test network and no production data mounts.
Only production schema was copied; no business rows were copied or changed.
MySQL migrations 113/114 passed twice, including idempotency. Real database
group create/update/read, API-key authentication projection, monitor aggregation,
90-minute matrix and recent-request SQL passed. Real Redis independent-client
contention, combined global/group limits and Live lease transfer/refresh/release
passed. The isolated test containers, test volumes and network were removed.
The temporary harness used a Go test overlay to avoid the unrelated broken
integration-tag suite; this does not claim that the full integration suite passes.

The first QQ image build exposed an SDK schema error: optional parameters must
omit required rather than specify required:false. This prevented the entire QQ
tool module from registering. The format declaration was corrected, a real SDK
registration regression was added, and runtime failures now expose mock tool
error results. All 36 Node tests passed in an isolated container on the QQ host,
including real SDK execution with a mock model and restart/history recovery.
No real QQ messages were sent for these checks.

GitHub image builds and actual deployment verification continue after the fix.
Release evidence and rollback assets are stored on the corresponding hosts at
/data/ops/releases/custom-operations-cd76199af-20261108; credentials and business
data are not included in repository evidence. AOCI's final maintenance check
reported 1223 historical/out-of-scope targets and 12 orphans, with no pending
recovery. No semantic index update or alignment is claimed.
