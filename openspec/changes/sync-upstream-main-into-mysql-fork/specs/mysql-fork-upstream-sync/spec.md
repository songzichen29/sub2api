## ADDED Requirements

### Requirement: Sync branch must preserve MySQL database direction
The upstream synchronization process SHALL keep MySQL as the fork's authoritative runtime database target and SHALL NOT reintroduce PostgreSQL as an active runtime dependency.

#### Scenario: Upstream changes include PostgreSQL-specific database code
- **WHEN** an upstream change modifies migrations, repository SQL, database configuration, setup flow, deployment files, or database tests using PostgreSQL-specific behavior
- **THEN** the sync implementation SHALL adapt the change to MySQL-compatible behavior before accepting it into active runtime paths

#### Scenario: Conflict resolution touches database initialization
- **WHEN** merge conflicts involve database setup, DSN generation, Ent initialization, migration runner code, or backup/restore tooling
- **THEN** the resolved code SHALL continue using MySQL driver names, MySQL DSN syntax, MySQL migration behavior, and MySQL backup/restore tools

### Requirement: Upstream behavior must be triaged by risk area
The upstream synchronization process SHALL classify upstream changes and conflicts by risk area before final conflict resolution.

#### Scenario: Merge preview identifies conflicts
- **WHEN** a merge preview or real merge reports conflicting files
- **THEN** the files SHALL be grouped at minimum into generated code, database/migrations, repository SQL, backend service/handler behavior, frontend API/UI, and deploy/CI areas

#### Scenario: A conflict spans backend DTOs and frontend API types
- **WHEN** backend DTO or settings changes conflict with frontend API/type/view changes
- **THEN** the resolution SHALL be validated as one API contract group rather than independently accepting either side

### Requirement: Database migrations must be executable by the MySQL migration path
Active migration artifacts introduced or changed by the sync SHALL be executable by the fork's MySQL migration runner.

#### Scenario: Upstream adds a PostgreSQL migration
- **WHEN** upstream adds or modifies a migration that uses PostgreSQL-only syntax or numbering that conflicts with the fork's MySQL sequence
- **THEN** the sync implementation SHALL convert, renumber, or fold the migration intent into the MySQL baseline or MySQL increment sequence

#### Scenario: Migration runner is executed repeatedly
- **WHEN** the MySQL migration runner is executed against a database that has already applied the synchronized migrations
- **THEN** it SHALL complete idempotently without duplicate-index, duplicate-column, or checksum inconsistencies caused by the upstream sync

### Requirement: Generated code must be rebuilt from source definitions
The synchronization process SHALL treat generated wire and Ent outputs as derived artifacts.

#### Scenario: Generated files conflict during merge
- **WHEN** generated files such as `wire_gen.go` or `backend/ent/**` conflict
- **THEN** source definitions and schemas SHALL be resolved first and generated outputs SHALL be regenerated afterward

#### Scenario: Ent schema changes include both upstream fields and MySQL type definitions
- **WHEN** an upstream Ent schema change overlaps with fork MySQL type customization
- **THEN** the final schema SHALL include the upstream domain field behavior while preserving MySQL-compatible schema type definitions

### Requirement: PostgreSQL regressions must be scanned before completion
The synchronized tree SHALL be checked for PostgreSQL-only regressions in active runtime and deployment paths before the sync is considered complete.

#### Scenario: Conflict resolution is complete
- **WHEN** all merge conflicts are resolved and generated code has been rebuilt
- **THEN** the implementation SHALL run focused searches for PostgreSQL-specific drivers, dialects, DSN syntax, SQL operators, migration syntax, service names, and documentation that contradicts the MySQL target

#### Scenario: A PostgreSQL term remains after scanning
- **WHEN** a PostgreSQL-specific term remains in an active runtime or deployment path
- **THEN** the implementation SHALL either replace it with MySQL behavior or document why it is non-runtime historical/reference content

### Requirement: Verification gates must cover merge hotspots
The synchronized tree SHALL pass targeted verification gates for the conflict hotspots before the sync is declared ready.

#### Scenario: Backend conflict hotspots are resolved
- **WHEN** backend conflicts in gateway, OpenAI, content moderation, scheduler outbox, API key middleware, settings, repository, or migrations are resolved
- **THEN** targeted Go tests for those packages or features SHALL be run and failures SHALL be addressed or documented

#### Scenario: Frontend conflict hotspots are resolved
- **WHEN** frontend conflicts in admin account, risk control, settings, usage views, API clients, i18n, or shared types are resolved
- **THEN** frontend typecheck/build or focused tests SHALL be run and failures SHALL be addressed or documented

#### Scenario: Verification cannot be fully executed locally
- **WHEN** a required MySQL integration, Docker, or frontend verification gate cannot run in the local environment
- **THEN** the sync result SHALL document the missing gate, the reason it was skipped, and the residual risk
