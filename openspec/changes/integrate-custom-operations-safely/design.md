# Compatibility design

## Default behavior

New limits default to zero; fallback IDs default to null. Missing new request
fields preserve existing settings on update. All-off paths retain the existing
scheduler, billing, concurrency, account metric recovery and QQ behavior.
Public monitor responses retain current authorization and redaction.

## Group operations

Persist user_concurrency_limit and fallback_group_id_on_unavailable on groups,
including Ent schema, additive migrations, DTOs, admin inputs, repository projections,
authentication snapshots and invalidation. Reserve global user and group slots
atomically in Redis using a shared request ID; cancellation and completion release
both once. Keep existing account limits, API-key observation and WS ingress leases.

Group fallback is at most one hop, same-platform, active and cycle checked.
It requires confirmed absence of eligible accounts, not database errors,
saturation or cancellation. Preserve original billing group, price overrides,
subscription, RPM, reservation, model access and profit checks. Owner-bound
continuations cannot move credentials. Existing special-purpose fallback fields
and handlers retain their semantics.

## Account metrics

Keep production account_metric_recovery services, repositories, endpoints, UI
and migrations unchanged. Do not import the draft's competing account_performance
service/schema/admission hooks. Preserve manual status/schedulable, opt-in
membership, revision/pause-generation CAS protection, bounded trials and the
existing no-refund admission budget. Group concurrency adds a user-level limit
without replacing account metric admission.

## Monitor additions

TPS uses valid output tokens divided by generation time excluding TTFT.
Missing measurements stay unknown. Recent requests are capped at five per visible
dimension within the requested window and at most 24 hours, scoped to authorized
groups, with detailed errors limited to admins. Leave existing cache-rate
numerator/denominator and health thresholds intact.

## QQ additions

Explicit output format is opt-in. Existing default normalization remains. A GIF
request must return a real bounded GIF; preserve frames and refuse malformed or
oversized content instead of silently flattening it. Use the current business
queue, OneBot sending, authorization and accounting. Keep check-in distribution,
ledger keys, lottery eligibility and disabled invitation commands unchanged.

## Excluded quality work

The user explicitly removed model quality detection from this integration on
November 8, 2026. No capability checks, metadata checks, visual assessment,
renderer, quality migrations or quality UI changes are introduced.

## Validation

Cover all-off compatibility, group snapshot propagation, concurrent slot
admission/release, production metric admission, one-hop fallback identity,
monitor authorization, and default QQ reward/image behavior. Run focused backend
and frontend regressions, both builds, migration numbering/BOM checks and
database checks where an isolated runtime is available. Record unavailable
environments and baseline failures precisely.
