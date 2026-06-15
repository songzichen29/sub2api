## 1. Evidence and Current Behavior

- [x] 1.1 Re-run focused comparison against `upstream/main` for `openai_gateway_service.go`, `openai_ws_forwarder.go`, and `openai_ws_v2/passthrough_relay.go`; keep the decisive notes in the implementation summary.
- [x] 1.2 Identify all current first-token helper functions and call sites across HTTP streaming, OAuth passthrough, WS v1, and WS v2 relay.
- [x] 1.3 Confirm where `upstream_first_event_ms` and `first_token_ms` are written into ops context, scheduler reporting, and usage logs.

## 2. First Token Classification

- [x] 2.1 Add or refactor a shared OpenAI Responses first-token classifier that rejects preamble, lifecycle, terminal, debug, `[DONE]`, and empty-delta events.
- [x] 2.2 Ensure the classifier accepts non-empty effective output delta events, including text delta, function call arguments delta, reasoning summary delta, and unknown non-empty `*.delta` events.
- [x] 2.3 Decide and encode any explicit non-`.delta` effective-output whitelist only when payload evidence proves it represents real output progress.
- [x] 2.4 Update WS v1 `isOpenAIWSTokenEvent` and WS v2 relay `isTokenEvent` to match the shared semantics or prove equivalent behavior with shared table tests.

## 3. Streaming Path Integration

- [x] 3.1 Update normal HTTP streaming to record `firstTokenMs` immediately when the classifier recognizes the upstream token event, before downstream write/flush.
- [x] 3.2 Update OAuth passthrough streaming to use the same first-token classifier and preserve `upstreamFirstEventMs` behavior.
- [x] 3.3 Keep preamble force-release and debug timing comment behavior independent from `firstTokenMs`.
- [x] 3.4 Ensure client disconnect and stream read error paths return collected `firstTokenMs` and `upstreamFirstEventMs` without using terminal events as fallback.

## 4. Tests

- [x] 4.1 Add table-driven tests for the shared first-token classifier covering preamble, lifecycle, terminal, empty delta, text delta, function argument delta, reasoning delta, and event-line fallback.
- [x] 4.2 Add normal HTTP streaming tests for preamble-only force release, debug timing comment, terminal-only stream, event-line delta, and slow downstream writer.
- [x] 4.3 Add OAuth passthrough tests mirroring the HTTP streaming first-token cases.
- [x] 4.4 Add or adjust WS v1 and WS v2 relay tests so terminal events and output item lifecycle events do not set `firstTokenMs`, while non-empty delta events do.

## 5. Verification

- [x] 5.1 Run the focused Go tests for OpenAI gateway streaming and WebSocket relay packages.
- [x] 5.2 Run `go test` for the affected backend service package or a broader package set if focused tests expose shared helper impacts.
- [x] 5.3 Review logs/diagnostic fields in tests or code paths to verify `upstream_first_event_ms` remains separate from `first_token_ms`.
- [x] 5.4 Summarize behavior differences from fork source and explain expected impact on reported long first-token latency.

