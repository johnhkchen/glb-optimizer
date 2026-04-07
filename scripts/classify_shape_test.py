#!/usr/bin/env python3
"""
T-004-02 unit tests for the production shape classifier.

Bare-assert style; the project does not depend on pytest. Run with:

    python3 scripts/classify_shape_test.py

Tests cover the synthetic taxonomy plus the rotational-symmetry
fallback and node-transform composition. Real-asset checks live in
the main script's --self-test mode.
"""

import io
import json
import math
import os
import struct
import sys
import tempfile

import numpy as np

# Import the module under test from the same directory.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import classify_shape as cs  # noqa: E402


# ---------------------------------------------------------------------------
# Synthetic-shape taxonomy.
# ---------------------------------------------------------------------------

def test_round_bush_synthetic():
    r = cs.classify_points(cs.synth_round_bush())
    assert r["category"] == "round-bush", r
    assert r["is_hard_surface"] is False, r
    assert 0 <= r["confidence"] <= 1


def test_planar_lattice():
    r = cs.classify_points(cs.synth_lattice())
    assert r["category"] == "planar", r
    assert r["is_hard_surface"] is True, r


def test_directional_row():
    r = cs.classify_points(cs.synth_row())
    assert r["category"] == "directional", r
    assert r["is_hard_surface"] is True, r


def test_tall_pole():
    r = cs.classify_points(cs.synth_pole())
    assert r["category"] == "tall-narrow", r
    assert r["is_hard_surface"] is True, r


def test_cube_hard_surface():
    """Cube exercises the rotational-symmetry fallback. The shape
    class is whichever PCA centroid wins (cube is by definition
    ambiguous between round-bush and planar in eigenvalue space);
    the load-bearing assertion is the hard-surface flag."""
    r = cs.classify_points(cs.synth_cube())
    assert r["is_hard_surface"] is True, r


# ---------------------------------------------------------------------------
# Edge cases.
# ---------------------------------------------------------------------------

def test_softmax_confidence_in_range():
    for fn in (cs.synth_round_bush, cs.synth_lattice, cs.synth_row, cs.synth_pole, cs.synth_cube):
        r = cs.classify_points(fn())
        assert 0.0 <= r["confidence"] <= 1.0, (fn.__name__, r["confidence"])


def test_rotational_symmetry_uses_canonical_alignment():
    """A pole has r2 ≈ r3 so the trailing eigenvectors are arbitrary;
    the fallback should still report axis_alignment = 1.0 because the
    pole IS axis-aligned with the canonical Y axis."""
    r = cs.classify_points(cs.synth_pole())
    assert r["features"]["axis_alignment"] == 1.0


def test_classification_category_in_enum():
    for fn in (cs.synth_round_bush, cs.synth_lattice, cs.synth_row, cs.synth_pole, cs.synth_cube):
        r = cs.classify_points(fn())
        assert r["category"] in cs.VALID_CATEGORIES


# ---------------------------------------------------------------------------
# T-004-04: candidates ranking surfaced for the comparison-UI modal.
# ---------------------------------------------------------------------------

def test_classify_points_emits_candidates_ranking():
    """The geometric ranking lives inside features.candidates as a
    softmax-normalized, descending list. The top entry must match the
    chosen category for non-hard-surface inputs."""
    r = cs.classify_points(cs.synth_round_bush())
    assert r["is_hard_surface"] is False, r
    cands = r["features"]["candidates"]
    assert isinstance(cands, list)
    assert 1 <= len(cands) <= 3
    seen = set()
    prev_score = 1.1
    for c in cands:
        assert isinstance(c, dict)
        assert set(c.keys()) == {"category", "score"}
        assert c["category"] in cs.VALID_CATEGORIES
        assert 0.0 <= c["score"] <= 1.0
        assert c["score"] <= prev_score, "candidates must be sorted descending by score"
        prev_score = c["score"]
        assert c["category"] not in seen, "no duplicate categories"
        seen.add(c["category"])
    assert cands[0]["category"] == r["category"]


def test_classify_points_candidates_promotes_hard_surface_overlay():
    """When the hard-surface overlay fires, the candidates list must
    surface `hard-surface` as the top entry — the geometric centroids
    do not include a hard-surface entry on their own (T-004-02 review
    §"Open concerns #1")."""
    r = cs.classify_points(cs.synth_lattice())
    assert r["is_hard_surface"] is True
    cands = r["features"]["candidates"]
    assert cands[0]["category"] == "hard-surface"
    assert cands[0]["score"] == 1.0


# ---------------------------------------------------------------------------
# GLB transform composition.
# ---------------------------------------------------------------------------

def _build_glb(positions, node=None):
    """Write a minimal GLB containing one mesh with one POSITION
    attribute. Optionally attach a transform to the root node. Returns
    the path to a temp file (caller is responsible for unlink)."""
    pos_bytes = positions.astype(np.float32).tobytes()
    pad = (-len(pos_bytes)) % 4
    pos_bytes += b"\x00" * pad
    bin_chunk = pos_bytes
    bbox_min = positions.min(axis=0).tolist()
    bbox_max = positions.max(axis=0).tolist()

    gltf = {
        "asset": {"version": "2.0"},
        "scene": 0,
        "scenes": [{"nodes": [0]}],
        "nodes": [{"mesh": 0}],
        "meshes": [{
            "primitives": [{"attributes": {"POSITION": 0}}],
        }],
        "accessors": [{
            "bufferView": 0,
            "componentType": 5126,
            "count": positions.shape[0],
            "type": "VEC3",
            "min": bbox_min,
            "max": bbox_max,
        }],
        "bufferViews": [{
            "buffer": 0,
            "byteOffset": 0,
            "byteLength": positions.shape[0] * 12,
        }],
        "buffers": [{"byteLength": len(bin_chunk)}],
    }
    if node is not None:
        gltf["nodes"][0].update(node)

    json_bytes = json.dumps(gltf).encode("utf-8")
    json_pad = (-len(json_bytes)) % 4
    json_bytes += b" " * json_pad

    total = 12 + 8 + len(json_bytes) + 8 + len(bin_chunk)
    out = io.BytesIO()
    out.write(struct.pack("<III", 0x46546C67, 2, total))
    out.write(struct.pack("<II", len(json_bytes), 0x4E4F534A))
    out.write(json_bytes)
    out.write(struct.pack("<II", len(bin_chunk), 0x004E4942))
    out.write(bin_chunk)

    f = tempfile.NamedTemporaryFile(suffix=".glb", delete=False)
    f.write(out.getvalue())
    f.close()
    return f.name


def test_node_transform_translation():
    """A node with a translation should land its vertices in world
    space, not local space."""
    pts = cs.synth_pole()
    path = _build_glb(pts, node={"translation": [10.0, 20.0, 30.0]})
    try:
        loaded = cs.load_all_positions(path)
    finally:
        os.unlink(path)
    expected_centroid = pts.mean(axis=0) + np.array([10.0, 20.0, 30.0])
    assert np.allclose(loaded.mean(axis=0), expected_centroid, atol=1e-4)


def test_node_transform_scale():
    pts = cs.synth_round_bush()
    path = _build_glb(pts, node={"scale": [2.0, 2.0, 2.0]})
    try:
        loaded = cs.load_all_positions(path)
    finally:
        os.unlink(path)
    # Scaled bbox extents should be 2x the originals.
    orig_extent = float(pts.max(axis=0).max() - pts.min(axis=0).min())
    new_extent = float(loaded.max(axis=0).max() - loaded.min(axis=0).min())
    assert math.isclose(new_extent, 2.0 * orig_extent, rel_tol=1e-4)


def test_no_node_transform_is_identity():
    pts = cs.synth_lattice()
    path = _build_glb(pts)
    try:
        loaded = cs.load_all_positions(path)
    finally:
        os.unlink(path)
    assert loaded.shape == pts.shape
    assert np.allclose(loaded, pts, atol=1e-4)


# ---------------------------------------------------------------------------
# Tiny test runner.
# ---------------------------------------------------------------------------

def main():
    tests = [(name, fn) for name, fn in sorted(globals().items())
             if name.startswith("test_") and callable(fn)]
    failures = 0
    for name, fn in tests:
        try:
            fn()
        except AssertionError as exc:
            failures += 1
            print(f"FAIL {name}: {exc}", file=sys.stderr)
        except Exception as exc:  # noqa: BLE001
            failures += 1
            print(f"ERROR {name}: {type(exc).__name__}: {exc}", file=sys.stderr)
        else:
            print(f"ok   {name}")
    if failures:
        print(f"\n{failures}/{len(tests)} failed", file=sys.stderr)
        return 1
    print(f"\nall {len(tests)} tests passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
