## 1. Database and Models

- [x] 1.1 Pick the next available PostgreSQL and MySQL migration numbers and add `request_body` storage to `content_moderation_logs` without modifying already-applied migrations.
- [x] 1.2 Add `RequestBody`, `HasRequestBody`, and `RequestBodySize` fields to `service.ContentModerationLog`, keeping `RequestBody` omitted from normal list JSON unless explicitly returned by the download endpoint.
- [x] 1.3 Add a bounded request-session audit-bundle preparation helper that parses known protocols, records session boundary metadata, sanitizes content, and caps stored bytes before persistence.

## 2. Moderation Logging

- [x] 2.1 Update `buildLog` to accept the raw request body and persist the prepared request-local session audit bundle only when `flagged == true`.
- [x] 2.2 Update keyword block, hash block, synchronous moderation block/allow, and cyber-policy logging call sites to pass the available raw body where appropriate.
- [x] 2.3 Ensure error and non-flagged records do not persist full request bodies while retaining existing `input_excerpt` behavior.

## 3. Repository and Service APIs

- [x] 3.1 Update `CreateLog` to insert `request_body`.
- [x] 3.2 Update `ListLogs` to return `has_request_body` and `request_body_size` metadata without selecting or scanning the full body content.
- [x] 3.3 Add repository/service methods to fetch one moderation log request session audit bundle by log ID, returning a typed not-found result when no body exists.
- [x] 3.4 Ensure existing cleanup/delete paths remove request bodies together with their moderation log rows.

## 4. Admin HTTP API

- [x] 4.1 Add an admin-only route such as `GET /api/v1/admin/risk-control/logs/:id/request-body`.
- [x] 4.2 Implement handler validation for log ID and return a response containing the saved audit bundle plus filename/content metadata suitable for frontend download.
- [x] 4.3 Ensure missing body, missing log, or invalid ID responses use existing typed error/response conventions.

## 5. Frontend UI

- [x] 5.1 Extend `frontend/src/api/admin/riskControl.ts` types with `has_request_body`, `request_body_size`, and a request-body download API method.
- [x] 5.2 Add a download action in `RiskControlView.vue` table/input detail UI only for rows with saved request bodies.
- [x] 5.3 Implement browser download using the single-log API response and a safe filename based on log ID/request ID/time.
- [x] 5.4 Add zh/en i18n labels for download action, unavailable body, and download failure.

## 6. Tests and Verification

- [x] 6.1 Update repository SQL tests for `CreateLog`, `ListLogs`, and the new request-body fetch method.
- [x] 6.2 Add or update service tests for flagged-only audit-bundle persistence, protocol boundary extraction, size limiting, and non-flagged omission.
- [x] 6.3 Add handler tests for successful request-body download and missing-body/invalid-ID errors.
- [x] 6.4 Run focused backend tests for service/repository/admin handler packages.
- [x] 6.5 Run frontend type checking with `pnpm typecheck`.

