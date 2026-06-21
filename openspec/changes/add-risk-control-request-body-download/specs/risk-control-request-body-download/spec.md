## ADDED Requirements

### Requirement: Flagged moderation request body persistence
The system SHALL persist a sanitized request session audit bundle for risk-control moderation log records when the moderation result is flagged and the original raw request body/session payload is available.

#### Scenario: Flagged request body is stored
- **WHEN** risk control evaluates a request with a non-empty raw request body and creates a flagged moderation log
- **THEN** the system SHALL store a sanitized bounded copy of the request body with that moderation log

#### Scenario: Non-flagged request body is not stored
- **WHEN** risk control evaluates a request that is not flagged
- **THEN** the system SHALL keep the existing `input_excerpt` behavior but SHALL NOT store the request session audit bundle

#### Scenario: Request body exceeds storage limit
- **WHEN** a flagged request body exceeds the configured implementation storage limit
- **THEN** the system SHALL store only the bounded sanitized body and SHALL NOT fail the moderation decision because of body size alone

### Requirement: Lightweight moderation log list metadata
The system SHALL keep the paginated risk-control log list lightweight by excluding full request bodies from list rows while exposing metadata for download availability.

#### Scenario: List row has downloadable body metadata
- **WHEN** an administrator lists risk-control logs
- **THEN** each row SHALL include whether a saved request session audit bundle exists and the stored request body size and session metadata

#### Scenario: List endpoint omits full body content
- **WHEN** an administrator lists risk-control logs containing records with saved request bodies
- **THEN** the list response SHALL NOT include the request session audit bundle content in each row

### Requirement: Admin request body download
The system SHALL provide an admin-only API for retrieving the saved request session audit bundle of a single risk-control moderation log.

#### Scenario: Download existing saved body
- **WHEN** an administrator requests the saved request session audit bundle for a moderation log that has one
- **THEN** the system SHALL return the stored sanitized request body and metadata sufficient for the frontend to download it as a file

#### Scenario: Download missing body
- **WHEN** an administrator requests the saved request session audit bundle for a moderation log that has no saved body
- **THEN** the system SHALL return a not-found or equivalent typed error without exposing unrelated log data

### Requirement: Risk Control UI download action
The Risk Control admin UI SHALL allow administrators to download a saved request session audit bundle from records where it is available.

#### Scenario: Download button appears for saved body
- **WHEN** the risk-control table or input detail view displays a log row with saved request session audit bundle metadata
- **THEN** the UI SHALL show a download action for that row

#### Scenario: Download file uses saved body
- **WHEN** an administrator clicks the download action for a row with a saved request session audit bundle
- **THEN** the UI SHALL fetch the single-log request body endpoint and trigger a browser download using the returned content

#### Scenario: Excerpt fallback remains available
- **WHEN** a row does not have a saved request session audit bundle
- **THEN** the UI SHALL continue to display the existing `input_excerpt` and MAY offer existing excerpt-only inspection behavior without pretending a full body is available

### Requirement: Request-local session boundary
The system SHALL define downloaded session boundaries from the blocked request payload itself.

#### Scenario: Chat request contains multiple messages
- **WHEN** a flagged request body contains a protocol-specific message array such as `messages`, `input`, or `contents`
- **THEN** the saved audit bundle SHALL identify the session start as the first item in that array and the session end as the last item in that same request payload

#### Scenario: Request includes system context
- **WHEN** a flagged request body contains protocol-level system instructions or system messages
- **THEN** the saved audit bundle SHALL include that system context before the conversational messages when generating the downloaded session file

#### Scenario: Client omits previous turns
- **WHEN** a flagged request contains only the latest user message and no previous turns
- **THEN** the system SHALL save only the session context present in that request and SHALL NOT imply that earlier omitted turns were captured
