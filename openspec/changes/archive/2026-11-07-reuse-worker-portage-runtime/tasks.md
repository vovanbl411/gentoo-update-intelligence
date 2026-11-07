# Tasks

## 1. Shared runtime identity

- [x] 1.1 Keep one generator per worker and route cache-key lookup and generation through it; verify the worker and generator use the same resolved Portage version.
- [x] 1.2 Add concurrency, retry-after-failure, and new-generator tests for Portage version resolution; verify with package tests and race detector.
- [x] 1.3 Run backend tests, vet, worker/backend builds, and record the phase-one performance comparison and result compatibility evidence.
- [x] 1.4 Update the metadata preparation guide and backlog/VPS experiment record; verify documented behavior against the implementation.

## Workflow follow-up

- Archive the change after review and successful strict validation.
- Run both strict OpenSpec validations after archive.
