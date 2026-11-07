# Design

## Goals

- Resolve Portage version once after the first request that needs generated-cache lookup or generation.
- Have worker cache lookup and generator provenance share that value.
- Preserve retries after a failed lookup and isolate the command environment.

## Non-Goals

- Reusing prepared repository checkouts; that is a separate change.
- Changing generated-cache keys, `GeneratorVersion`, HTTP behavior, or result formats.

## Decisions

`Generator` owns the configured executable paths and a synchronized version resolver. Concurrent callers join the current attempt. Each attempt publishes its own result to its waiters; a failure is not cached, so a later caller can retry. A successful version remains cached for the generator lifetime.

The worker constructs one generator at startup, after which the process drops to the Portage user before serving requests. Version lookup runs with the generator's isolated environment and a temporary home directory. The generator's `Generate` method calls the same resolver, so its provenance agrees with the worker's generated-cache key.

## Risks / Trade-offs

The configured executable path is captured when the generator is created. Runtime replacement therefore takes effect after worker restart. Version lookup still needs a writable temporary directory even when no checkout is prepared.
