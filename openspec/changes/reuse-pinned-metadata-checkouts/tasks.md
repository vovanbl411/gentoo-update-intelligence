# Tasks

## 1. Pool ownership and configuration

- [x] 1.1 Validate `METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES` and create a process-exclusive pool owner; verify defaults, zero, invalid values, stale cleanup, and a competing owner.
- [x] 1.2 Add keying, leasing, idle LRU limits, allocated-block accounting, and disable mode; verify distinct identities, 4 GiB/16-set bounds, and oversized set removal.

## 2. Checkout lifecycle

- [x] 2.1 Route only post-cache-miss requests through the pool; verify same-key packages reuse a set and generated-cache hits bypass it.
- [x] 2.2 Restore and validate main and ordered master trees at release; verify tracked, untracked, and ignored changes are removed and cleanup failures block allocation.
- [x] 2.3 Keep generation temp directories request-local and make pool leases wait with request cancellation; verify cancellation releases only that waiter's interest.

## 3. Integration and acceptance

- [x] 3.1 Add compact pool lifecycle logs and update Compose, configuration, metadata preparation, backlog, and VPS records; verify the documented defaults and runtime behavior.
- [x] 3.2 Run Go tests, vet, backend and worker builds, race tests, and sequential real `egencache` generations for distinct packages in one reused set.
- [ ] 3.3 Run the phase-two cold/warm and control matrix, verify reports and metrics, and save the report, reproduction steps, and artifact hashes.

## Workflow follow-up

- Archive only after the implementation and performance acceptance checks pass.
- Run both strict OpenSpec validations after archive.
