## Why

Risk-control audit records currently keep only a truncated `input_excerpt`, which is not enough for administrators to reconstruct blocked multi-turn sessions or inspect the full offending request payload. Operators need a safe way to persist and download the complete request body for flagged moderation events without bloating the normal log list API.

## What Changes

- Persist a sanitized full request body for risk-control records only when the moderation event is flagged/blocked and a raw request body is available.
- Add database storage for the saved request body plus lightweight metadata that lets the UI know whether a downloadable body exists.
- Expose an admin-only download/detail API for a single moderation log's saved request body instead of returning large payloads in the paginated list response.
- Add a download action in the Risk Control log table/input detail UI for flagged records that have a saved request body, with fallback copy/download behavior for excerpt-only records.
- Add backend and frontend tests covering persistence, listing metadata, download access, and UI/type safety.

## Capabilities

### New Capabilities
- `risk-control-request-body-download`: Store and download full request bodies for flagged risk-control moderation logs.

### Modified Capabilities

None.

## Impact

- Backend service: `ContentModerationLog`, `buildLog`, cyber-policy logging paths, request-body size limiting and sanitization.
- Backend repository/API: `content_moderation_logs` persistence, list metadata, and a new admin-only single-log request-body endpoint.
- Database: new migration for PostgreSQL and MySQL using the next available migration numbers rather than the stale numbers in the draft plan.
- Frontend: Risk Control log types/API client, `RiskControlView.vue` download button, i18n labels.
- Tests: repository SQL tests, service log-building tests where practical, handler/API tests, and frontend type checking.
