## Investigation
- [x] Inventory account credential paths, provider transports and stateful ownership.
- [x] Record create/edit configuration differences and intended parity.

## Implementation
- [x] Add validated ordered credentials, safe redaction, count and update semantics.
- [x] Implement bounded ordered upstream retry and recoverable exhaustion handling.
- [x] Integrate supported provider paths and protect stateful/streaming operations.
- [x] Add shared ordered-key controls to create/edit modals and account visibility.
- [x] Align missing create/edit settings and their persisted payloads.

## Verification and delivery
- [x] Run backend credential/transport/error-handling regressions.
- [x] Run frontend form parity, payload, locale and build checks.
- [x] Inspect the final diff and perform one AOCI maintenance checkpoint (blocked by out-of-scope historical debt; no index changes).
- [x] Record validation limits and release handoff for this implementation.

## Usage-query follow-up
- [x] Share create-extra assembly so ordinary API-key creation validates and persists usage-query and quota settings.
- [x] Query enabled third-party usage regardless of the provider's built-in OAuth/usage eligibility, and preserve retry after errors.
- [x] Show third-party usage-query ratios as percentages without a duplicate progress bar; keep native usage-window bars unchanged.
- [x] Verify submission, list loading/refresh, provider display consistency and frontend build.

Production merge/push/deployment is a separate delivery step; this feature has
not been released. Follow the repository's production workflow when releasing.
