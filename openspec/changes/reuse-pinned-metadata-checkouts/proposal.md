# Proposal

## Why

Cold metadata preparation repeatedly clones the same pinned repositories and masters for different packages in one analysis. A bounded pool of clean pinned checkouts can remove that repeated setup while keeping each request's generated files and provenance separate.

## What Changes

- Reuse a pinned main checkout and its ordered master checkouts after the existing repository and generated-cache lookups miss.
- Serialize use of each matching set, restore it to its pinned commits after use, and discard sets that cannot be verified or cleaned.
- Bound idle storage by bytes and entry count, with a configurable byte budget and process-exclusive ownership of the pool directory.
- Preserve generated-cache keys, result formats, and HTTP behavior.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `automatic-metadata-preparation`: define safe reuse, cleanup, size limits, and startup ownership for prepared checkouts.

## Impact

Changes affect metadata-worker checkout lifecycle, its configuration, Compose, tests, and preparation documentation. The pool remains private to one worker process and lives below `METADATA_WORKER_TEMP_PATH`.
