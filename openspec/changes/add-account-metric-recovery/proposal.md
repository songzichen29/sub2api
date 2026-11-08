# Proposal

## Why

Quota-window scheduling thresholds only describe provider-reported consumption; they do not detect accounts with persistently poor request success, cache efficiency, first-token latency, or total duration. Add opt-in account-level metric rules so administrators can isolate degraded accounts and let them recover through a bounded, observable trial without changing behavior for unconfigured accounts.

## What Changes

- Add per-account pause and recovery thresholds for success rate, cache rate, first-token latency, and request duration.
- Evaluate enabled rules on a bounded background cadence using existing operations telemetry.
- Record pause/recovery state and an audit history; require revision-aware writes for configuration and release actions.
- Add a bounded trial-admission path so a paused account can prove recovery without admitting unrestricted traffic.
- Keep the capability disabled by default and preserve existing account status, quota scheduling, gateway routing, and unrelated group protections.

## Capabilities

### New Capabilities
- `account-metric-recovery`: Opt-in account performance rules, automatic pause/recovery, bounded trial traffic, and auditable administration.

### Modified Capabilities

## Impact

- Backend account/ops repositories, scheduler admission, gateway concurrency, admin account APIs, and migrations for PostgreSQL and MySQL.
- Admin account list/details UI and focused backend/frontend tests.
- No changes to deployment workflows or default runtime behavior.
