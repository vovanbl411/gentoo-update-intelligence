# Design

## Goals

- Reuse the complete pinned repository set for different packages with the same repository, commits, profile, and runtime identity.
- Keep each checkout exclusively leased to one request and each generation temp directory private to that request.
- Bound idle pool disk use and recover only the worker-owned pool after restart.

## Non-Goals

- Reusing checkouts across processes or worker restarts.
- Changing repository resolution, generated-cache identity, provenance, HTTP contracts, or the existing preparation and generation concurrency limits.

## Decisions

The pool key includes repository and ordered master names, URLs and mirror paths; the selected repository commit; ordered master commits; requested profile; and generator/runtime identity. Package and version are excluded. Existing repository and generated-cache checks run before a checkout lease is requested.

Each entry owns the main checkout and ordered master checkouts. A lease serializes use of an entry. Release restores all trees with `git reset --hard` to their pinned commits followed by `git clean -ffdx`; validation still checks clean status, `HEAD`, ebuild blob, and profile boundaries. Generation output stays in a per-request directory outside those trees.

Idle entries use LRU eviction with limits of 4 GiB and 16 sets. Size accounting uses allocated blocks and does not follow symlinks. An entry larger than the byte budget is usable for its current request and removed after release. A lock file inside the dedicated pool directory prevents another process from owning it; after acquiring the lock, startup removes only contents managed by this pool. Cleanup failure disables new pool allocations while cache hits remain available.

`METADATA_WORKER_CHECKOUT_CACHE_MAX_BYTES` defaults to `4294967296`; `0` disables reuse, and invalid or negative values prevent startup. Pool creation, reuse, eviction, and cleanup errors are logged locally.

## Risks / Trade-offs

Restoration adds Git cleanup work to each lease and may reduce the cold-path savings. Locking and recovery must fail closed so a dirty checkout is never passed to `egencache`. Storage accounting and removal are limited to the dedicated pool subtree.
