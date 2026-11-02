## ADDED Requirements

### Requirement: Authenticated QQ connection and group scope
The system SHALL accept OneBot v11 group events only from a connection authenticated with the configured token and robot QQ. It MUST ignore non-whitelisted groups, other bots, unsupported events and duplicate messages.

#### Scenario: Invalid connection
- **WHEN** a client supplies an invalid token or the feature is disabled
- **THEN** no event is accepted and no account or reward is changed

#### Scenario: Replayed event
- **WHEN** the same message is delivered again after reconnect
- **THEN** it cannot repeat binding, reward or model generation

### Requirement: Website-confirmed unique account binding
The system SHALL provide an authenticated expiring binding-code workflow. A QQ claim MUST be confirmed by the originating website user before binding. Both QQ and website user MUST have at most one active binding.

#### Scenario: Successful binding
- **WHEN** a logged-in user generates a code, sends it from QQ in an allowed group, and confirms the displayed QQ on the website
- **THEN** future QQ commands resolve to that user without exposing their password or API Key

#### Scenario: Invalid or expired claim
- **WHEN** a code is expired, invalid, reused or submitted for an already-bound QQ
- **THEN** the system rejects the claim without revealing the owner's identity

### Requirement: Atomic daily check-in rewards
The system SHALL credit a configured positive reward to an active bound user at most once per Asia/Shanghai date across all QQ accounts and groups. Reward record and balance increment MUST commit together, with cache invalidation after commit.

#### Scenario: Concurrent duplicate check-in
- **WHEN** concurrent requests target the same user and day
- **THEN** exactly one reward is credited and the remaining requests return already checked in

#### Scenario: Random reward and repeated claim
- **WHEN** a positive minimum and a maximum at least as large are configured
- **THEN** the credited amount is chosen within that inclusive range and persisted with the balance update; repeated claims return the original amount without drawing or crediting another reward

#### Scenario: Disabled or failed award
- **WHEN** rewards are disabled or any write in the transaction fails
- **THEN** the user's balance is unchanged and no successful reward is recorded

### Requirement: Authorized account and channel queries
The system SHALL resolve identity from the authenticated QQ event and current binding. It MUST expose only the caller's balance and channel metrics within that user's currently available groups.

#### Scenario: Attempt to query another account
- **WHEN** message text contains another QQ, user ID, nickname or claimed administrator role
- **THEN** the backend continues to use the sender's binding and never returns another user's balance

### Requirement: Bounded operator-funded conversation
The system SHALL send explicit chat requests through the existing Sub2API gateway using the operator's dedicated API Key, with per-member/group history isolation, daily member/site quotas, cooldown and output/history bounds. It MUST NOT grant the model financial mutation tools.

#### Scenario: Chat limit reached
- **WHEN** a member or the site reaches its configured quota
- **THEN** no model request is sent and a clear limit reply is returned

### Requirement: Management, secret storage and records
The system SHALL provide administrator configuration, connection/model tests, binding/reward/message records and user binding management. Secrets MUST be encrypted at rest and omitted from configuration read responses.

#### Scenario: Save configuration without replacing secrets
- **WHEN** an administrator leaves an existing credential field empty
- **THEN** its stored value is retained and never returned as plaintext
