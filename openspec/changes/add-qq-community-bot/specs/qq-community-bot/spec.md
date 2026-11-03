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
The system SHALL resolve identity from the authenticated QQ event and current binding. It MUST expose only the caller's balance. Public channel reports and channel notifications MUST exclude exclusive groups.

#### Scenario: Attempt to query another account
- **WHEN** message text contains another QQ, user ID, nickname or claimed administrator role
- **THEN** the backend continues to use the sender's binding and never returns another user's balance

### Requirement: Bounded operator-funded conversation
The system SHALL send chat requests through the configured gateway using the operator's dedicated API Key, with daily member/site quotas, cooldown and output/history bounds. Group history SHALL retain speaker identity and exclude private commands. Clearing history MUST delete only the sender's history in the current group. Autonomous replies SHALL default to disabled. The model MUST NOT receive financial mutation tools.

#### Scenario: Chat limit reached
- **WHEN** a member or the site reaches its configured quota
- **THEN** no model request is sent and a clear limit reply is returned

#### Scenario: Frequent autonomous participation
- **WHEN** an operator configures five-second group and member cooldowns and full casual sampling
- **THEN** meaningful short questions and chat can reach the model without a mention, candidate pacing honors five seconds, and the model prioritizes natural replies

#### Scenario: New messages during an answer
- **WHEN** another group message arrives while a recent autonomous reply is being generated
- **THEN** the original answer remains eligible, references its source message, and still observes its age, current policy, cooldown and quota limits

### Requirement: Management, secret storage and records
The system SHALL provide administrator configuration, connection/model tests, binding/reward/message records and user binding management. Secrets MUST be encrypted at rest and omitted from configuration read responses.

#### Scenario: Save configuration without replacing secrets
- **WHEN** an administrator leaves an existing credential field empty
- **THEN** its stored value is retained and never returned as plaintext

### Requirement: Key model discovery and image generation
The system SHALL retrieve all nonempty model IDs available to the selected chat or image connection, without model-name filtering. Administrators MAY select a returned model or enter one manually. Changing an endpoint MUST require a replacement or explicit clearance of a saved credential before forwarding it. Image generation SHALL support generation, edits and referenced images through OneBot native image segments, and SHALL default to disabled.

#### Scenario: Discover models with saved credentials
- **WHEN** an administrator requests the selected connection's models
- **THEN** the backend uses that connection's credential and returns unique sorted IDs without returning the secret

#### Scenario: Image delivery confirmation
- **WHEN** a generated image is sent through OneBot
- **THEN** delivery is marked successful only after a successful receipt containing a message ID

### Requirement: Full community activities with disabled defaults
The system SHALL provide invitation rewards, transfers, lotteries, growth and periodic tasks, rankings, custom commands, site knowledge, group administration and automatic notifications. New activities, autonomous replies, moderation and notifications SHALL default to disabled; existing check-in settings SHALL be preserved.

#### Scenario: Confirm a transfer once
- **WHEN** concurrent confirmations use the same email token
- **THEN** at most one transfer commits, with sender and recipient balances and ledgers updated atomically

### Requirement: Restart-safe upgrade
The system SHALL migrate the legacy configuration, bindings, pending binding codes, check-in receipts and command audit in one transaction into independent qqc_ tables and settings. It MUST NOT replay past balance changes or tasks. Legacy private chat history MUST NOT be copied into shared group context.

#### Scenario: Restart after migration
- **WHEN** initialization runs after a committed migration
- **THEN** the configuration marker prevents repeated migration and balances remain unchanged

### Requirement: Optional contextual meme search
The system SHALL provide an administrator-controlled tool to search existing memes by a short topic or emotion. Searches SHALL use a fixed public source without sending site credentials or account identifiers. Returned images MUST come from the allowed image host, use public-IP-pinned downloads, satisfy size and format limits, and retain original GIF bytes. A meme turn SHALL use chat quota and SHALL NOT reserve image-generation quota. Failed searches SHALL return an honest text fallback.

#### Scenario: Search with image generation disabled
- **WHEN** meme search is enabled and the chat model requests an existing meme
- **THEN** the backend searches and sends an actual image through OneBot, even if image generation is disabled

### Requirement: Automatic offline login QR recovery
The system SHALL monitor configured NapCat login state without requiring an open admin browser. It SHALL request a QR code when an idle offline login has no code, refresh stale unscanned codes with backoff, and replace codes left over after logout. It MUST NOT refresh or restart an online, scanned, initializing or reconnecting login. An online QQ account with a disconnected OneBot transport SHALL be displayed separately and MUST NOT trigger QR regeneration. QR login still requires phone confirmation.

#### Scenario: Offline recovery and scan confirmation
- **WHEN** an online bot loses its login and NapCat finishes its own reconnect attempt
- **THEN** a fresh QR is requested and shown in the admin login panel, and subsequent phone confirmation is never interrupted by automatic refresh


### Requirement: Sampled autonomous image and sticker participation
The system SHALL recognize ordinary image, GIF, QQ face and market-sticker events without a mention when the media-interjection switch is enabled. It SHALL sample media independently at a default 10%, enforce a per-group daily recognition budget, and retain existing allowed groups, hours, cooldowns and chat/reply quotas. Explicit mentions and name calls SHALL retain the normal chat path. Autonomous media SHALL never invoke image generation.

#### Scenario: Unreadable or unverified visual input
- **WHEN** an image cannot be downloaded or decoded, its contents are unclear, or the chat model cannot return an image-only random verification value
- **THEN** the bot silently skips and records the reason instead of inventing image contents

#### Scenario: Existing meme response
- **WHEN** the model understands a sampled image or a reliably mapped QQ face and selects a contextual existing meme
- **THEN** the bot searches the existing trusted source and sends original image/GIF bytes with a confirmed receipt; failed search uses optional short text or silence without a repeated search-failure notice

#### Scenario: Limited animation evidence
- **WHEN** a GIF is analyzed
- **THEN** bounded decoding supplies a few keyframes or one static frame and explicitly limits the model's claims about the full animation

#### Scenario: Repeated stickers and concurrent media budgets
- **WHEN** the same stable sticker is resent or concurrent recognition attempts reach the daily group cap
- **THEN** persistent duplicate suppression and atomic quota reservations prevent extra recognition calls

### Requirement: Bounded queue for ongoing conversation
The system SHALL retain up to three autonomous jobs per group during generation and cooldown, serializing autonomous model work per group while preserving other groups and explicit command priority. Deferred jobs SHALL reserve no model quota until eligible and SHALL expire within the existing 45-second freshness bound.

#### Scenario: Follow-up arrives before the previous answer completes
- **WHEN** a new eligible question arrives while one autonomous reply is running
- **THEN** it remains queued, waits for applicable cooldown, and is not discarded solely because the prior answer advanced the message counter
