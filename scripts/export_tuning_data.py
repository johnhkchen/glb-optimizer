#!/usr/bin/env python3
"""Aggregate ~/.glb-optimizer tuning artifacts into a single JSONL bundle.

T-003-04. Produces one JSONL file with three kinds of records:

    {"kind":"asset",   ...}   one per asset that has any tuning artifact
    {"kind":"profile", ...}   one per saved profile
    {"kind":"meta",    ...}   final sentinel line at EOF

Asset records embed the asset's current settings, the accepted snapshot
if one exists, and the full event stream for that asset (sorted by
timestamp). Profile records carry the named profile in full.

Thumbnail paths are stored RELATIVE to the workdir
(e.g. "accepted/thumbs/<id>.jpg") so the bundle is portable. The meta
record carries the absolute workdir for users who want to reconstruct
full paths.

Stdlib only.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path

EXPORT_SCHEMA_VERSION = 1


def expand_workdir(s: str | None) -> Path:
    if s:
        return Path(os.path.expanduser(s)).resolve()
    return Path(os.path.expanduser("~/.glb-optimizer")).resolve()


def load_json(p: Path) -> dict | None:
    """Load a single JSON file. Returns None on missing or malformed."""
    try:
        with p.open("r", encoding="utf-8") as f:
            return json.load(f)
    except FileNotFoundError:
        return None
    except (OSError, json.JSONDecodeError) as e:
        print(f"export: skip {p}: {e}", file=sys.stderr)
        return None


def iter_jsonl(p: Path):
    """Yield decoded JSON envelopes from a JSONL file. Tolerates torn or
    malformed lines (logs to stderr)."""
    try:
        f = p.open("r", encoding="utf-8")
    except OSError as e:
        print(f"export: open {p}: {e}", file=sys.stderr)
        return
    with f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError as e:
                print(f"export: skip {p}:{lineno}: {e}", file=sys.stderr)


def collect_events_by_asset(workdir: Path) -> tuple[dict[str, list[dict]], int]:
    """Walk tuning/*.jsonl once, bucket events by asset_id, and count
    the total number of valid events seen."""
    by_asset: dict[str, list[dict]] = {}
    total = 0
    tuning_dir = workdir / "tuning"
    if not tuning_dir.is_dir():
        return by_asset, 0
    for jsonl in sorted(tuning_dir.glob("*.jsonl")):
        for ev in iter_jsonl(jsonl):
            if not isinstance(ev, dict):
                continue
            asset_id = ev.get("asset_id") or ""
            by_asset.setdefault(asset_id, []).append(ev)
            total += 1
    for evs in by_asset.values():
        evs.sort(key=lambda e: e.get("timestamp", ""))
    return by_asset, total


def enumerate_asset_ids(workdir: Path, events_by_asset: dict[str, list[dict]]) -> list[str]:
    """Union of asset ids found in originals/, settings/, accepted/,
    and the events index. Sorted ascending for deterministic output."""
    ids: set[str] = set(events_by_asset.keys())
    ids.discard("")  # events with no asset_id are not asset records
    for sub, suffix in (("originals", ".glb"), ("settings", ".json"), ("accepted", ".json")):
        d = workdir / sub
        if not d.is_dir():
            continue
        for f in d.iterdir():
            if f.is_file() and f.name.endswith(suffix):
                ids.add(f.name[: -len(suffix)])
    return sorted(ids)


def build_asset_record(
    workdir: Path,
    asset_id: str,
    events: list[dict],
) -> dict | None:
    settings = load_json(workdir / "settings" / f"{asset_id}.json")
    accepted = load_json(workdir / "accepted" / f"{asset_id}.json")
    if not settings and not accepted and not events:
        return None
    return {
        "kind": "asset",
        "asset_id": asset_id,
        "current_settings": settings,
        "accepted": accepted,
        "events": events,
    }


def collect_profiles(workdir: Path) -> list[dict]:
    out: list[dict] = []
    profiles_dir = workdir / "profiles"
    if not profiles_dir.is_dir():
        return out
    for f in sorted(profiles_dir.glob("*.json")):
        p = load_json(f)
        if not p:
            continue
        rec = {"kind": "profile"}
        rec.update(p)
        out.append(rec)
    return out


def build_meta(
    workdir: Path,
    asset_count: int,
    profile_count: int,
    event_count: int,
) -> dict:
    return {
        "kind": "meta",
        "schema_version": EXPORT_SCHEMA_VERSION,
        "exported_at": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "workdir": str(workdir),
        "thumbnail_path_format": "relative-to-workdir",
        "record_counts": {
            "assets": asset_count,
            "profiles": profile_count,
            "events": event_count,
        },
    }


def export(workdir: Path, out) -> tuple[int, int, int]:
    events_by_asset, total_events = collect_events_by_asset(workdir)
    asset_ids = enumerate_asset_ids(workdir, events_by_asset)

    asset_count = 0
    for asset_id in asset_ids:
        rec = build_asset_record(workdir, asset_id, events_by_asset.get(asset_id, []))
        if rec is None:
            continue
        out.write(json.dumps(rec) + "\n")
        asset_count += 1

    profiles = collect_profiles(workdir)
    for rec in profiles:
        out.write(json.dumps(rec) + "\n")

    meta = build_meta(workdir, asset_count, len(profiles), total_events)
    out.write(json.dumps(meta) + "\n")

    return asset_count, len(profiles), total_events


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--dir", help="Workdir (default: ~/.glb-optimizer)")
    parser.add_argument("--out", default="-", help="Output JSONL path (default: stdout)")
    parser.add_argument("--self-test", action="store_true", help="Run a smoke test against a temp workdir")
    args = parser.parse_args(argv)

    if args.self_test:
        return 0 if self_test() else 1

    workdir = expand_workdir(args.dir)
    if not workdir.is_dir():
        print(f"export: workdir does not exist: {workdir}", file=sys.stderr)
        return 1

    if args.out == "-":
        a, p, e = export(workdir, sys.stdout)
    else:
        with open(args.out, "w", encoding="utf-8") as f:
            a, p, e = export(workdir, f)

    print(
        f"export: wrote {a} asset records, {p} profile records, {e} events from {workdir}",
        file=sys.stderr,
    )
    return 0


def self_test() -> bool:
    """Build a tempdir mimicking the workdir layout, run the exporter,
    and assert the JSONL has the expected shape."""
    with tempfile.TemporaryDirectory() as tmp:
        wd = Path(tmp)
        for sub in ("originals", "settings", "accepted", "accepted/thumbs", "tuning", "profiles"):
            (wd / sub).mkdir(parents=True, exist_ok=True)

        asset_id = "abc123"
        (wd / "originals" / f"{asset_id}.glb").write_bytes(b"glb")
        (wd / "settings" / f"{asset_id}.json").write_text(json.dumps({
            "schema_version": 1, "bake_exposure": 1.0,
        }))
        (wd / "accepted" / f"{asset_id}.json").write_text(json.dumps({
            "schema_version": 1,
            "asset_id": asset_id,
            "accepted_at": "2026-04-07T00:00:00Z",
            "comment": "ship it",
            "thumbnail_path": f"accepted/thumbs/{asset_id}.jpg",
            "settings": {"schema_version": 1, "bake_exposure": 1.0},
        }))
        (wd / "accepted" / "thumbs" / f"{asset_id}.jpg").write_bytes(b"\xff\xd8\xff\xe0")
        session = "00000000-0000-4000-8000-000000000000"
        with (wd / "tuning" / f"{session}.jsonl").open("w") as f:
            f.write(json.dumps({
                "schema_version": 1, "event_type": "session_start",
                "timestamp": "2026-04-07T00:00:00Z",
                "session_id": session, "asset_id": asset_id, "payload": {"trigger": "open_asset"},
            }) + "\n")
            f.write(json.dumps({
                "schema_version": 1, "event_type": "accept",
                "timestamp": "2026-04-07T00:01:00Z",
                "session_id": session, "asset_id": asset_id,
                "payload": {"settings": {}, "thumbnail_path": f"accepted/thumbs/{asset_id}.jpg"},
            }) + "\n")
            f.write("not valid json\n")  # exercise the per-line tolerance
        (wd / "profiles" / "round-bushes.json").write_text(json.dumps({
            "schema_version": 1, "name": "round-bushes",
            "comment": "warm light",
            "created_at": "2026-04-07T00:00:00Z",
            "source_asset_id": asset_id,
            "settings": {"schema_version": 1, "bake_exposure": 1.0},
        }))

        out_path = wd / "out.jsonl"
        with out_path.open("w") as f:
            export(wd, f)

        lines = out_path.read_text().splitlines()
        if not lines:
            print("self_test: no output", file=sys.stderr)
            return False
        records = [json.loads(l) for l in lines]
        kinds = [r["kind"] for r in records]
        if kinds.count("asset") != 1:
            print(f"self_test: expected 1 asset record, got {kinds.count('asset')}", file=sys.stderr)
            return False
        if kinds.count("profile") != 1:
            print(f"self_test: expected 1 profile record, got {kinds.count('profile')}", file=sys.stderr)
            return False
        if kinds[-1] != "meta":
            print(f"self_test: meta record should be last, got {kinds[-1]}", file=sys.stderr)
            return False
        asset_rec = next(r for r in records if r["kind"] == "asset")
        if asset_rec["asset_id"] != asset_id:
            print("self_test: asset_id mismatch", file=sys.stderr)
            return False
        if not asset_rec["accepted"]:
            print("self_test: accepted block missing", file=sys.stderr)
            return False
        if asset_rec["accepted"].get("thumbnail_path") != f"accepted/thumbs/{asset_id}.jpg":
            print("self_test: thumbnail_path missing or wrong", file=sys.stderr)
            return False
        if not asset_rec["events"]:
            print("self_test: events array empty", file=sys.stderr)
            return False
        if not any(e.get("event_type") == "accept" for e in asset_rec["events"]):
            print("self_test: no accept event in events array", file=sys.stderr)
            return False
        meta_rec = records[-1]
        if meta_rec["record_counts"]["assets"] != 1:
            print("self_test: meta asset count wrong", file=sys.stderr)
            return False
        if meta_rec["record_counts"]["profiles"] != 1:
            print("self_test: meta profile count wrong", file=sys.stderr)
            return False
        # 2 valid events + 1 corrupt skipped = 2
        if meta_rec["record_counts"]["events"] != 2:
            print(f"self_test: meta event count = {meta_rec['record_counts']['events']}, want 2", file=sys.stderr)
            return False

    print("self_test: PASS")
    return True


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
