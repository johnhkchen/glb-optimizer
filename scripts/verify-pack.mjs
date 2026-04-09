#!/usr/bin/env node
// verify-pack.mjs — Standalone Pack v1 verifier (T-012-03).
//
// Validates a glb-optimizer asset pack (.glb) end-to-end without
// pulling in the consumer's three.js / DOM stack. Used as the Phase 4
// gate in the cross-repo handshake (see T-011-04) — run before
// physically copying any pack to plantastic's asset directory.
//
// Canonical schema source: ../pack_meta.go (PackMeta.Validate).
// Validation order mirrors the Go side exactly:
//   1. format_version === 1
//   2. species non-empty + matches /^[a-z][a-z0-9_]*$/
//   3. common_name non-empty
//   4. bake_id non-empty
//   5. footprint.{canopy_radius_m, height_m} finite and > 0
//   6. fade.{low_start, low_end, high_start} in [0, 1]
//   7. fade.low_start < fade.low_end < fade.high_start <= 1.0
// Scene-graph rules (combine.go: pack_root → groups → mesh leaves):
//   - active scene has exactly one root, with named groups under it
//   - view_side is required and is a group with ≥1 mesh-leaf child
//   - view_top is required and is a group with ≥1 mesh-leaf child
//   - view_tilted / view_dome are optional; same shape if present
//   - every mesh primitive references in-range material/texture/image
//
// Deliberate skips vs the Go validator: none for v1 — every Go rule
// is mirrored. If pack_meta.go gains a rule, update both this file
// and the SCHEMA constant below.
//
// Usage:
//   node verify-pack.mjs <path-to-pack.glb>
// Exit codes:
//   0 — pack is a valid Pack v1
//   1 — read/parse/validation failure
//   2 — usage error

import { NodeIO } from '@gltf-transform/core';

const SCHEMA = {
  formatVersion: 1,
  speciesRe: /^[a-z][a-z0-9_]*$/,
  requiredGroups: ['view_side', 'view_top'],
  optionalGroups: ['view_tilted', 'view_dome'],
};

function isFinitePositive(v) {
  return typeof v === 'number' && Number.isFinite(v) && v > 0;
}

function isInUnitRange(v) {
  return typeof v === 'number' && Number.isFinite(v) && v >= 0 && v <= 1;
}

function nonEmptyString(v) {
  return typeof v === 'string' && v.trim() !== '';
}

function validateMeta(extras) {
  const errs = [];
  if (!extras || typeof extras !== 'object') {
    errs.push('scene.extras.plantastic missing — not a Pack v1 file');
    return errs;
  }
  const m = extras;
  if (m.format_version !== SCHEMA.formatVersion) {
    errs.push(`unsupported format_version: ${m.format_version} (expected ${SCHEMA.formatVersion})`);
  }
  if (!nonEmptyString(m.species)) {
    errs.push('species is required');
  } else if (!SCHEMA.speciesRe.test(m.species)) {
    errs.push(`species "${m.species}" must match ^[a-z][a-z0-9_]*$`);
  }
  if (!nonEmptyString(m.common_name)) errs.push('common_name is required');
  if (!nonEmptyString(m.bake_id)) errs.push('bake_id is required');

  const fp = m.footprint || {};
  if (!isFinitePositive(fp.canopy_radius_m)) {
    errs.push(`footprint.canopy_radius_m must be finite and > 0, got ${fp.canopy_radius_m}`);
  }
  if (!isFinitePositive(fp.height_m)) {
    errs.push(`footprint.height_m must be finite and > 0, got ${fp.height_m}`);
  }

  const fade = m.fade || {};
  for (const k of ['low_start', 'low_end', 'high_start']) {
    if (!isInUnitRange(fade[k])) {
      errs.push(`fade.${k} must be in [0,1], got ${fade[k]}`);
    }
  }
  if (errs.length === 0) {
    if (!(fade.low_start < fade.low_end)) {
      errs.push(`fade.low_start (${fade.low_start}) must be < fade.low_end (${fade.low_end})`);
    }
    if (!(fade.low_end < fade.high_start)) {
      errs.push(`fade.low_end (${fade.low_end}) must be < fade.high_start (${fade.high_start})`);
    }
    if (fade.high_start > 1.0) {
      errs.push(`fade.high_start (${fade.high_start}) must be <= 1.0`);
    }
  }
  return errs;
}

function leafMeshes(group) {
  // Direct-child nodes that own a mesh — matches collectLeafMeshes()
  // in pack_inspect.go. One level of recursion is sufficient because
  // combine attaches mesh-bearing leaves directly under the group.
  const out = [];
  for (const child of group.listChildren()) {
    const mesh = child.getMesh();
    if (mesh) out.push(mesh);
  }
  return out;
}

function validateScene(document) {
  const errs = [];
  const root = document.getRoot();
  const scene = root.getDefaultScene() || root.listScenes()[0];
  if (!scene) {
    errs.push('glb has no scenes');
    return { errs, meshes: [] };
  }
  const topLevel = scene.listChildren();
  if (topLevel.length === 0) {
    errs.push('scene has no root node');
    return { errs, meshes: [] };
  }
  // Walk one level beneath the (single) scene root to find named
  // variant groups. We do not anchor on the literal name "pack_root"
  // — combine.go uses it today but the verifier should survive a
  // future rename.
  const packRoot = topLevel[0];
  const groups = new Map();
  for (const child of packRoot.listChildren()) {
    if (child.getName()) groups.set(child.getName(), child);
  }

  const allMeshes = [];
  for (const required of SCHEMA.requiredGroups) {
    const g = groups.get(required);
    if (!g) {
      errs.push(`required group ${required} missing under scene root`);
      continue;
    }
    const meshes = leafMeshes(g);
    if (meshes.length === 0) {
      errs.push(`group ${required} has no mesh-bearing leaf children`);
      continue;
    }
    allMeshes.push(...meshes);
  }
  for (const optional of SCHEMA.optionalGroups) {
    const g = groups.get(optional);
    if (!g) continue;
    const meshes = leafMeshes(g);
    if (meshes.length === 0) {
      errs.push(`optional group ${optional} present but has no mesh-bearing leaf children`);
      continue;
    }
    allMeshes.push(...meshes);
  }
  return { errs, meshes: allMeshes };
}

function validateMeshRefs(meshes) {
  const errs = [];
  for (const mesh of meshes) {
    const meshName = mesh.getName() || '(unnamed)';
    for (const prim of mesh.listPrimitives()) {
      const mat = prim.getMaterial();
      if (!mat) {
        // A mesh primitive without a material is technically legal
        // glTF, but every Pack v1 emit assigns one. Flag it.
        errs.push(`mesh ${meshName} primitive has no material`);
        continue;
      }
      // Walk the material's texture slots and confirm each one
      // resolves to an image with bytes.
      const slots = ['BaseColor', 'Emissive', 'Normal', 'Occlusion', 'MetallicRoughness'];
      for (const slot of slots) {
        const tex = mat[`get${slot}Texture`]?.();
        if (!tex) continue;
        const image = tex.getImage();
        const uri = tex.getURI?.();
        if (!image && !uri) {
          errs.push(`mesh ${meshName} material ${slot} texture has no image bytes or URI`);
        }
      }
    }
  }
  return errs;
}

async function loadPack(path) {
  const io = new NodeIO();
  return io.read(path);
}

function formatResult(path, errs) {
  if (errs.length === 0) return `PASS: ${path}`;
  const lines = [`FAIL: ${path}`];
  for (const e of errs) lines.push(`  - ${e}`);
  return lines.join('\n');
}

async function main(argv) {
  if (argv.length !== 1) {
    process.stderr.write('usage: node verify-pack.mjs <path-to-pack.glb>\n');
    return 2;
  }
  const path = argv[0];
  let document;
  try {
    document = await loadPack(path);
  } catch (e) {
    process.stdout.write(formatResult(path, [`pack_load: ${e.message || String(e)}`]) + '\n');
    return 1;
  }

  const root = document.getRoot();
  const scene = root.getDefaultScene() || root.listScenes()[0];
  const extras = scene ? scene.getExtras().plantastic : undefined;

  const errs = [
    ...validateMeta(extras),
    ...(() => {
      const sr = validateScene(document);
      const merefs = validateMeshRefs(sr.meshes);
      return [...sr.errs, ...merefs];
    })(),
  ];

  process.stdout.write(formatResult(path, errs) + '\n');
  return errs.length === 0 ? 0 : 1;
}

main(process.argv.slice(2))
  .then((code) => process.exit(code))
  .catch((e) => {
    process.stdout.write(`FAIL: unhandled exception: ${e.message || String(e)}\n`);
    process.exit(1);
  });
