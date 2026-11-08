# Design

## Context

The repository already has provider quota-window scheduling thresholds and an Ops monitoring switch, but it has no account-level rolling performance monitor. Existing routing state is cached and account status/schedulable fields also represent administrator intent, so metric pauses must remain separate from those fields and be checked at the live admission boundary.

## Goals / Non-Goals

**Goals:**
- Add opt-in success-rate, cache-rate, first-token latency, and duration rules with explicit pause and recovery bounds.
- Use request outcomes and timing already captured by Ops error logs and usage logs.
- Persist monitor state and trial admission atomically across service instances, with audit evidence.
- Keep database unavailability fail-closed for accounts whose metric pause state must be checked.

**Non-Goals:**
- Replacing provider quota thresholds, existing account cooldowns, group failover, or manual account controls.
- Deleting or replacing any existing anti-abuse, group protection, or gateway behavior.
- Making metric monitoring active for existing accounts or enabling it by default.

## Decisions

- **Keep monitor state in dedicated tables.** Account status and schedulable flags carry manual/admin intent and are used throughout the gateway. A separate monitor row owns only this feature's pause, configuration revision, trial state, and reason; the scheduler overlays that state without rewriting the account.
- **Reuse Ops telemetry with a metric-only success definition.** Positive cost or positive text/cache tokens identify successful usage, including free-token responses and billed images. Zero-cost, zero-token failure placeholders do not count. Deduplicate by account/API key/request identity, retaining the latest row. Terminal provider errors override partial HTTP usage; WebSocket connection errors retain completed turns. Both aggregates exclude cyber-blocked and channel-monitor requests (`request_type` 4 and 6); errors also exclude token-count, business-limit, and client-owned events. Cache and TTFT use eligible successful streaming/WebSocket observations; image input tokens are excluded from the cache denominator. Zero timing is valid; negative/null timing is excluded. The billing/dashboard success filter is unchanged.
- **Gate evaluation and enablement on Ops monitoring.** Disabled Ops stops evaluation and prevents enabling rules, while still allowing configuration disable and explicit release. Membership refresh and protected accounts' persisted pause checks continue. An empty fresh measurement window does not mean Ops is unavailable.
- **Separate list visibility from fresh evaluation evidence.** Batch list metrics always cover the complete preceding hour, independent of configuration edits and recovery. Detail/evaluation start at the latest of the rolling-hour boundary, `metrics_since`, and (when applicable) the current trial start. Each enabled rule uses its own observation count; insufficient or invalid measurements do not imply a breach or recovery.
- **Support dual bounds and preserve saved rules.** Each metric accepts pause min/max, recovery min/max and its own `min_samples`. Omitted recovery bounds inherit the corresponding pause bound and must define a range inside the normal range. Empty pause bounds disable that metric; recovery-only bounds are rejected. Legacy single-bound JSON retains its semantics, including a zero rate threshold. Saved cooldown/sample values are not overwritten. New defaults are 300 seconds, 20 pause samples and three recovery samples; recovery samples allow 1–100. The previous 30-second minimum cooldown remains accepted for compatibility. Missing `auto_recover` preserves automatic recovery; explicit false disables trials and automatic recovery.
- **Use hysteresis and bounded trials for recovery.** Recovery bounds default to the pause bounds if not specified and must remain within the normal range. After cooldown, persistent SQL state admits at most one trial every five seconds, for ten minutes and no more than three times `recovery_samples`. Account concurrency is acquired before reserving a trial; metric rejection releases that concurrency slot. Later gateway rejection or cancellation does not refund a reserved trial, so the budget is a conservative admission limit rather than a count of completed upstream requests. A trial recovers only when every configured metric meets its recovery bound for the configured sample count.
- **Make writes revision-aware and auditable.** Configuration saves, explicit release, pause, trial, recovery, and retirement use transactions and monotonically increasing revisions/generations where relevant. Background evidence is bound to configuration revision, pause generation and trial start; the row lock rejects evidence from an older cycle. Stale admin writes fail with a conflict instead of changing newer state. Configuration audit events include the full saved configuration.
- **Expose batch metrics and administration without replacing manual state.** A batch endpoint accepts 1–100 positive account IDs and uses two SQL queries per batch. Existing restricted admin-key middleware rejects metric routes. The account list shows the four metrics under recent usage and a separate enabled/paused/trial marker under status; either entry opens the existing configuration panel in a dialog. Pagination, aborted/late requests and account switching cannot overwrite a newer view. Saves/releases and account-list HTTP 304 refresh metric state independently. The panel shows sample counts, the actual evaluation window, pause reason, trial budget, and paginated audit evidence; stale-write conflicts require refresh and preserve the unsaved form. Explicit release clears the pause and disables rules.
- **Integrate at the existing account admission boundary with an opt-in membership snapshot.** Startup must load the enabled-or-paused account set successfully before serving requests. Ordinary accounts outside that immutable set bypass metric SQL entirely; known protected accounts read persisted pause state on every admission and cannot fall back to a cached allow on database errors. A paused account can pass only through the atomic trial admission path. Existing account checks remain in place; account concurrency is acquired before metric admission.
- **Serialize local configuration writes and membership refreshes.** A local enable publishes protection before its SQL write, while a disable/release removes protection only after a successful readback. An uncertain write retains protection until a successful refresh reconciles it. Refreshes run even when Ops is disabled, preserve the last good set on failure, and cannot overwrite a newer local write with an older list. Request readers do not wait on the refresh/write mutex.
- **Keep cross-instance enablement explicitly eventually consistent.** A newly enabled account becomes protected on other instances after their next successful background refresh (normally on the 30-second cadence). A failed refresh extends that delay without a finite upper bound for an account previously unknown to that instance. Persisted pauses and shared trial limits are authoritative once the account is in that instance's protected set; this is not an immediate cluster-wide enablement guarantee. Multi-instance rollout must synchronize membership before relying on the new account's protection; stronger propagation is outside this patch.
- **Support both SQL dialects explicitly.** Add PostgreSQL and MySQL migrations and repository queries using existing dialect helpers. Do not depend on PostgreSQL-only JSON operators or array binding in MySQL code.

## Risks / Trade-offs

- [Risk] Ops error and usage rows can represent different phases of a request → Mitigation: deduplicate by account/API key/request identity, exclude internal probes and token-count-only events, and cover terminal-error overrides with repository tests.
- [Risk] Trial traffic can harm users if too many requests reach a degraded upstream → Mitigation: shared transactional rate/budget limits, short trial lifetime, and normal gateway concurrency/admission checks.
- [Risk] Cached routing snapshots can continue to include a paused account → Mitigation: enforce persisted pause authority for known protected accounts at live admission and fail closed on DB read errors. The separate opt-in membership propagation boundary above still applies to newly enabled accounts on other instances.
- [Risk] MySQL JSON/locking semantics differ from PostgreSQL → Mitigation: separate migrations and dialect-specific SQL; add MySQL integration coverage for atomic trial admission and revision conflicts.
- [Risk] Enabling Ops monitoring later may expose old rolling data → Mitigation: use the later of the rolling one-hour start and persisted `metrics_since`; configuration saves and successful recovery reset that timestamp so older evidence cannot immediately re-pause a recovered account.

## Migration Plan

1. Add additive monitor and event tables in both migration tracks; no account rows or existing scheduling settings are rewritten.
2. Deploy code with all new configs disabled by default. Validate native migrations/locking on both engines, Ops monitoring, and admin visibility before enabling rules. For multiple gateway instances, confirm that every instance has refreshed a newly enabled account before relying on its protection.
3. Roll back by disabling rules and deploying the previous image; persisted monitor tables may remain unused and contain only this feature's configuration/audit records.

## Open Questions

Immediate cross-instance enablement is not provided by the membership cache; deployment review must account for that boundary before enabling the feature on multiple instances. Native dialect verification is recorded in `tasks.md`.

## Reference integration boundaries

The completion compares the account-performance feature in `deanzfc/sub2api-custom` at `915cc0ed24e896f404a1fe0918ee055de4acd40c`. It adapts the rule controls and visibility to this repository's independent monitor tables and existing admission path; it does not merge the fork wholesale. Deliberate retained differences are per-metric sample sufficiency, legacy 30-second cooldown support, conservative non-refundable trial reservations, and eventually consistent cross-instance enablement. Manual account status, schedulable flags, group zero-cache protections, billing, and unrelated gateway behavior retain their existing ownership.
