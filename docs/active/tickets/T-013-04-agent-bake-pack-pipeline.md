---
id: T-013-04
story: S-013
title: agent-bake-pack-pipeline
type: task
status: open
priority: high
phase: done
depends_on: [T-013-03]
---

## Context

A coding agent should be able to produce packs from source GLBs without human interaction. The agent calls `just bake-all` and gets packs in `dist/plants/` ready for USB transfer to the demo laptop.

This ticket is about the **agent experience**: clear error messages, non-interactive prompts, structured output that the agent can parse, and documentation that a coding agent can follow without human guidance.

## Acceptance Criteria

### Structured output

- `just bake <file>` and `just bake-all` accept a `--json` flag that emits machine-parseable JSON per asset:
  ```json
  {"source":"achillea.glb","species":"achillea_millefolium","pack":"dist/plants/achillea_millefolium.glb","size":1842311,"status":"ok"}
  ```
- Error output is also JSON-structured when `--json` is set:
  ```json
  {"source":"bad.glb","error":"bake timeout after 300s","step":"billboard","screenshot":"dist/bake-errors/bad_billboard.png"}
  ```

### Agent documentation

- New file `docs/agent-pack-workflow.md`:
  - Prerequisites: Go toolchain, Node.js, Playwright installed
  - Full worked example: "Given `inbox/dahlia_blush.glb`, produce a pack" (use the reference model already in the inbox)
  - Troubleshooting: common failure modes and their fixes
  - Verification: how to run `just verify-pack dahlia_blush` (T-012-03) after baking

### Non-interactive

- No prompts, no "press enter to continue", no browser window that needs manual interaction
- All file paths use the default working directory (`~/.glb-optimizer/`) unless overridden
- Species id derived automatically via the resolver chain (T-012-01) — no manual `_meta.json` authoring needed

## Out of Scope

- Running on CI (local agent execution only for now)
- Cross-repo deployment to plantastic (manual copy)
- Visual quality validation of the bake output (the agent trusts the pipeline)
- Blender-based baking (T-013 uses Playwright; Blender is a future story)
