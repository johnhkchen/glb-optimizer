# Default: build and run
default: run

# Build the binary
build:
    go build -o glb-optimizer .

# Build and run on default port
run: build
    ./glb-optimizer

# Run on a custom port
serve port="8787": build
    ./glb-optimizer --port {{port}}

# Stop any running glb-optimizer dev server
down:
    @pkill -f "(^|/)glb-optimizer( |$)" && echo "✓ stopped glb-optimizer" || echo "no glb-optimizer process found"

# Remove build artifacts
clean:
    rm -f glb-optimizer

# Remove all built asset packs from dist/plants/ (does not touch outputs/)
clean-packs:
    rm -rf dist/plants
    @mkdir -p dist/plants
    @echo "✓ cleaned dist/plants/"

# Show stale packs in ~/.glb-optimizer/dist/plants/ that no longer
# have a matching source intermediate (dry-run; nothing is deleted).
clean-stale-packs: build
    ./glb-optimizer clean-stale-packs

# Actually delete the stale packs identified above. No undo —
# always run `just clean-stale-packs` first to audit.
clean-stale-packs-apply: build
    ./glb-optimizer clean-stale-packs --apply

# Pack a single asset by id (writes ~/.glb-optimizer/dist/plants/{species}.glb)
pack id: build
    ./glb-optimizer pack {{id}}

# Pack every asset that has a baked side billboard. Walks
# ~/.glb-optimizer/outputs/, runs combine sequentially, prints a
# summary table, and exits non-zero if any pack failed or
# exceeded the 5 MiB cap. Demo-day refresh recipe.
pack-all: build
    ./glb-optimizer pack-all

# Full bake pipeline: start server if needed, run headless bake + pack, stop server.
# Requires: Playwright installed (just bake-install), gltfpack on PATH.
# Example: just bake inbox/dahlia_blush.glb
#          just bake inbox/dahlia_blush.glb --json
bake source json="":
    #!/usr/bin/env bash
    set -euo pipefail
    PORT=8787
    SERVER_PID=""
    JSON_FLAG="{{json}}"
    log() { if [ -n "$JSON_FLAG" ]; then echo "$1" >&2; else echo "$1"; fi; }
    cleanup() {
        if [ -n "$SERVER_PID" ]; then
            kill "$SERVER_PID" 2>/dev/null && log "[just bake] stopped server (pid $SERVER_PID)" || true
        fi
    }
    trap cleanup EXIT
    # Check if server is already running
    if curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
        log "[just bake] server already running on port $PORT"
    else
        log "[just bake] starting server on port $PORT..."
        go run . --port "$PORT" &
        SERVER_PID=$!
        # Poll until server is ready (max 15s)
        for i in $(seq 1 30); do
            if curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
                log "[just bake] server ready (pid $SERVER_PID)"
                break
            fi
            if ! kill -0 "$SERVER_PID" 2>/dev/null; then
                log "[just bake] ERROR: server process exited unexpectedly"
                exit 1
            fi
            sleep 0.5
        done
        if ! curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
            log "[just bake] ERROR: server not ready after 15s"
            exit 1
        fi
    fi
    # Run headless bake (which also builds the pack via the UI)
    cd scripts && npx tsx headless-bake.ts "../{{source}}" --headless --port "$PORT" {{json}}

# Batch bake: process every .glb in an inbox directory sequentially.
# Reuses a single server + browser instance across all assets.
# Successfully baked files are moved to inbox/done/.
# Example: just bake-all            (uses inbox/)
#          just bake-all my-models/  (custom inbox)
#          just bake-all --json      (structured output)
bake-all inbox="inbox" json="":
    #!/usr/bin/env bash
    set -euo pipefail
    PORT=8787
    SERVER_PID=""
    JSON_FLAG="{{json}}"
    log() { if [ -n "$JSON_FLAG" ]; then echo "$1" >&2; else echo "$1"; fi; }
    cleanup() {
        if [ -n "$SERVER_PID" ]; then
            kill "$SERVER_PID" 2>/dev/null && log "[just bake-all] stopped server (pid $SERVER_PID)" || true
        fi
    }
    trap cleanup EXIT
    # Check if server is already running
    if curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
        log "[just bake-all] server already running on port $PORT"
    else
        log "[just bake-all] starting server on port $PORT..."
        go run . --port "$PORT" &
        SERVER_PID=$!
        # Poll until server is ready (max 15s)
        for i in $(seq 1 30); do
            if curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
                log "[just bake-all] server ready (pid $SERVER_PID)"
                break
            fi
            if ! kill -0 "$SERVER_PID" 2>/dev/null; then
                log "[just bake-all] ERROR: server process exited unexpectedly"
                exit 1
            fi
            sleep 0.5
        done
        if ! curl -sf "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
            log "[just bake-all] ERROR: server not ready after 15s"
            exit 1
        fi
    fi
    # Run batch bake
    cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT" {{json}}

# Run the headless bake in headed mode (browser visible for debugging).
bake-debug source:
    cd scripts && npx tsx headless-bake.ts "../{{source}}"

# Show intermediate completeness for all assets in outputs/.
bake-status: build
    ./glb-optimizer bake-status

# One-time setup for Playwright + tsx. Re-run after pulling a package.json bump.
bake-install:
    @cd scripts && npm install && npx playwright install chromium
    @echo "✓ playwright + tsx ready"

# Verify a pack against Pack v1 (schema + scene graph). Accepts a
# species id (resolved to ~/.glb-optimizer/dist/plants/{id}.glb) or
# a path to a .glb. Used as the Phase 4 gate in the cross-repo
# handshake — run before USB-copying any pack to plantastic.
verify-pack arg:
    @if echo "{{arg}}" | grep -Eq '^[a-z][a-z0-9_]*$'; then \
        node scripts/verify-pack.mjs "$HOME/.glb-optimizer/dist/plants/{{arg}}.glb"; \
    else \
        node scripts/verify-pack.mjs "{{arg}}"; \
    fi

# Run the verifier's own shell test suite (synthetic fixtures).
verify-pack-test:
    @bash scripts/test-verify-pack.sh

# One-time setup for the node verifier deps. Re-run after pulling a
# package.json bump.
verify-pack-install:
    @cd scripts && npm install

# Validate Blender-produced intermediates against a known-good reference.
# Runs checks 1-6 from T-014-06. Checks 7-8 are manual.
validate id ref="1e562361be18ea9606222f8dcf81849d": build
    bash scripts/validate-blender-output.sh {{id}} --ref {{ref}}

# Check that dependencies are installed
check:
    @which go > /dev/null 2>&1 && echo "✓ go found: $(go version)" || echo "✗ go not found"
    @which gltfpack > /dev/null 2>&1 && echo "✓ gltfpack found: $(gltfpack -v 2>&1 | head -1)" || echo "✗ gltfpack not found — see README for install instructions"
    @blender -b --python-expr "import bpy; print('✓ blender found: ' + '.'.join(map(str, bpy.app.version)))" 2>/dev/null | grep '✓' || echo "✗ blender not found (optional — enables high-quality remesh LODs)"
