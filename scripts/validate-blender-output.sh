#!/usr/bin/env bash
# validate-blender-output.sh — Automated validation of Blender-produced
# GLB intermediates against a known-good browser-baked reference (T-014-06).
#
# Runs checks 1-6 from the ticket. Checks 7-8 (cross-repo load + visual
# spot check) are manual.
#
# Usage:
#   scripts/validate-blender-output.sh <test_asset_id> [--ref <reference_id>]
#
# Exit codes:
#   0 — all checks passed
#   1 — one or more checks failed
#   2 — usage/prerequisite error

set -euo pipefail

# --- Constants ---
DEFAULT_REF="1e562361be18ea9606222f8dcf81849d"
OUTPUTS_DIR="${HOME}/.glb-optimizer/outputs"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# --- Argument parsing ---
TEST_ID=""
REF_ID="${DEFAULT_REF}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --ref)
      REF_ID="$2"
      shift 2
      ;;
    --help|-h)
      echo "Usage: $0 <test_asset_id> [--ref <reference_id>]"
      echo "Default reference: ${DEFAULT_REF}"
      exit 0
      ;;
    -*)
      echo "Unknown flag: $1" >&2
      exit 2
      ;;
    *)
      if [[ -z "$TEST_ID" ]]; then
        TEST_ID="$1"
      else
        echo "Unexpected argument: $1" >&2
        exit 2
      fi
      shift
      ;;
  esac
done

if [[ -z "$TEST_ID" ]]; then
  echo "Usage: $0 <test_asset_id> [--ref <reference_id>]" >&2
  exit 2
fi

# --- Helpers ---
PASS_COUNT=0
FAIL_COUNT=0

pass() {
  echo "  $1 ... PASS"
  PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
  echo "  $1 ... FAIL: $2" >&2
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

file_size() {
  stat -f%z "$1" 2>/dev/null || stat -c%s "$1" 2>/dev/null || echo 0
}

# --- Prerequisites ---
echo "=== Blender Output Validation ==="
echo "Test asset:      ${TEST_ID}"
echo "Reference asset: ${REF_ID}"
echo ""

# Check node
if ! command -v node &>/dev/null; then
  echo "ERROR: node not found on PATH" >&2
  exit 2
fi

# Check inspect script
INSPECT_SCRIPT="${SCRIPT_DIR}/inspect-intermediate.mjs"
if [[ ! -f "$INSPECT_SCRIPT" ]]; then
  echo "ERROR: ${INSPECT_SCRIPT} not found" >&2
  exit 2
fi

# Check node_modules
if [[ ! -d "${SCRIPT_DIR}/node_modules" ]]; then
  echo "ERROR: scripts/node_modules not found — run 'just verify-pack-install'" >&2
  exit 2
fi

# Check reference files exist
for suffix in _billboard.glb _billboard_tilted.glb _volumetric.glb; do
  ref_file="${OUTPUTS_DIR}/${REF_ID}${suffix}"
  if [[ ! -f "$ref_file" ]]; then
    echo "ERROR: reference file missing: ${ref_file}" >&2
    exit 2
  fi
done

# Check test files exist
for suffix in _billboard.glb _billboard_tilted.glb _volumetric.glb; do
  test_file="${OUTPUTS_DIR}/${TEST_ID}${suffix}"
  if [[ ! -f "$test_file" ]]; then
    echo "ERROR: test file missing: ${test_file}" >&2
    echo "  Have you run 'glb-optimizer prepare' for this asset?" >&2
    exit 2
  fi
done

# =====================================================================
# CHECK 1: File size parity (0.5x – 2x of reference)
# =====================================================================
echo "[1/6] File size parity"

for suffix in _billboard.glb _billboard_tilted.glb _volumetric.glb; do
  label="${suffix%.glb}"
  label="${label#_}"

  ref_size=$(file_size "${OUTPUTS_DIR}/${REF_ID}${suffix}")
  test_size=$(file_size "${OUTPUTS_DIR}/${TEST_ID}${suffix}")

  lower=$((ref_size / 2))
  upper=$((ref_size * 2))

  if [[ "$test_size" -ge "$lower" && "$test_size" -le "$upper" ]]; then
    pass "${label}: ${test_size} bytes (ref: ${ref_size})"
  else
    fail "${label}: ${test_size} bytes (ref: ${ref_size})" "outside 0.5x–2x range [${lower}–${upper}]"
  fi
done
echo ""

# =====================================================================
# CHECK 2: GLB structure (mesh counts)
# =====================================================================
echo "[2/6] GLB structure"

# Inspect all three test intermediates
BB_JSON=$(node "$INSPECT_SCRIPT" "${OUTPUTS_DIR}/${TEST_ID}_billboard.glb" 2>/dev/null)
TILTED_JSON=$(node "$INSPECT_SCRIPT" "${OUTPUTS_DIR}/${TEST_ID}_billboard_tilted.glb" 2>/dev/null)
VOL_JSON=$(node "$INSPECT_SCRIPT" "${OUTPUTS_DIR}/${TEST_ID}_volumetric.glb" 2>/dev/null)

# Billboard: expect 7 meshes (6 side + 1 top)
bb_count=$(echo "$BB_JSON" | node -e "process.stdin.resume(); let d=''; process.stdin.on('data',c=>d+=c); process.stdin.on('end',()=>console.log(JSON.parse(d).mesh_count))")
if [[ "$bb_count" -eq 7 ]]; then
  pass "billboard: ${bb_count} meshes (expected 7)"
else
  fail "billboard: ${bb_count} meshes" "expected 7 (6 side + 1 top)"
fi

# Tilted: expect 6 meshes
tilted_count=$(echo "$TILTED_JSON" | node -e "process.stdin.resume(); let d=''; process.stdin.on('data',c=>d+=c); process.stdin.on('end',()=>console.log(JSON.parse(d).mesh_count))")
if [[ "$tilted_count" -eq 6 ]]; then
  pass "tilted: ${tilted_count} meshes (expected 6)"
else
  fail "tilted: ${tilted_count} meshes" "expected 6"
fi

# Volumetric: expect >= 1 mesh
vol_count=$(echo "$VOL_JSON" | node -e "process.stdin.resume(); let d=''; process.stdin.on('data',c=>d+=c); process.stdin.on('end',()=>console.log(JSON.parse(d).mesh_count))")
if [[ "$vol_count" -ge 1 ]]; then
  pass "volumetric: ${vol_count} meshes (expected >= 1)"
else
  fail "volumetric: ${vol_count} meshes" "expected >= 1"
fi
echo ""

# =====================================================================
# CHECK 3: Texture dimensions (all 512x512)
# =====================================================================
echo "[3/6] Texture dimensions"

check_textures() {
  local label="$1"
  local json="$2"
  local expected_w="${3:-512}"
  local expected_h="${4:-512}"

  local result
  result=$(echo "$json" | node -e "
    process.stdin.resume();
    let d='';
    process.stdin.on('data',c=>d+=c);
    process.stdin.on('end',()=>{
      const r = JSON.parse(d);
      const bad = r.meshes.filter(m =>
        !m.has_texture || m.texture_width !== ${expected_w} || m.texture_height !== ${expected_h}
      );
      if (bad.length === 0) {
        console.log('ok');
      } else {
        const names = bad.map(m => m.name + ':' + m.texture_width + 'x' + m.texture_height);
        console.log('fail:' + names.join(','));
      }
    });
  ")

  if [[ "$result" == "ok" ]]; then
    pass "${label}: all ${expected_w}x${expected_h}"
  else
    fail "${label}" "${result#fail:}"
  fi
}

check_textures "billboard" "$BB_JSON"
check_textures "tilted" "$TILTED_JSON"
check_textures "volumetric" "$VOL_JSON"
echo ""

# =====================================================================
# CHECK 4: Quad geometry (no zero-area, correct normals)
# =====================================================================
echo "[4/6] Quad geometry"

check_geometry() {
  local label="$1"
  local json="$2"
  local expected_side_normal="$3"  # dominant normal for side quads
  local expected_top_normal="$4"   # dominant normal for top quad (empty if none)

  local result
  result=$(echo "$json" | node -e "
    process.stdin.resume();
    let d='';
    process.stdin.on('data',c=>d+=c);
    process.stdin.on('end',()=>{
      const r = JSON.parse(d);
      const issues = [];
      for (const m of r.meshes) {
        if (m.area <= 0) {
          issues.push(m.name + ': zero area');
        }
      }
      // Check normals: last mesh is top quad for billboard, all are side for tilted
      const sideNormal = '${expected_side_normal}';
      const topNormal = '${expected_top_normal}';
      if (topNormal) {
        // Billboard: first N-1 are side, last is top
        for (let i = 0; i < r.meshes.length - 1; i++) {
          if (r.meshes[i].dominant_normal_axis !== sideNormal) {
            issues.push(r.meshes[i].name + ': expected normal ' + sideNormal + ' got ' + r.meshes[i].dominant_normal_axis);
          }
        }
        const top = r.meshes[r.meshes.length - 1];
        if (top.dominant_normal_axis !== topNormal) {
          issues.push(top.name + ': expected normal ' + topNormal + ' got ' + top.dominant_normal_axis);
        }
      } else {
        // Tilted or volumetric: all meshes should have the expected normal
        for (const m of r.meshes) {
          if (m.dominant_normal_axis !== sideNormal) {
            issues.push(m.name + ': expected normal ' + sideNormal + ' got ' + m.dominant_normal_axis);
          }
        }
      }
      if (issues.length === 0) {
        console.log('ok');
      } else {
        console.log('fail:' + issues.join('; '));
      }
    });
  ")

  if [[ "$result" == "ok" ]]; then
    pass "${label}: no zero-area, normals correct"
  else
    fail "${label}" "${result#fail:}"
  fi
}

# Billboard: side quads have dominant Z normal, top quad has dominant Y
check_geometry "billboard" "$BB_JSON" "z" "y"

# Tilted: all quads are tilted but still more vertical — dominant Z or Y depending
# on elevation angle. Use a lenient check: just verify no zero-area.
# The tilted quads at 30° elevation can have dominant Z or Y depending on aspect ratio.
tilted_result=$(echo "$TILTED_JSON" | node -e "
  process.stdin.resume();
  let d='';
  process.stdin.on('data',c=>d+=c);
  process.stdin.on('end',()=>{
    const r = JSON.parse(d);
    const zeroArea = r.meshes.filter(m => m.area <= 0);
    if (zeroArea.length === 0) {
      console.log('ok');
    } else {
      console.log('fail:' + zeroArea.map(m => m.name).join(',') + ' have zero area');
    }
  });
")
if [[ "$tilted_result" == "ok" ]]; then
  pass "tilted: no zero-area quads"
else
  fail "tilted" "${tilted_result#fail:}"
fi

# Volumetric: dome slices should have dominant Y normal (horizontal)
check_geometry "volumetric" "$VOL_JSON" "y" ""
echo ""

# =====================================================================
# CHECK 5: Pack combine
# =====================================================================
echo "[5/6] Pack combine"

BINARY="${PROJECT_DIR}/glb-optimizer"
if [[ ! -x "$BINARY" ]]; then
  # Try building
  if command -v go &>/dev/null; then
    (cd "$PROJECT_DIR" && go build -o glb-optimizer . 2>/dev/null)
  fi
fi

if [[ ! -x "$BINARY" ]]; then
  # Try the temp binary
  BINARY="${PROJECT_DIR}/glb-opt-tmp"
fi

if [[ ! -x "$BINARY" ]]; then
  fail "pack combine" "glb-optimizer binary not found — build with 'go build'"
else
  pack_output=$("$BINARY" pack "$TEST_ID" 2>&1) || true

  # Extract species from the tabwriter output (first column of data row after SPECIES header)
  PACK_SPECIES=$(echo "$pack_output" | awk '/^SPECIES/{found=1; next} found && !/^TOTAL/ && !/^  / && NF>0 {print $1; exit}')
  PACK_FILE="${HOME}/.glb-optimizer/dist/plants/${PACK_SPECIES}.glb"

  if echo "$pack_output" | grep -q "ok$"; then
    pass "pack combine: species=${PACK_SPECIES}"
  else
    fail "pack combine" "$(echo "$pack_output" | head -3)"
  fi
fi
echo ""

# =====================================================================
# CHECK 6: Pack verify
# =====================================================================
echo "[6/6] Pack verify"

VERIFY_SCRIPT="${SCRIPT_DIR}/verify-pack.mjs"
if [[ ! -f "$VERIFY_SCRIPT" ]]; then
  fail "pack verify" "verify-pack.mjs not found"
elif [[ -z "${PACK_FILE:-}" || ! -f "${PACK_FILE:-}" ]]; then
  fail "pack verify" "no pack file found (pack combine may have failed)"
else
  verify_output=$(node "$VERIFY_SCRIPT" "$PACK_FILE" 2>&1) || true
  if echo "$verify_output" | grep -q "^PASS"; then
    pass "pack verify: $(echo "$verify_output" | head -1)"
  else
    fail "pack verify" "$(echo "$verify_output" | head -3)"
  fi
fi
echo ""

# =====================================================================
# Summary
# =====================================================================
TOTAL=$((PASS_COUNT + FAIL_COUNT))
echo "Result: ${PASS_COUNT}/${TOTAL} checks passed"

if [[ "$FAIL_COUNT" -gt 0 ]]; then
  echo ""
  echo "Note: Checks 7-8 (plantastic cross-repo load + visual spot check) are manual."
  exit 1
fi

echo ""
echo "Note: Checks 7-8 (plantastic cross-repo load + visual spot check) are manual."
exit 0
