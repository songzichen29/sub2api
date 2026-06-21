## Context

Risk-control moderation logs currently persist a truncated `input_excerpt` (currently capped at 12,000 runes) and expose it through the paginated admin log list. This is useful for quick review but insufficient for blocked multi-turn Chat Completions or Anthropic Messages requests whose full JSON request body can be much larger than the excerpt.

The draft plan is directionally correct but needs implementation corrections:

- Migration numbers in the draft are stale. The current PostgreSQL migration sequence is beyond `155_add_coupons_and_discount_fields.sql`; MySQL is beyond `036_add_coupons_and_discount_fields.sql`. The implementation must use the next available numbers at implementation time.
- Full request bodies must not be returned in the paginated list response. Lists should stay lightweight and expose only metadata such as `has_request_body` and `request_body_size`.
- Storage must have an application-level byte cap and sanitization step. Database `TEXT`/`LONGTEXT` capacity is not an invitation to persist unbounded request payloads.

## Goals / Non-Goals

**Goals:**

- Persist a sanitized audit bundle for flagged risk-control moderation records when the original request body is available. The bundle includes the exact request payload evaluated by risk control plus protocol-specific session boundary metadata.
- Keep existing `input_excerpt` behavior for table display and search.
- Add an admin-only endpoint to download/read the saved audit bundle for one moderation log.
- Expose lightweight list metadata so the frontend can show download availability without transferring the body in every row.
- Support both PostgreSQL and MySQL deployments.

**Non-Goals:**

- Storing full request bodies for non-flagged allow/pass records.
- Storing arbitrary GB-sized payloads; storage is bounded by code.
- Replacing `input_excerpt` search or changing moderation decision semantics.
- Providing user-facing access to request bodies.
- Backfilling old moderation logs that only have excerpts.

## Decisions

1. **Use a separate download endpoint instead of embedding `request_body` in `ListLogs`.**
   - Decision: add an admin route such as `GET /api/v1/admin/risk-control/logs/:id/request-body` that returns the saved body for one log.
   - Rationale: the log list can contain many rows. Returning full JSON bodies inline would increase latency, memory pressure, and browser payload size.
   - Alternative considered: include `request_body` directly in `ListLogs`. Rejected because it makes the common list view expensive and exposes large/sensitive data more broadly than needed.

2. **Store request-body/session metadata in list rows.**
   - Decision: add `has_request_body`, `request_body_size`, and optional `session_message_count` fields to `ContentModerationLog` list responses.
   - Rationale: the UI can render a download button only when data is present, without fetching the body eagerly.
   - Alternative considered: infer availability from `flagged`. Rejected because some flagged records may have no original body or body may be dropped by size/sanitization rules.

3. **Persist only sanitized, bounded request bodies for flagged records.**
   - Decision: store a sanitized audit bundle derived from `ContentModerationCheckInput.Body` only when `flagged == true` and body is non-empty, with a fixed byte limit such as 1 MiB unless project configuration already provides a better central limit. For chat protocols, parse the body to identify session start/end from the request payload itself: Anthropic `system` plus `messages[0..n-1]`, OpenAI Chat `messages[0..n-1]`, OpenAI Responses `input[0..n-1]` where applicable, and Gemini `contents[0..n-1]`. If parsing fails, store the bounded sanitized raw JSON with `boundary_source=raw_request`.
   - Rationale: prevents unbounded DB growth and reduces risk of storing credentials or transport secrets.
   - Alternative considered: storing the raw body unmodified. Rejected due to sensitive data and storage risk.

4. **Keep `input_excerpt` unchanged for compatibility.**
   - Decision: continue writing and returning `input_excerpt` exactly as today; full body storage is additive.
   - Rationale: existing search, display, tests, and operational workflows depend on excerpts.

5. **Use current migration sequence at implementation time.**
   - Decision: create new PostgreSQL and MySQL migrations using the next available sequence numbers, not the stale numbers in the input plan.
   - Rationale: avoids checksum/order conflicts and matches current repository state.

## Risks / Trade-offs

- **Sensitive content may be persisted** → Apply sanitization before persistence, avoid saving non-flagged bodies, and restrict the download endpoint to existing admin risk-control routes.
- **Database growth may increase** → Enforce an application-level byte cap, expose stored size, and continue using existing retention cleanup to delete old logs and their bodies together.
- **List performance may degrade if body is selected accidentally** → Do not select the full body in `ListLogs`; select only `request_body IS NOT NULL` and byte/char length metadata.
- **Existing deployments may have migration ordering differences** → Use `ADD COLUMN IF NOT EXISTS` where supported/appropriate and MySQL information-schema guards consistent with current migrations.
- **Very large blocked sessions may be truncated** → Mark body size from stored content and document that download returns the stored bounded body, not necessarily unlimited original bytes.

## Session Boundary Semantics

A downloaded “session” is the exact conversation context carried by the blocked HTTP request, not a reconstructed cross-request conversation history.

- **Start**: the first protocol-specific conversational item in the request body (`messages[0]`, `input[0]`, or `contents[0]`), plus protocol-level `system` content when present.
- **End**: the last conversational item present in the same request body at the time risk control made the decision.
- **Trigger location**: store the audited text excerpt/hash and, where cheaply derivable, the last user-message index. For keyword blocks, the matched keyword can identify the content source more precisely. For external moderation API scores, exact character spans are usually unavailable, so the record must identify the full audited request/session and decision metadata rather than claim a token-level match position.
- **Limitation**: if a client sends only the latest message and does not include prior turns or a stable conversation/session ID, the backend cannot recover earlier turns. Supporting cross-request reconstruction would require a separate conversation/session capture feature keyed by a client-provided session identifier, which is out of scope for this change.
