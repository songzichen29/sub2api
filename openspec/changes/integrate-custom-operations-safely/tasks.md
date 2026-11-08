# Implementation

- [x] Preserve original draft and create isolated worktree from current production.
- [x] Record provenance and exclude model quality work per user instruction.
- [x] Add group fields, additive MySQL/PostgreSQL migrations and ORM generation.
- [x] Verify propagation through repositories, DTOs, auth snapshots and invalidation.
- [x] Enforce atomic user/group concurrency including Live lease lifecycle.
- [x] Add group management controls and regression tests.
- [x] Implement one-hop fallback retaining original billing/admission identity.
- [x] Verify error classification, ownership and no recursive/saturated fallback.
- [x] Preserve production metric recovery and verify admission compatibility.
- [x] Add monitor TPS and bounded recent requests with current visibility rules.
- [x] Verify monitor UI and unchanged health/statistical semantics.
- [x] Add explicit QQ image formats and bounded GIF preservation.
- [x] Verify QQ defaults, command permissions and rewards remain unchanged.
- [x] Run focused tests, available database checks, builds and diff/BOM checks.
- [x] Check AOCI binding/status once without expanding historical maintenance.
- [x] Write integration/validation report and prepare reviewable changes.

## Validation limits

- Real database/Redis execution remains unverified: this machine has no Docker
  runtime, and the existing integration-tag test suite fails compilation in
  unrelated account/proxy tests. The new persistence regression is included for
  execution after that harness is repaired. No live database was used for tests.
- The unit-tag service suite also has pre-existing proxy/backup test API drift.
  Default-tag focused regressions pass; this is not a claim that all test suites pass.
- AOCI MCP is bound to D:/data/sub2api, not this isolated worktree. Its check reports
  1203 semantic changes and 12 orphans in that main repository. Maintenance was
  not invoked against the wrong repository, and no index alignment is claimed.
- No live QQ delivery, browser end-to-end run, push or production deployment
  forms part of this completed local implementation.
