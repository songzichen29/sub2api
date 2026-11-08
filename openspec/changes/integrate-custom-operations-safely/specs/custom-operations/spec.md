## ADDED Requirements

### Requirement: Explicit activation preserves existing behavior
The system SHALL retain existing routing, billing, quotas, monitor visibility,
account metric recovery, quality functionality and QQ rewards when new options
are unset or disabled.

#### Scenario: Upgrade with existing configuration
- **WHEN** an existing installation applies the additive migrations
- **THEN** no new group limits, fallback routes or QQ reward rules activate

### Requirement: Group capacity and fallback retain ownership
The system SHALL enforce user/global and user/group concurrency atomically.
Unavailable-account fallback SHALL retain the original billing identity and SHALL
NOT activate on saturation, infrastructure failures, unsafe replay or a recursive route.

#### Scenario: Group capacity is exhausted
- **WHEN** the user's group limit is reached while the global limit has capacity
- **THEN** existing wait/rejection behavior applies without leaked slots or borrowing another group

### Requirement: Production metric protection remains authoritative
The integration SHALL use the deployed account metric recovery implementation
and SHALL NOT create competing metric tables, services, configuration or UI.

#### Scenario: A manually disabled account has recovered metrics
- **WHEN** metric recovery succeeds later
- **THEN** manual disablement and all other existing restrictions remain in force

### Requirement: Diagnostics are additive and scoped
New TPS/recent-request fields SHALL retain group visibility and redaction.
Only admins SHALL receive detailed error diagnostics. Missing TPS measurements
SHALL remain unknown; previous health thresholds SHALL remain unchanged.

### Requirement: QQ media changes preserve business rules
Explicit GIF output SHALL preserve valid animation within bounded resources.
Default image handling, transport/runtime, reward distribution and disabled
invitation commands SHALL retain their behavior.

### Requirement: Model quality remains outside this change
No model quality detection, capability/metadata checks, visual assessments or
changes to existing quality plans, history, UI or scheduling SHALL be introduced.
