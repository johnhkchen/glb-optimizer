#!/usr/bin/env python3
"""
T-004-01 spike: vertex-cloud PCA shape classification.

Research-only. Loads a GLB (or generates a synthetic point cloud),
computes PCA + axis-alignment + per-axis density peakiness, and
proposes a class from the S-004 taxonomy.

Usage:
    python3 scripts/spike_shape_pca.py
    python3 scripts/spike_shape_pca.py assets/rose_julia_child.glb
    python3 scripts/spike_shape_pca.py synthetic:lattice synthetic:row

This is the spike. Production classifier lives in T-004-02.
"""

import argparse
import json
import math
import os
import struct
import sys

import numpy as np


# ---------------------------------------------------------------------------
# Class hypotheses (initial — design.md). Spike measures actuals.
# ---------------------------------------------------------------------------

# Centroids in (r2, r3) space, where r2 = lambda2/lambda1, r3 = lambda3/lambda1.
CLASS_CENTROIDS = {
    "round-bush":  (0.85, 0.75),
    "tall-narrow": (0.10, 0.10),  # disambiguated from directional by axis dir
    "directional": (0.20, 0.15),
    "planar":      (0.60, 0.05),
}

HARD_SURFACE_THRESHOLDS = {
    "axis_alignment_min": 0.95,
    "mean_peakiness_min": 3.0,
}

DEFAULT_INPUTS = [
    "assets/rose_julia_child.glb",
    "assets/wood_raised_bed.glb",
    "synthetic:lattice",
    "synthetic:row",
    "synthetic:pole",  # extra sanity case for tall-narrow
]


# ---------------------------------------------------------------------------
# GLB loading (positions only). Lifted/simplified from
# scripts/parametric_reconstruct.py.
# ---------------------------------------------------------------------------

def parse_glb(path):
    with open(path, "rb") as f:
        magic, version, _length = struct.unpack("<III", f.read(12))
        assert magic == 0x46546C67, f"{path}: not a GLB file"
        assert version == 2, f"{path}: unsupported glTF version {version}"

        chunk_len, chunk_type = struct.unpack("<II", f.read(8))
        assert chunk_type == 0x4E4F534A, f"{path}: expected JSON chunk"
        gltf = json.loads(f.read(chunk_len))

        chunk_len, chunk_type = struct.unpack("<II", f.read(8))
        assert chunk_type == 0x004E4942, f"{path}: expected BIN chunk"
        bin_data = f.read(chunk_len)

    return gltf, bin_data


def extract_positions(gltf, bin_data, path_for_errors=""):
    """Return float32[N, 3] vertex positions for the first primitive of
    the first mesh. Asserts the assumptions confirmed in research.md:
    single mesh, single primitive, identity node transforms."""

    assert len(gltf.get("meshes", [])) == 1, (
        f"{path_for_errors}: spike only handles single-mesh GLBs "
        f"(found {len(gltf.get('meshes', []))})"
    )
    prims = gltf["meshes"][0]["primitives"]
    assert len(prims) == 1, (
        f"{path_for_errors}: spike only handles single-primitive meshes "
        f"(found {len(prims)})"
    )

    for n in gltf.get("nodes", []):
        for key in ("matrix", "rotation", "translation", "scale"):
            assert key not in n, (
                f"{path_for_errors}: node has '{key}' transform; spike "
                "does not compose world transforms (see research.md)"
            )

    prim = prims[0]
    pos_acc = gltf["accessors"][prim["attributes"]["POSITION"]]
    pos_bv = gltf["bufferViews"][pos_acc.get("bufferView", 0)]
    offset = pos_bv.get("byteOffset", 0) + pos_acc.get("byteOffset", 0)
    count = pos_acc["count"]
    raw = bin_data[offset:offset + count * 12]
    return np.frombuffer(raw, dtype=np.float32).reshape(-1, 3).astype(np.float64)


# ---------------------------------------------------------------------------
# Synthetic generators. Each returns float64[N, 3].
# ---------------------------------------------------------------------------

def synth_lattice(n=4000, seed=1):
    """Flat trellis panel: 1.2 (X) x 1.8 (Y) x 0.05 (Z) lattice of slats.
    Three vertical slats + four horizontal slats sampled uniformly."""
    rng = np.random.default_rng(seed)
    pts = []
    per_slat = n // 7
    # 3 vertical slats (along Y) at x = 0.0, 0.6, 1.2
    for x_center in (0.0, 0.6, 1.2):
        s = rng.uniform(low=[-0.025, 0.0, -0.025],
                        high=[0.025, 1.8, 0.025], size=(per_slat, 3))
        s[:, 0] += x_center
        pts.append(s)
    # 4 horizontal slats (along X) at y = 0.0, 0.6, 1.2, 1.8
    for y_center in (0.0, 0.6, 1.2, 1.8):
        s = rng.uniform(low=[0.0, -0.025, -0.025],
                        high=[1.2, 0.025, 0.025], size=(per_slat, 3))
        s[:, 1] += y_center
        pts.append(s)
    return np.concatenate(pts, axis=0)


def synth_row(n=4000, seed=2):
    """Long horizontal box: 6.0 (X) x 0.4 (Y) x 0.4 (Z). Vertices
    concentrate on the six faces (so density peakiness should be high)."""
    rng = np.random.default_rng(seed)
    dims = np.array([6.0, 0.4, 0.4])
    # Sample faces of the box, not the interior, so density peaks exist.
    pts = []
    per_face = n // 6
    for axis in range(3):
        for sign in (0.0, 1.0):
            s = rng.uniform(0.0, 1.0, size=(per_face, 3)) * dims
            s[:, axis] = sign * dims[axis]
            pts.append(s)
    arr = np.concatenate(pts, axis=0)
    arr -= arr.mean(axis=0)
    return arr


def synth_pole(n=4000, seed=3):
    """Vertical pole: 0.1 (X) x 3.0 (Y) x 0.1 (Z), faces sampled."""
    rng = np.random.default_rng(seed)
    dims = np.array([0.1, 3.0, 0.1])
    pts = []
    per_face = n // 6
    for axis in range(3):
        for sign in (0.0, 1.0):
            s = rng.uniform(0.0, 1.0, size=(per_face, 3)) * dims
            s[:, axis] = sign * dims[axis]
            pts.append(s)
    arr = np.concatenate(pts, axis=0)
    arr -= arr.mean(axis=0)
    return arr


SYNTH_TABLE = {
    "lattice": synth_lattice,
    "row":     synth_row,
    "pole":    synth_pole,
}


def load_points(spec):
    """Resolve a CLI input to (label, np.ndarray[N,3])."""
    if spec.startswith("synthetic:"):
        name = spec.split(":", 1)[1]
        if name not in SYNTH_TABLE:
            raise SystemExit(f"unknown synthetic '{name}'; "
                             f"options: {sorted(SYNTH_TABLE)}")
        return spec, SYNTH_TABLE[name]()
    gltf, bin_data = parse_glb(spec)
    return spec, extract_positions(gltf, bin_data, path_for_errors=spec)


# ---------------------------------------------------------------------------
# PCA + features
# ---------------------------------------------------------------------------

def compute_pca(points):
    centroid = points.mean(axis=0)
    centered = points - centroid
    cov = np.cov(centered, rowvar=False)
    # eigh: ascending eigenvalues for symmetric matrices.
    evals_asc, evecs_asc = np.linalg.eigh(cov)
    order = np.argsort(evals_asc)[::-1]
    evals = evals_asc[order]
    evecs = evecs_asc[:, order]            # columns are eigenvectors
    # Guard against tiny negative numerical noise on the smallest eval.
    evals = np.clip(evals, 0.0, None)

    bbox_min = points.min(axis=0)
    bbox_max = points.max(axis=0)
    dims = bbox_max - bbox_min
    aspect = float(dims[1] / max(dims[0], dims[2]))

    lam1 = float(evals[0]) if evals[0] > 0 else 1e-12
    r2 = float(evals[1]) / lam1
    r3 = float(evals[2]) / lam1

    principal_axis = evecs[:, 0]
    return {
        "centroid": centroid.tolist(),
        "bbox_min": bbox_min.tolist(),
        "bbox_max": bbox_max.tolist(),
        "dimensions": dims.tolist(),
        "aspect_ratio": aspect,
        "eigenvalues": evals.tolist(),
        "eigenvectors": evecs.T.tolist(),  # rows = axes for legibility
        "ratios": {"r2": r2, "r3": r3},
        "principal_axis": principal_axis.tolist(),
    }


def axis_alignment_score(evecs_rows):
    """Mean over PCA eigenvectors of max |dot| with canonical basis.
    1.0 = each PCA axis is parallel to some world axis (boxy).
    1/sqrt(3) ≈ 0.577 = pointing equally between all world axes."""
    canonical = np.eye(3)
    scores = []
    for v in evecs_rows:
        dots = np.abs(np.asarray(v) @ canonical)
        scores.append(float(dots.max()))
    return float(np.mean(scores))


def density_peakiness(points, axis_vec, n_bins=50):
    """Project onto axis, histogram, return (max - mean) / std.
    High = sharp discrete peaks (faces of a box, layers of boards).
    ~0  = smooth distribution (sphere, isotropic blob)."""
    proj = points @ np.asarray(axis_vec)
    counts, _edges = np.histogram(proj, bins=n_bins)
    counts = counts.astype(np.float64)
    std = counts.std()
    if std < 1e-9:
        return 0.0
    return float((counts.max() - counts.mean()) / std)


def all_peakiness(points, evecs_rows, n_bins=50):
    return [density_peakiness(points, v, n_bins) for v in evecs_rows]


def principal_axis_orientation(v, vertical_threshold=0.85):
    """Return 'vertical' if the axis is mostly along world Y,
    'horizontal' if mostly in the XZ plane, else 'diagonal'."""
    vy = abs(float(v[1]))
    if vy >= vertical_threshold:
        return "vertical"
    if vy <= (1.0 - vertical_threshold):
        return "horizontal"
    return "diagonal"


# ---------------------------------------------------------------------------
# Classification
# ---------------------------------------------------------------------------

def nearest_class(r2, r3, principal_orientation):
    """Return (class, confidence, ranking).

    confidence = (d_2nd - d_1st) / (d_2nd + d_1st), in [0, 1]."""

    distances = {}
    for name, (cr2, cr3) in CLASS_CENTROIDS.items():
        distances[name] = math.hypot(r2 - cr2, r3 - cr3)

    # tall-narrow vs directional disambiguation by axis direction.
    # If principal axis isn't vertical, tall-narrow gets penalized so
    # the classifier prefers directional, and vice versa.
    if principal_orientation == "vertical":
        distances["directional"] += 0.5
    elif principal_orientation == "horizontal":
        distances["tall-narrow"] += 0.5
    # diagonal: leave both, let the geometry decide.

    ranking = sorted(distances.items(), key=lambda kv: kv[1])
    best_name, d_best = ranking[0]
    _second_name, d_second = ranking[1]
    denom = d_second + d_best
    confidence = 0.0 if denom < 1e-9 else (d_second - d_best) / denom
    confidence = max(0.0, min(1.0, confidence))
    return best_name, confidence, ranking


def apply_hard_surface_overlay(axis_alignment, peakiness_list):
    mean_peak = float(np.mean(peakiness_list))
    is_hs = (
        axis_alignment >= HARD_SURFACE_THRESHOLDS["axis_alignment_min"]
        and mean_peak >= HARD_SURFACE_THRESHOLDS["mean_peakiness_min"]
    )
    return is_hs, mean_peak


# ---------------------------------------------------------------------------
# Reporting
# ---------------------------------------------------------------------------

def features_for(label, points):
    pca = compute_pca(points)
    align = axis_alignment_score(pca["eigenvectors"])
    peak = all_peakiness(points, pca["eigenvectors"])
    orient = principal_axis_orientation(pca["principal_axis"])
    cls, conf, ranking = nearest_class(
        pca["ratios"]["r2"], pca["ratios"]["r3"], orient
    )
    is_hs, mean_peak = apply_hard_surface_overlay(align, peak)
    return {
        "label": label,
        "n_points": int(points.shape[0]),
        **pca,
        "principal_orientation": orient,
        "axis_alignment": align,
        "peakiness": peak,
        "mean_peakiness": mean_peak,
        "classification": {
            "class": cls,
            "confidence": conf,
            "is_hard_surface": is_hs,
            "ranking": [(n, round(d, 4)) for n, d in ranking],
        },
    }


def fmt_vec(v, prec=3):
    return "[" + ", ".join(f"{x:+.{prec}f}" for x in v) + "]"


def print_report(f):
    print(f"\n=== {f['label']} ===")
    print(f"  n_points        : {f['n_points']}")
    print(f"  bbox dims       : {fmt_vec(f['dimensions'])}")
    print(f"  aspect (h/w)    : {f['aspect_ratio']:+.3f}")
    print(f"  eigenvalues     : {fmt_vec(f['eigenvalues'], 6)}")
    print(f"  ratios r2,r3    : r2={f['ratios']['r2']:.4f} "
          f"r3={f['ratios']['r3']:.4f}")
    print(f"  principal axis  : {fmt_vec(f['principal_axis'])} "
          f"({f['principal_orientation']})")
    print(f"  axis alignment  : {f['axis_alignment']:.4f}")
    print(f"  peakiness x/y/z': {fmt_vec(f['peakiness'])} "
          f"(mean {f['mean_peakiness']:.3f})")
    c = f["classification"]
    print(f"  -> class        : {c['class']:12s} "
          f"confidence={c['confidence']:.3f} "
          f"hard_surface={c['is_hard_surface']}")
    print(f"     ranking      : {c['ranking']}")


def print_summary_table(features_list):
    print("\n=== cross-asset summary ===")
    header = (
        f"{'asset':40s}  {'r2':>7s}  {'r3':>7s}  {'align':>6s}  "
        f"{'peak':>6s}  {'class':14s}  {'conf':>5s}  hs"
    )
    print(header)
    print("-" * len(header))
    for f in features_list:
        c = f["classification"]
        print(
            f"{f['label'][:40]:40s}  "
            f"{f['ratios']['r2']:7.4f}  {f['ratios']['r3']:7.4f}  "
            f"{f['axis_alignment']:6.3f}  {f['mean_peakiness']:6.2f}  "
            f"{c['class']:14s}  {c['confidence']:5.2f}  "
            f"{'Y' if c['is_hard_surface'] else 'N'}"
        )


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "inputs",
        nargs="*",
        help="GLB paths or 'synthetic:{lattice,row,pole}'. "
             "Defaults to the spike's standard test set.",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Also dump per-asset features as one-line JSON.",
    )
    args = parser.parse_args(argv)

    inputs = args.inputs or DEFAULT_INPUTS

    features_list = []
    for spec in inputs:
        if spec.startswith("synthetic:") or os.path.exists(spec):
            label, points = load_points(spec)
        else:
            print(f"!! skipping missing path: {spec}", file=sys.stderr)
            continue
        f = features_for(label, points)
        features_list.append(f)
        print_report(f)
        if args.json:
            # Drop heavy fields for the JSON line.
            slim = {k: v for k, v in f.items()
                    if k not in ("eigenvectors", "centroid",
                                 "bbox_min", "bbox_max")}
            print("JSON " + json.dumps(slim, separators=(",", ":")))

    print_summary_table(features_list)


if __name__ == "__main__":
    main()
