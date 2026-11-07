## ADDED Requirements

### Requirement: Reuse the worker Portage runtime identity
The metadata worker SHALL resolve the configured Portage version lazily on the first request that needs generated metadata or generated-cache lookup, then reuse that successful version for the lifetime of the worker. Its generated-cache key and generated provenance SHALL contain the same version. Repository metadata found directly in the pinned revision SHALL remain available without resolving Portage.

#### Scenario: Concurrent first lookup
- **WHEN** multiple requests require the Portage version before a successful lookup exists
- **THEN** they share one `emerge --version` invocation and use its version in cache identity and provenance

#### Scenario: Failed lookup is retried
- **WHEN** one version lookup fails and a later request again needs the version
- **THEN** the later request starts a new lookup

### Requirement: Share concurrent Portage version lookups
Concurrent requests that need an unresolved Portage version SHALL share one lookup attempt. If that attempt fails, all waiting requests SHALL receive its failure and a later request MAY retry.

#### Scenario: Concurrent failed lookup
- **WHEN** multiple requests wait on a Portage version lookup that fails
- **THEN** all waiting requests receive the failure and a later request can start a new attempt

#### Scenario: Repository metadata hit
- **WHEN** the exact metadata entry exists in the selected pinned repository revision
- **THEN** the worker returns it without starting Portage

#### Scenario: Worker restart observes runtime update
- **WHEN** a new worker process starts with a changed Portage executable or runtime
- **THEN** it resolves and uses the new version for subsequent generated-cache keys and provenance
