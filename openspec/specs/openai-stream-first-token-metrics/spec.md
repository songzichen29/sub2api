# openai-stream-first-token-metrics Specification

## Purpose

Define consistent OpenAI streaming first-event and first-token metrics across HTTP streaming, OAuth passthrough, Responses WebSocket forwarding, and Responses WebSocket v2 relay.

## Requirements

### Requirement: Distinguish upstream first event from first token
The system SHALL record the first upstream OpenAI streaming event separately from the first effective output token or output progress event.

#### Scenario: Upstream preamble arrives before token
- **WHEN** an OpenAI stream emits `response.created` before any output delta
- **THEN** the system SHALL record `upstream_first_event_ms` for the preamble arrival and SHALL NOT set `first_token_ms` from that preamble

#### Scenario: Upstream emits no token before terminal event
- **WHEN** an OpenAI stream emits only preamble and terminal events without any effective output token event
- **THEN** the system SHALL leave `first_token_ms` unset and SHALL NOT use the terminal event time as a fallback first token

### Requirement: First token uses effective non-empty output events
The system SHALL set `first_token_ms` only when it observes the first non-empty OpenAI streaming event that represents output token content or effective output progress.

#### Scenario: Text delta starts first token
- **WHEN** the stream emits `response.output_text.delta` with a non-empty `delta`
- **THEN** the system SHALL set `first_token_ms` to the elapsed time at that upstream event

#### Scenario: Tool argument delta starts first token
- **WHEN** the stream emits `response.function_call_arguments.delta` with a non-empty `delta`
- **THEN** the system SHALL set `first_token_ms` to the elapsed time at that upstream event

#### Scenario: Reasoning summary delta starts first token
- **WHEN** the stream emits `response.reasoning_summary_text.delta` with a non-empty `delta`
- **THEN** the system SHALL set `first_token_ms` to the elapsed time at that upstream event

#### Scenario: Empty delta is ignored
- **WHEN** the stream emits a `*.delta` event whose `delta` value is empty
- **THEN** the system SHALL NOT set `first_token_ms` from that event

### Requirement: Non-token events do not start first token
The system SHALL NOT classify OpenAI preamble, lifecycle, terminal, debug, or empty events as first token events.

#### Scenario: Preamble events are excluded
- **WHEN** the stream emits `response.created` or `response.in_progress`
- **THEN** the system SHALL NOT set `first_token_ms` from those events

#### Scenario: Output item lifecycle events are excluded
- **WHEN** the stream emits `response.output_item.added` or `response.output_item.done` without an explicit non-empty output delta
- **THEN** the system SHALL NOT set `first_token_ms` from those events

#### Scenario: Terminal events are excluded
- **WHEN** the stream emits `response.completed`, `response.done`, `response.failed`, `response.incomplete`, `response.cancelled`, or `response.canceled`
- **THEN** the system SHALL NOT set `first_token_ms` from those events

#### Scenario: Debug timing comment is excluded
- **WHEN** debug timing emits an SSE comment containing upstream first event timing
- **THEN** the system SHALL NOT set or modify `first_token_ms` from that comment

### Requirement: First token measurement is independent of downstream write latency
The system SHALL record `first_token_ms` when the upstream effective token event is recognized, before downstream write, flush, proxy buffering, or client disconnect handling can add latency.

#### Scenario: Downstream writer is slow
- **WHEN** an effective token event arrives and the downstream writer or flush blocks afterward
- **THEN** `first_token_ms` SHALL reflect the upstream token recognition time rather than the delayed downstream write completion time

#### Scenario: Client disconnects after first token arrives
- **WHEN** an effective token event arrives and the client disconnects during or after downstream write
- **THEN** `first_token_ms` SHALL remain recorded from the upstream token event

### Requirement: First token semantics are consistent across OpenAI streaming transports
The system SHALL apply equivalent first token classification to normal HTTP streaming, OAuth passthrough streaming, Responses WebSocket forwarding, and Responses WebSocket v2 relay.

#### Scenario: HTTP and passthrough receive the same event sequence
- **WHEN** normal HTTP streaming and OAuth passthrough streaming process the same sequence of preamble, lifecycle, delta, and terminal events
- **THEN** both paths SHALL produce equivalent `first_token_ms` presence and classification behavior

#### Scenario: WebSocket relay receives terminal without delta
- **WHEN** Responses WebSocket forwarding or v2 relay receives a terminal event without any prior effective token event
- **THEN** the relay SHALL NOT set `first_token_ms` from that terminal event

