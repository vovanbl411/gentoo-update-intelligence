# Proposal

## Why

Metadata preparation currently runs `emerge --version` once in the worker and again for each generated entry. Reusing one worker runtime removes repeated process startup and ensures generated-cache keys and provenance use the same Portage version.

## What Changes

- Keep one metadata generator for the lifetime of each worker process.
- Resolve Portage version lazily through the generator, share concurrent attempts, cache success, and permit retries after failure.
- Keep executable configuration fixed for each generator and use the same resolved version in cache keys and generated provenance.
- Preserve repository metadata hits that do not need Portage.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `automatic-metadata-preparation`: define worker runtime version reuse and cache/provenance consistency.

## Impact

Changes are limited to the Go metadata worker, metadata generator, tests, and preparation contract. HTTP request and response formats and generated-cache key format remain unchanged.
