#!/usr/bin/env node
// build-fixtures.mjs — One-shot builder for verify-pack test fixtures.
//
// RUN ONCE — outputs are committed to scripts/fixtures/. Re-run only
// if the Pack v1 schema (pack_meta.go) changes shape. The verifier's
// shell tests consume these binary blobs directly.
//
// Produces:
//   scripts/fixtures/valid-pack.glb            (passes verify-pack)
//   scripts/fixtures/broken-pack-no-top.glb    (missing view_top)
//
// Usage:
//   node scripts/build-fixtures.mjs

import { NodeIO, Document } from '@gltf-transform/core';
import { mkdir } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const fixturesDir = join(here, 'fixtures');

const VALID_META = {
  format_version: 1,
  bake_id: 'fixturebake0001',
  species: 'fixture_plant',
  common_name: 'Fixture Plant',
  footprint: { canopy_radius_m: 0.5, height_m: 1.2 },
  fade: { low_start: 0.1, low_end: 0.4, high_start: 0.8 },
};

function buildPack({ includeViewTop }) {
  const doc = new Document();
  const buf = doc.createBuffer();

  // One material so validateMeshRefs has something to chew on. No
  // texture slots populated — Pack v1 textures are tested separately.
  const mat = doc.createMaterial('fixture_mat').setBaseColorFactor([0.4, 0.6, 0.3, 1]);

  // One tiny triangle mesh re-used by both variant leaves.
  const positions = doc
    .createAccessor()
    .setType('VEC3')
    .setArray(new Float32Array([0, 0, 0, 1, 0, 0, 0, 1, 0]))
    .setBuffer(buf);
  const prim = doc.createPrimitive().setAttribute('POSITION', positions).setMaterial(mat);
  const mesh = doc.createMesh('fixture_quad').addPrimitive(prim);

  const variantLeaf = doc.createNode('variant_0').setMesh(mesh);
  const sideGroup = doc.createNode('view_side').addChild(variantLeaf);

  const packRoot = doc.createNode('pack_root').addChild(sideGroup);

  if (includeViewTop) {
    const topLeaf = doc.createNode('top_quad').setMesh(mesh);
    const topGroup = doc.createNode('view_top').addChild(topLeaf);
    packRoot.addChild(topGroup);
  }

  const scene = doc.createScene('pack_scene').addChild(packRoot);
  doc.getRoot().setDefaultScene(scene);
  scene.setExtras({ plantastic: VALID_META });

  return doc;
}

async function main() {
  await mkdir(fixturesDir, { recursive: true });
  const io = new NodeIO();
  await io.write(join(fixturesDir, 'valid-pack.glb'), buildPack({ includeViewTop: true }));
  await io.write(join(fixturesDir, 'broken-pack-no-top.glb'), buildPack({ includeViewTop: false }));
  process.stdout.write('wrote scripts/fixtures/{valid-pack,broken-pack-no-top}.glb\n');
}

main().catch((e) => {
  process.stderr.write(`build-fixtures failed: ${e.stack || e.message || String(e)}\n`);
  process.exit(1);
});
