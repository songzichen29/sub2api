## ADDED Requirements

### Requirement: Ordered credentials with return to primary
An API-key account SHALL support an ordered list of keys for the same upstream.
Each independent request SHALL prefer the earliest usable key.

#### Scenario: Primary exhausted and then recovered
- GIVEN keys A, B and C in order
- WHEN A returns a definitive credential/quota rejection and B succeeds
- THEN that request succeeds through B without changing the logical account
- AND a later independent request tries A first and uses A if it succeeds.

### Requirement: Bounded safe retry
Key failover SHALL occur only before response delivery and SHALL preserve bodies,
headers, proxy routing and response ownership. Successful streams SHALL never be
replayed. Non-replayable requests and stateful continuations SHALL fail safely.

#### Scenario: Every key is rejected
- WHEN every eligible key is rejected
- THEN the request fails through existing error mapping
- AND credential exhaustion alone does not permanently disable the account.

### Requirement: Secret-safe administration
Stored keys SHALL be absent from ordinary account DTOs and audit log values.
Unrelated account updates SHALL preserve all stored keys in their original order.

#### Scenario: Edit a name without entering new keys
- WHEN the administrator changes only account metadata
- THEN the stored ordered keys remain unchanged.

### Requirement: Create/edit configuration parity
Applicable editable configuration SHALL also be available during creation, with
equivalent eligibility, validation, defaults and persistence semantics. Data or
actions that depend on an existing account MAY remain edit-only.
