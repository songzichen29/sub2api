# Integrate selected custom operations capabilities

## Why

Adopt selected operational features from DeanZFC/sub2api-custom at
915cc0ed24e896f404a1fe0918ee055de4acd40c while preserving this fork's
MySQL/PostgreSQL support, billing, scheduler, and OneBot/DSH integration.
The integration baseline is production commit
333de3d70600b6688bff6ccc80cc6841d39781c5. The original uncommitted draft in
codex/integrate-custom-operations-20261107 remains intact.

## Changes

- Preserve the deployed account metric recovery implementation and regression-test
  its admission behavior together with the new group controls.
- Add optional per-user, per-group concurrency alongside the existing global limit.
- Add one-hop account fallback after ordinary selection exhausts the original group;
  retain the original billing and permission identity.
- Add output TPS and bounded recent-request diagnostics without changing existing
  channel monitor rates, health rules, model filters, or public permissions.
- Add explicit generated-image format validation and bounded GIF preservation to
  the existing QQ business path; retain default image behavior and all rewards.

## Excluded

Per the user's November 8, 2026 instruction, model quality detection and all
quality extensions are excluded; existing plans, assessments, history and UI remain
unchanged. Also excluded: duplicate account performance tables/services, API-key
billing-group fallback, strict account tiers, automatic quality-driven account
movement, public-vote scheduling, QQ transport replacement, weighted check-in
defaults, restored invitation commands, Mihomo, ticket harvesting, and BPS.
Production deployment was initially excluded; the user's subsequent request
authorized production merge, push, image builds, deployment and verification.
Existing migration files remain immutable.

## Provenance

Reference: https://github.com/DeanZFC/sub2api-custom (LGPL-3.0).
Applicable attribution is retained in adapted components. Integration and
validation evidence are recorded separately. OpenSpec CLI is unavailable;
artifacts are maintained directly and no CLI validation is claimed.
