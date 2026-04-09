#!/usr/bin/env node
// inspect-intermediate.mjs — Structural inspection of GLB intermediate files.
//
// Loads a GLB (billboard, tilted, or volumetric intermediate) and emits a
// JSON report with mesh names, vertex/index counts, texture dimensions,
// triangle areas, and dominant normal axes. Used by validate-blender-output.sh
// to automate checks 2-4 from T-014-06.
//
// Usage:
//   node scripts/inspect-intermediate.mjs <path.glb>
// Exit codes:
//   0 — inspection succeeded (JSON on stdout)
//   1 — read/parse failure
//   2 — usage error

import { NodeIO } from '@gltf-transform/core';

function vec3Cross(a, b) {
  return [
    a[1] * b[2] - a[2] * b[1],
    a[2] * b[0] - a[0] * b[2],
    a[0] * b[1] - a[1] * b[0],
  ];
}

function vec3Length(v) {
  return Math.sqrt(v[0] * v[0] + v[1] * v[1] + v[2] * v[2]);
}

function vec3Sub(a, b) {
  return [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
}

// Compute total triangle area and average face normal from position + index data.
function computeGeometryStats(positionAccessor, indexAccessor) {
  const posArray = positionAccessor.getArray();
  const indexArray = indexAccessor ? indexAccessor.getArray() : null;

  let totalArea = 0;
  const normalSum = [0, 0, 0];

  const triCount = indexArray
    ? Math.floor(indexArray.length / 3)
    : Math.floor(posArray.length / 9);

  for (let t = 0; t < triCount; t++) {
    let i0, i1, i2;
    if (indexArray) {
      i0 = indexArray[t * 3];
      i1 = indexArray[t * 3 + 1];
      i2 = indexArray[t * 3 + 2];
    } else {
      i0 = t * 3;
      i1 = t * 3 + 1;
      i2 = t * 3 + 2;
    }

    const p0 = [posArray[i0 * 3], posArray[i0 * 3 + 1], posArray[i0 * 3 + 2]];
    const p1 = [posArray[i1 * 3], posArray[i1 * 3 + 1], posArray[i1 * 3 + 2]];
    const p2 = [posArray[i2 * 3], posArray[i2 * 3 + 1], posArray[i2 * 3 + 2]];

    const e1 = vec3Sub(p1, p0);
    const e2 = vec3Sub(p2, p0);
    const cross = vec3Cross(e1, e2);
    const area = vec3Length(cross) * 0.5;
    totalArea += area;

    normalSum[0] += cross[0];
    normalSum[1] += cross[1];
    normalSum[2] += cross[2];
  }

  // Dominant normal axis
  const absN = [Math.abs(normalSum[0]), Math.abs(normalSum[1]), Math.abs(normalSum[2])];
  let dominantAxis = 'x';
  if (absN[1] >= absN[0] && absN[1] >= absN[2]) dominantAxis = 'y';
  else if (absN[2] >= absN[0] && absN[2] >= absN[1]) dominantAxis = 'z';

  // Min Y from positions
  let minY = Infinity;
  for (let i = 1; i < posArray.length; i += 3) {
    if (posArray[i] < minY) minY = posArray[i];
  }

  return { area: totalArea, dominant_normal_axis: dominantAxis, min_y: minY };
}

// Extract texture dimensions from a material's baseColorTexture.
function getTextureDims(material) {
  if (!material) return { has_texture: false, texture_width: 0, texture_height: 0 };

  const tex = material.getBaseColorTexture();
  if (!tex) return { has_texture: false, texture_width: 0, texture_height: 0 };

  const image = tex.getImage();
  if (!image || image.byteLength === 0) {
    return { has_texture: true, texture_width: 0, texture_height: 0 };
  }

  const size = tex.getSize();
  if (size) {
    return { has_texture: true, texture_width: size[0], texture_height: size[1] };
  }

  // Fallback: parse PNG header for dimensions
  const view = new DataView(image.buffer, image.byteOffset, image.byteLength);
  if (image.byteLength >= 24 && view.getUint8(1) === 0x50 && view.getUint8(2) === 0x4e && view.getUint8(3) === 0x47) {
    const w = view.getUint32(16, false);
    const h = view.getUint32(20, false);
    return { has_texture: true, texture_width: w, texture_height: h };
  }

  return { has_texture: true, texture_width: 0, texture_height: 0 };
}

async function main(argv) {
  if (argv.length !== 1) {
    process.stderr.write('usage: node inspect-intermediate.mjs <path.glb>\n');
    return 2;
  }

  const path = argv[0];
  let document;
  try {
    const io = new NodeIO();
    document = await io.read(path);
  } catch (e) {
    process.stderr.write(`error: failed to read ${path}: ${e.message || String(e)}\n`);
    return 1;
  }

  const root = document.getRoot();
  const meshes = root.listMeshes();
  const report = {
    path,
    size_bytes: 0,
    mesh_count: meshes.length,
    meshes: [],
  };

  // Get file size
  try {
    const { statSync } = await import('node:fs');
    report.size_bytes = statSync(path).size;
  } catch { /* ignore */ }

  for (const mesh of meshes) {
    const meshInfo = {
      name: mesh.getName() || '(unnamed)',
      vertex_count: 0,
      index_count: 0,
      has_texture: false,
      texture_width: 0,
      texture_height: 0,
      area: 0,
      dominant_normal_axis: '',
      min_y: 0,
    };

    const prims = mesh.listPrimitives();
    if (prims.length > 0) {
      const prim = prims[0];
      const posAccessor = prim.getAttribute('POSITION');
      const indexAccessor = prim.getIndices();

      if (posAccessor) {
        meshInfo.vertex_count = posAccessor.getCount();
      }
      if (indexAccessor) {
        meshInfo.index_count = indexAccessor.getCount();
      }

      if (posAccessor) {
        const stats = computeGeometryStats(posAccessor, indexAccessor);
        meshInfo.area = Math.round(stats.area * 10000) / 10000;
        meshInfo.dominant_normal_axis = stats.dominant_normal_axis;
        meshInfo.min_y = Math.round(stats.min_y * 10000) / 10000;
      }

      const mat = prim.getMaterial();
      const texDims = getTextureDims(mat);
      meshInfo.has_texture = texDims.has_texture;
      meshInfo.texture_width = texDims.texture_width;
      meshInfo.texture_height = texDims.texture_height;
    }

    report.meshes.push(meshInfo);
  }

  process.stdout.write(JSON.stringify(report, null, 2) + '\n');
  return 0;
}

main(process.argv.slice(2))
  .then((code) => process.exit(code))
  .catch((e) => {
    process.stderr.write(`error: unhandled exception: ${e.message || String(e)}\n`);
    process.exit(1);
  });
