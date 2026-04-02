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

# Remove build artifacts
clean:
    rm -f glb-optimizer

# Check that dependencies are installed
check:
    @which go > /dev/null 2>&1 && echo "✓ go found: $(go version)" || echo "✗ go not found"
    @which gltfpack > /dev/null 2>&1 && echo "✓ gltfpack found: $(gltfpack -v 2>&1 | head -1)" || echo "✗ gltfpack not found — see README for install instructions"
    @blender -b --python-expr "import bpy; print('✓ blender found: ' + '.'.join(map(str, bpy.app.version)))" 2>/dev/null | grep '✓' || echo "✗ blender not found (optional — enables high-quality remesh LODs)"
