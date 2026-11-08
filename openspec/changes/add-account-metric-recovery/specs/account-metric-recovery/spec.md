# Spec Delta

## Purpose

Allows administrators to pause degraded accounts using recent request metrics and to recover them through bounded trial traffic. Unconfigured accounts retain the existing scheduling behavior.

## ADDED Requirements

### Requirement: Metric rules are opt-in and validated per account
The system SHALL keep metric-based pause and recovery disabled unless an administrator enables a valid configuration for that account.

#### Scenario: Unconfigured account
- **WHEN** an initialized instance has no enabled or paused monitor for an account in its membership snapshot
- **THEN** metric recovery SHALL NOT change its scheduling eligibility or query the metric database for request admission

#### Scenario: Local configuration is enabled
- **WHEN** an administrator enables valid metric rules through an instance
- **THEN** that instance SHALL publish protection before the persistence write and SHALL retain it if the write outcome is uncertain until a successful reconciliation

#### Scenario: Configuration is enabled on another instance
- **WHEN** another instance enables an account absent from this instance's membership snapshot
- **THEN** this instance SHALL discover that account on its next successful background refresh, including when Ops evaluation is disabled
- **AND** protection for that newly enabled account is eventually consistent; a failed refresh can extend the propagation delay beyond the normal 30-second cadence

#### Scenario: Invalid or stale configuration update
- **WHEN** an administrator saves invalid thresholds or a stale configuration revision
- **THEN** the system SHALL reject the update without changing the saved configuration or pause state

#### Scenario: Dual bounds and existing configuration
- **WHEN** an administrator configures a metric
- **THEN** the system SHALL support pause/recovery minimum and maximum bounds with per-metric sample counts, and omitted recovery bounds SHALL inherit pause bounds
- **AND** existing single-bound JSON and explicitly saved cooldown/sample values SHALL retain their meaning
- **AND** new defaults SHALL be a 300-second cooldown, 20 pause samples per metric and three recovery samples; the legacy 30-second minimum cooldown SHALL remain supported

#### Scenario: Ops monitoring is disabled
- **WHEN** Ops monitoring is disabled
- **THEN** enabling metric rules SHALL be rejected and automatic evaluation SHALL stop
- **AND** an administrator SHALL still be able to disable the rules or explicitly release a pause

### Requirement: Degraded rolling metrics pause only the affected account
The system SHALL pause an enabled account only when a configured pause threshold is breached using sufficient, valid observations from the rolling measurement window.

#### Scenario: Threshold breached with sufficient samples
- **WHEN** at least one configured metric breaches its pause threshold and meets the configured minimum sample count
- **THEN** the system SHALL mark that account unavailable to new requests and retain the measured reason and evidence

#### Scenario: Insufficient or unavailable measurements
- **WHEN** a configured metric has too few samples, invalid values, or unavailable telemetry
- **THEN** the system SHALL NOT infer a breach from that metric

#### Scenario: Failed usage placeholders and internal requests
- **WHEN** usage contains zero-cost failure placeholders, cyber-blocked requests, or channel-monitor probes
- **THEN** those rows SHALL NOT count as successful observations or suppress a corresponding terminal provider error
- **AND** internal probe and cyber-blocked errors SHALL NOT count as user-request failures

#### Scenario: Free successes and terminal partial failures
- **WHEN** metric statistics aggregate usage and provider errors
- **THEN** positive token usage SHALL count even when its cost is zero, terminal HTTP errors SHALL override partial-output usage, and duplicate request identities SHALL count once per source
- **AND** completed WebSocket turns SHALL remain observations even when their connection later fails
- **AND** cache/TTFT SHALL use eligible successful streaming/WebSocket observations, exclude image-input tokens from the cache denominator, and accept valid zero timings
- **AND** billing and dashboard success definitions SHALL remain unchanged

#### Scenario: Account is automatically paused
- **WHEN** metric rules pause an account
- **THEN** the system SHALL preserve the account's manual status and schedulable setting and SHALL NOT change other accounts or groups

### Requirement: Recovery uses bounded trial traffic
The system SHALL automatically recover a metric-paused account only after its cooldown and after bounded trial observations satisfy every configured recovery threshold.

#### Scenario: Trial succeeds
- **WHEN** the cooldown has elapsed and the required trial observations meet every recovery threshold within the trial window
- **THEN** the system SHALL clear the metric pause and record recovery evidence

#### Scenario: Automatic recovery is disabled
- **WHEN** an account has `auto_recover=false` and is metric-paused
- **THEN** the system SHALL admit no recovery trial and SHALL NOT recover it automatically
- **AND** a missing legacy `auto_recover` field SHALL retain the prior automatic behavior

#### Scenario: Evaluation finishes after its cycle changed
- **WHEN** a background evaluation's configuration revision, pause generation or trial start differs from the locked current monitor
- **THEN** that evaluation SHALL NOT pause, recover or end the newer cycle

#### Scenario: Trial fails or exhausts its budget
- **WHEN** a trial breaches a pause threshold, times out, or reaches its finite request budget without enough recovery evidence
- **THEN** the system SHALL keep the account paused and end that trial

#### Scenario: Trial admission is concurrent
- **WHEN** multiple gateway instances that have synchronized the account's opt-in membership concurrently request trial admission for the same paused account
- **THEN** the system SHALL enforce one shared rate limit and finite budget across those instances

#### Scenario: A reserved trial does not complete
- **WHEN** a request is rejected or cancelled after reserving trial admission
- **THEN** its trial reservation SHALL remain consumed; recovery still requires actual measured observations

### Requirement: Pause authority fails closed and changes are auditable
For accounts in an instance's enabled-or-paused membership snapshot, the system SHALL treat persisted metric-pause state as authoritative, fail closed when that state cannot be read, and retain an audit record for configuration, pause, recovery, and manual release actions. The separate membership propagation boundary applies to newly enabled accounts on other instances.

#### Scenario: Pause state cannot be checked
- **WHEN** a request for a known protected account cannot verify the persisted metric-pause state
- **THEN** the system SHALL NOT route that request through the affected account

#### Scenario: Membership cannot be refreshed
- **WHEN** a previously initialized instance cannot refresh monitor membership
- **THEN** it SHALL retain the previous snapshot and keep checking persisted pause state for known protected accounts
- **AND** it SHALL NOT treat a failed initial load as an empty snapshot or start accepting traffic without a successful initial load

#### Scenario: Administrator releases a paused account
- **WHEN** an administrator releases a metric-paused account using the current revision and pause generation
- **THEN** the system SHALL clear the pause, disable automatic evaluation for that configuration, and record the release

#### Scenario: Stale release request
- **WHEN** an administrator releases an account using an outdated revision or pause generation
- **THEN** the system SHALL reject the request without changing the current pause state

### Requirement: Account list metrics and configuration are visible and isolated
The system SHALL show recent metrics and the independent metric-rule state in the existing account list and provide a clickable configuration entry, including when no observations exist.

#### Scenario: Current page refresh
- **WHEN** the account list loads or refreshes
- **THEN** metrics SHALL be fetched in batches of no more than 100 IDs using the complete recent-hour window
- **AND** late responses from an old page SHALL NOT replace current-page data
- **AND** metric data SHALL refresh even if the account-list response is HTTP 304

#### Scenario: Configuration details and history
- **WHEN** the administrator opens the metric panel
- **THEN** it SHALL show the saved bounds, automatic recovery setting, per-metric samples, actual fresh evaluation window, pause reason, trial budget and paginated audit evidence
- **AND** a stale write SHALL require refresh while keeping the unsaved form visible
- **AND** manual release SHALL clearly state that it also disables rules

#### Scenario: Restricted admin key
- **WHEN** a scoped admin key attempts to read or change metrics through any metric-recovery route
- **THEN** the existing scoped-key middleware SHALL deny that request rather than expose unrestricted account data
