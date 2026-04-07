#!/usr/bin/env python3
"""
T-004-02 production shape classifier.

Reads a GLB, runs PCA + a hard-surface overlay, and prints a single
JSON object on stdout with the classification, confidence, and the
underlying features.

Usage:
    python3 scripts/classify_shape.py <input.glb>
    python3 scripts/classify_shape.py --self-test

The single-input mode prints exactly one JSON line to stdout. All
human-readable progress goes to stderr. Non-zero exit on any failure.

Calibrated thresholds and centroids are from T-004-01's spike review
(see docs/active/work/T-004-01/review.md). This file does NOT import
from spike_shape_pca.py — the spike is research history; this is the
production re-implementation.
"""

import argparse
import json
import math
import struct
import sys

import numpy as np


# Calibrated centroids in (r2, r3) eigenvalue-ratio space, where
# r2 = lambda2 / lambda1 and r3 = lambda3 / lambda1. Numbers come from
# the spike's measured asset suite (T-004-01 review §"Findings #2").
CLASS_CENTROIDS = {
    "round-bush":  (0.90, 0.50),
    "planar":      (0.45, 0.02),
    "directional": (0.05, 0.05),  # axis disambiguation: horizontal
    "tall-narrow": (0.05, 0.05),  # axis disambiguation: vertical
}

# Hard-surface overlay thresholds, calibrated in T-004-01.
HARD_SURFACE_THRESHOLDS = {
    "axis_alignment_min": 0.90,
    "mean_peakiness_min": 2.5,
}

# Softmax temperature for the confidence formula. Picked so the spike's
# clean test cases land in [0.7, 0.95] and runner-ups stay below 0.7.
CONFIDENCE_TEMPERATURE = 0.20

# When r3/r2 >= this, the trailing PCA eigenvectors are arbitrary
# (rotational symmetry). Fall back to AABB axis-alignment for the
# hard-surface overlay in that regime. T-004-01 review §"Findings #5".
ROT_SYMMETRY_RATIO = 0.5

# Closed enum from S-004 / acceptance criteria. Re-validated Go-side.
VALID_CATEGORIES = {
    "round-bush", "directional", "tall-narrow", "planar",
    "hard-surface", "unknown",
}


# ---------------------------------------------------------------------------
# GLB loader (full node-tree walk + transform composition).
# ---------------------------------------------------------------------------

def parse_glb(path):
    with open(path, "rb") as f:
        magic, version, _length = struct.unpack("<III", f.read(12))
        if magic != 0x46546C67:
            raise ValueError(f"{path}: not a GLB file")
        if version != 2:
            raise ValueError(f"{path}: unsupported glTF version {version}")
        chunk_len, chunk_type = struct.unpack("<II", f.read(8))
        if chunk_type != 0x4E4F534A:
            raise ValueError(f"{path}: expected JSON chunk")
        gltf = json.loads(f.read(chunk_len))
        chunk_len, chunk_type = struct.unpack("<II", f.read(8))
        if chunk_type != 0x004E4942:
            raise ValueError(f"{path}: expected BIN chunk")
        bin_data = f.read(chunk_len)
    return gltf, bin_data


def _accessor_positions(gltf, bin_data, accessor_idx):
    acc = gltf["accessors"][accessor_idx]
    bv = gltf["bufferViews"][acc.get("bufferView", 0)]
    offset = bv.get("byteOffset", 0) + acc.get("byteOffset", 0)
    count = acc["count"]
    raw = bin_data[offset:offset + count * 12]
    return np.frombuffer(raw, dtype=np.float32).reshape(-1, 3).astype(np.float64)


def _node_local_matrix(node):
    """Return a 4x4 local transform from a glTF node's TRS or matrix."""
    if "matrix" in node:
        # glTF matrices are column-major; numpy is row-major. Transpose.
        m = np.array(node["matrix"], dtype=np.float64).reshape(4, 4).T
        return m
    m = np.eye(4, dtype=np.float64)
    if "scale" in node:
        s = node["scale"]
        m[0, 0] *= s[0]; m[1, 1] *= s[1]; m[2, 2] *= s[2]
    if "rotation" in node:
        # quaternion (x,y,z,w) → rotation matrix
        x, y, z, w = node["rotation"]
        xx, yy, zz = x * x, y * y, z * z
        xy, xz, yz = x * y, x * z, y * z
        wx, wy, wz = w * x, w * y, w * z
        r = np.array([
            [1 - 2 * (yy + zz),     2 * (xy - wz),     2 * (xz + wy), 0],
            [    2 * (xy + wz), 1 - 2 * (xx + zz),     2 * (yz - wx), 0],
            [    2 * (xz - wy),     2 * (yz + wx), 1 - 2 * (xx + yy), 0],
            [                0,                 0,                 0, 1],
        ], dtype=np.float64)
        m = m @ r
    if "translation" in node:
        t = node["translation"]
        m[0, 3] = t[0]; m[1, 3] = t[1]; m[2, 3] = t[2]
    return m


def _apply_transform(positions, world):
    """Apply a 4x4 affine to an (N,3) array."""
    n = positions.shape[0]
    homog = np.ones((n, 4), dtype=np.float64)
    homog[:, :3] = positions
    transformed = homog @ world.T
    return transformed[:, :3]


def load_all_positions(path):
    """Walk the node tree, compose world transforms, concatenate
    positions across every primitive of every reachable mesh."""
    gltf, bin_data = parse_glb(path)

    # Determine the root scene's nodes.
    scenes = gltf.get("scenes", [])
    scene_idx = gltf.get("scene", 0) if scenes else None
    if scene_idx is not None and scene_idx < len(scenes):
        roots = scenes[scene_idx].get("nodes", [])
    else:
        # Fall back to all nodes (rare, but valid glTF).
        roots = list(range(len(gltf.get("nodes", []))))

    chunks = []

    def visit(node_idx, parent_world):
        if node_idx >= len(gltf.get("nodes", [])):
            return
        node = gltf["nodes"][node_idx]
        local = _node_local_matrix(node)
        world = parent_world @ local
        if "mesh" in node:
            mesh = gltf["meshes"][node["mesh"]]
            for prim in mesh["primitives"]:
                if "POSITION" not in prim["attributes"]:
                    continue
                pts = _accessor_positions(gltf, bin_data, prim["attributes"]["POSITION"])
                if not _is_identity(world):
                    pts = _apply_transform(pts, world)
                chunks.append(pts)
        for child in node.get("children", []):
            visit(child, world)

    identity = np.eye(4, dtype=np.float64)
    for r in roots:
        visit(r, identity)

    if not chunks:
        # Defensive: walk every mesh's primitives in declaration order
        # if the scene graph yielded nothing (e.g. a malformed GLB with
        # no scene). Better to classify than to crash.
        for mesh in gltf.get("meshes", []):
            for prim in mesh["primitives"]:
                if "POSITION" not in prim["attributes"]:
                    continue
                chunks.append(_accessor_positions(gltf, bin_data, prim["attributes"]["POSITION"]))

    if not chunks:
        raise ValueError(f"{path}: no POSITION attributes found")

    return np.concatenate(chunks, axis=0)


def _is_identity(m):
    return np.allclose(m, np.eye(4), atol=1e-9)


# ---------------------------------------------------------------------------
# PCA + features.
# ---------------------------------------------------------------------------

def compute_features(points):
    centroid = points.mean(axis=0)
    centered = points - centroid
    cov = np.cov(centered, rowvar=False)
    evals_asc, evecs_asc = np.linalg.eigh(cov)
    order = np.argsort(evals_asc)[::-1]
    evals = np.clip(evals_asc[order], 0.0, None)
    evecs = evecs_asc[:, order]

    bbox_min = points.min(axis=0)
    bbox_max = points.max(axis=0)
    dims = bbox_max - bbox_min

    lam1 = float(evals[0]) if evals[0] > 0 else 1e-12
    r2 = float(evals[1]) / lam1
    r3 = float(evals[2]) / lam1

    principal = evecs[:, 0]
    return {
        "n_points": int(points.shape[0]),
        "dimensions": dims.tolist(),
        "aspect_ratio": float(dims[1] / max(dims[0], dims[2], 1e-12)),
        "eigenvalues": evals.tolist(),
        "eigenvectors": evecs.T.tolist(),
        "ratios": {"r2": r2, "r3": r3},
        "principal_axis": principal.tolist(),
        "principal_orientation": _axis_orientation(principal),
    }


def _axis_orientation(v, vertical_threshold=0.85):
    vy = abs(float(v[1]))
    if vy >= vertical_threshold:
        return "vertical"
    if vy <= (1.0 - vertical_threshold):
        return "horizontal"
    return "diagonal"


# ---------------------------------------------------------------------------
# Hard-surface overlay (with rotational-symmetry fallback).
# ---------------------------------------------------------------------------

def _axis_alignment_score(rows):
    canonical = np.eye(3)
    scores = []
    for v in rows:
        scores.append(float(np.abs(np.asarray(v) @ canonical).max()))
    return float(np.mean(scores))


def _density_peakiness(points, axis_vec, n_bins=50):
    proj = points @ np.asarray(axis_vec)
    counts, _ = np.histogram(proj, bins=n_bins)
    counts = counts.astype(np.float64)
    std = counts.std()
    if std < 1e-9:
        return 0.0
    return float((counts.max() - counts.mean()) / std)


def hard_surface_overlay(points, features):
    """Decide whether the asset is a "hard surface" — i.e. boxy and
    axis-aligned with sharp face peaks in its vertex distribution.

    Two signals fire together: (1) PCA axis alignment to the world
    basis, with an AABB-axis-alignment fallback when the trailing
    eigenvalues are too close to disambiguate; and (2) mean per-axis
    density peakiness measured on the canonical XYZ axes (always),
    which is the right thing to measure for axis-aligned faces and
    has the side benefit of being invariant to PCA sign / order
    flips on rotationally symmetric inputs.
    """
    rows = features["eigenvectors"]
    r2 = features["ratios"]["r2"]
    r3 = features["ratios"]["r3"]

    if r2 > 1e-9 and (r3 / max(r2, 1e-12)) >= ROT_SYMMETRY_RATIO:
        # Trailing eigenvectors arbitrary; treat the canonical basis
        # as the "natural" axes. By construction this scores 1.0 for
        # any cuboid-shaped input, which is what we want — the
        # peakiness signal is the discriminator in this regime.
        align = 1.0
    else:
        align = _axis_alignment_score(rows)

    # Peakiness on canonical axes. Independent of PCA axis order, so
    # rotationally symmetric inputs (cube, row, pole) get a stable
    # signal. For a tilted box this drops correctly.
    canonical_axes = [
        np.array([1.0, 0, 0]),
        np.array([0, 1.0, 0]),
        np.array([0, 0, 1.0]),
    ]
    peakiness = [_density_peakiness(points, a) for a in canonical_axes]

    mean_peak = float(np.mean(peakiness))
    is_hs = (
        align >= HARD_SURFACE_THRESHOLDS["axis_alignment_min"]
        and mean_peak >= HARD_SURFACE_THRESHOLDS["mean_peakiness_min"]
    )
    return is_hs, align, peakiness, mean_peak


# ---------------------------------------------------------------------------
# Classification.
# ---------------------------------------------------------------------------

def classify(features):
    r2 = features["ratios"]["r2"]
    r3 = features["ratios"]["r3"]
    orientation = features["principal_orientation"]

    distances = {}
    for name, (cr2, cr3) in CLASS_CENTROIDS.items():
        distances[name] = math.hypot(r2 - cr2, r3 - cr3)

    # Disambiguate the colocated tall-narrow / directional centroid by
    # principal-axis orientation.
    if orientation == "vertical":
        distances["directional"] += 0.5
    elif orientation == "horizontal":
        distances["tall-narrow"] += 0.5

    ranking = sorted(distances.items(), key=lambda kv: kv[1])
    best, _ = ranking[0]

    # Softmax confidence with calibrated temperature. exp underflow is
    # fine — np handles it.
    T = CONFIDENCE_TEMPERATURE
    weights = {n: math.exp(-d / T) for n, d in distances.items()}
    total = sum(weights.values())
    confidence = weights[best] / total if total > 0 else 0.0
    confidence = max(0.0, min(1.0, confidence))
    return best, confidence, ranking, distances


def _build_candidates(distances, is_hs, top_n=3):
    """T-004-04: build the per-category ranking surfaced to the
    comparison-UI modal. Distances → softmax → sorted descending.
    The hard-surface overlay always wins when set, because the
    geometric centroids do not include a hard-surface entry — see
    T-004-02 review §"Open concerns #1" (the wood-bed asset measures
    as `planar` even though the overlay fires)."""
    T = CONFIDENCE_TEMPERATURE
    weights = {n: math.exp(-d / T) for n, d in distances.items()}
    total = sum(weights.values())
    if total <= 0:
        scored = [(n, 0.0) for n in distances]
    else:
        scored = [(n, w / total) for n, w in weights.items()]
    scored.sort(key=lambda kv: kv[1], reverse=True)
    out = [{"category": n, "score": float(s)} for n, s in scored]
    if is_hs:
        # Prepend a synthetic hard-surface entry. Score 1.0 reflects
        # "the overlay is binary; if it fired we trust it over the
        # geometric centroid distance".
        out = [{"category": "hard-surface", "score": 1.0}] + [
            c for c in out if c["category"] != "hard-surface"
        ]
    return out[:top_n]


def classify_points(points):
    features = compute_features(points)
    is_hs, align, peakiness, mean_peak = hard_surface_overlay(points, features)
    cls, confidence, _ranking, distances = classify(features)
    features["axis_alignment"] = align
    features["peakiness"] = peakiness
    features["mean_peakiness"] = mean_peak
    # T-004-04: per-category ranking for the multi-strategy comparison
    # UI. Lives inside the opaque `features` dict so it flows through
    # the Go subprocess wrapper and into the analytics event payload
    # without any Go-side schema change.
    features["candidates"] = _build_candidates(distances, is_hs)
    if cls not in VALID_CATEGORIES:
        cls = "unknown"
    return {
        "category": cls,
        "confidence": confidence,
        "is_hard_surface": bool(is_hs),
        "features": features,
    }


# ---------------------------------------------------------------------------
# Synthetic test cases (for --self-test and the python unit tests).
# ---------------------------------------------------------------------------

def synth_round_bush(n=4000, seed=11):
    rng = np.random.default_rng(seed)
    pts = rng.normal(size=(n, 3))
    pts /= np.linalg.norm(pts, axis=1, keepdims=True)
    pts *= rng.uniform(0.7, 1.0, size=(n, 1))
    pts[:, 0] *= 1.0
    pts[:, 1] *= 0.95
    pts[:, 2] *= 1.05
    return pts


def synth_lattice(n=4200, seed=1):
    rng = np.random.default_rng(seed)
    pts = []
    per = n // 7
    for x in (0.0, 0.6, 1.2):
        s = rng.uniform(low=[-0.025, 0, -0.025], high=[0.025, 1.8, 0.025], size=(per, 3))
        s[:, 0] += x
        pts.append(s)
    for y in (0.0, 0.6, 1.2, 1.8):
        s = rng.uniform(low=[0, -0.025, -0.025], high=[1.2, 0.025, 0.025], size=(per, 3))
        s[:, 1] += y
        pts.append(s)
    return np.concatenate(pts, axis=0)


def _box_faces(dims, n=4200, seed=2):
    rng = np.random.default_rng(seed)
    dims = np.asarray(dims, dtype=np.float64)
    pts = []
    per = n // 6
    for axis in range(3):
        for sign in (0.0, 1.0):
            s = rng.uniform(0, 1, size=(per, 3)) * dims
            s[:, axis] = sign * dims[axis]
            pts.append(s)
    arr = np.concatenate(pts, axis=0)
    arr -= arr.mean(axis=0)
    return arr


def synth_row():
    return _box_faces([6.0, 0.4, 0.4], seed=2)


def synth_pole():
    return _box_faces([0.1, 3.0, 0.1], seed=3)


def synth_cube():
    return _box_faces([1.0, 1.0, 1.0], seed=4)


SYNTHS = {
    "round-bush": synth_round_bush,
    "lattice":    synth_lattice,
    "row":        synth_row,
    "pole":       synth_pole,
    "cube":       synth_cube,
}


# ---------------------------------------------------------------------------
# Entrypoint.
# ---------------------------------------------------------------------------

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", nargs="?", help="path to a GLB file")
    parser.add_argument("--self-test", action="store_true",
                        help="run the bundled synthetic suite and exit")
    args = parser.parse_args(argv)

    if args.self_test:
        for name, fn in SYNTHS.items():
            result = classify_points(fn())
            print(
                f"{name:12s} -> {result['category']:12s} "
                f"conf={result['confidence']:.3f} hs={result['is_hard_surface']}",
                file=sys.stderr,
            )
        return 0

    if not args.input:
        parser.error("input GLB path is required (or pass --self-test)")

    try:
        points = load_all_positions(args.input)
        result = classify_points(points)
    except Exception as exc:
        print(f"classify_shape: {exc}", file=sys.stderr)
        return 2

    json.dump(result, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
