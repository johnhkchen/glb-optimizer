---
id: T-011-03
story: S-011
title: bake-id-embedding
type: task
status: open
priority: low
phase: done
depends_on: [T-011-02]
---

## Context

`bake_id` is a forward-looking hook for the future asset server. The server will eventually serve packs at URLs like `/{species}/{bake_id}.glb` and rely on cache headers + immutable URLs for invalidation. For the demo it's just a string in the metadata that's written and read but not used for routing.

This ticket is small but explicit because the field is easy to misuse — it MUST be a stable identifier per bake, not a wallclock that changes between combine runs of the same intermediates.

## Acceptance Criteria

- `bake_id` is set when the **bake** completes, NOT when combine runs
- Specifically: when "Build hybrid impostor" finishes, the bake driver writes `outputs/{id}_bake.json` with `{ "bake_id": "<RFC3339 UTC>", "completed_at": "..." }`
- T-011-02's `BuildPackMetaFromBake` reads `bake_id` from this file rather than generating a new timestamp
- If `outputs/{id}_bake.json` is absent (e.g., bake ran before this ticket), `BuildPackMetaFromBake` falls back to the current time and logs a warning
- Unit test: combining the same intermediates twice produces the same `bake_id` in the output pack

## Out of Scope

- Hash-based bake ids (timestamp is fine for demo)
- Bake provenance metadata beyond `bake_id` and `completed_at`
- Migration of pre-existing intermediates to backfill bake_id

## Notes

- This is the cheapest ticket in the epic but the easiest to get subtly wrong. If two consecutive packs of the same bake have different `bake_id`s, future cache-busting logic will treat them as different assets and the asset server will serve duplicate copies. The fix is "set it once at bake, read it forever after."
