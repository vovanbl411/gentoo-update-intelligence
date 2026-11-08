## ADDED Requirements

### Requirement: Reuse clean pinned metadata checkouts
The worker MAY reuse a complete set of prepared pinned checkouts for distinct package requests when repository identity, pinned commits, ordered masters, requested profile, and runtime identity match. It SHALL perform existing repository and generated-cache lookup before acquiring a set. A set SHALL be leased to at most one request at a time, and package-specific generation data SHALL remain separate.

#### Scenario: Different packages share the same pinned set
- **GIVEN** two requests use the same repository, selected commit, ordered masters, profile, and runtime
- **WHEN** both require prepared checkouts for generation
- **THEN** the worker reuses the set sequentially and creates separate generation temporary directories

#### Scenario: Different pinned inputs do not share a set
- **WHEN** repository identity, a commit, ordered masters, profile, or runtime differs
- **THEN** the worker allocates a different set

#### Scenario: Existing cache hit
- **WHEN** repository metadata or generated metadata is found before checkout preparation
- **THEN** the worker returns the hit without leasing a checkout set

### Requirement: Restore and validate pooled checkouts
After each lease the worker SHALL restore every checkout to its pinned commit and remove untracked and ignored files. Before generation, existing clean-tree, `HEAD`, exact ebuild blob, and profile boundary checks SHALL still apply. A set that fails validation or restoration SHALL be discarded; if its data cannot be removed, new pool allocations SHALL stop and the cleanup error SHALL be reported.

#### Scenario: Generation changes checkout files
- **WHEN** generation modifies tracked or untracked files in a leased checkout
- **THEN** release restores the pinned clean state and removes those files before the next lease

#### Scenario: Restoration fails
- **WHEN** the worker cannot restore or remove a set
- **THEN** it does not lease that set again and reports the cleanup failure

### Requirement: Bound and own the checkout pool
The worker SHALL limit idle pool storage to 4 GiB and 16 sets using LRU eviction. It SHALL count allocated blocks without following symlinks. `METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES` SHALL default to `4294967296`; `0` SHALL disable reuse, and negative or invalid values SHALL prevent startup. The pool SHALL be a dedicated child of `METADATA_WORKER_TEMP_PATH` protected by an exclusive process lock. Startup SHALL clean stale contents only after acquiring ownership.

#### Scenario: Idle pool reaches a limit
- **WHEN** adding an idle set exceeds the byte or set limit
- **THEN** the least recently used idle sets are removed until both limits hold

#### Scenario: Pool disabled
- **WHEN** the byte limit is set to `0`
- **THEN** the worker creates and removes a checkout set for each request as before

#### Scenario: Another process owns the pool
- **WHEN** a second worker starts against the same pool directory while its lock is held
- **THEN** it refuses pool ownership without deleting the active worker's data

#### Scenario: Stale pool after process restart
- **WHEN** a worker acquires the pool lock after a prior process exited
- **THEN** it removes stale pool contents before serving requests
