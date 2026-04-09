---
id: S-013
epic: E-002
title: Automated Pack Generation — Agent-Driven Bake-to-Pack Pipeline
status: open
priority: critical
tickets: [T-013-01, T-013-02, T-013-03, T-013-04]
---

## Goal

An agent (or human operator) should be able to go from "here's a source .glb" to "here's a ready-to-deploy Pack v1 .glb" via a single CLI command. Today the bake step (billboard + tilted + volumetric rendering) requires a human to open the browser UI, upload, click buttons, and wait. The pack step (combine intermediates) is already CLI-driven (`just pack <id>`). The gap is the bake.

## Why now

The demo ran but we couldn't clone the repo to a laptop over a 50kbps link. The next demo will need to run offline from a USB-transferred bundle (plantastic S-086). That bundle needs packs. If adding a new species requires a human to sit in front of the browser UI for 5 minutes per asset, scaling to 10+ species becomes a bottleneck. An automated pipeline lets an agent bake a batch overnight.

## Architecture options

The bake (billboard/tilted/volumetric rendering) currently runs in the BROWSER because it's client-side three.js code. Three paths to headless:

| Option | Effort | Quality | Notes |
|---|---|---|---|
| **Playwright driving the browser UI** | Low | Same as manual | Automates exactly what a human does. Requires a headed or headless Chromium. Works with existing code. |
| **Blender Python script** | Medium | Potentially better | Blender is already detected by the server. Can render orthographic billboard images at any resolution. Needs a new script but Blender's Python API is well-documented. |
| **Node + headless-gl** | High | Same | Port the three.js render code to node. Requires headless-gl native addon, fragile on macOS. |

**Recommended: Playwright first (fast win), Blender later (production quality).** Playwright automates the existing UI with 100 LOC of test-like code. Blender is the long-term path for production bakes (better quality, scriptable, no browser dependency) but is a larger investment.

## Reference model

`inbox/dahlia_blush.glb` (28 MB) — a TRELLIS-generated dahlia, confirmed by the user as a suitable test model. Use this for smoke tests across T-013-01..04. The `inbox/` directory at the repo root is the standard drop location for source GLBs awaiting bake.

## Tickets

| Ticket | Title | Depends on |
|---|---|---|
| T-013-01 | Playwright headless bake script | — |
| T-013-02 | End-to-end `just bake <source.glb>` recipe | T-013-01 |
| T-013-03 | Batch bake recipe for multiple species | T-013-02 |
| T-013-04 | Agent-callable bake+pack pipeline | T-013-03 |
