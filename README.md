# GLB Optimizer

A localhost web app for optimizing `.glb` 3D model files using [gltfpack](https://github.com/zeux/meshoptimizer). Provides a browser UI with drag-and-drop upload, interactive 3D preview, and configurable compression settings.

## Prerequisites

Install [gltfpack](https://meshoptimizer.org/gltf/):

```bash
# Download pre-built binary (recommended — supports texture compression)
curl -L https://github.com/zeux/meshoptimizer/releases/latest/download/gltfpack-macos.zip > gltfpack.zip
unzip -o gltfpack.zip && chmod a+x gltfpack && sudo mv gltfpack /usr/local/bin/
# If macOS blocks it, run: xattr -d com.apple.quarantine /usr/local/bin/gltfpack
```

Or via npm (no texture compression support):

```bash
npm install -g gltfpack
```

Verify it's installed:

```bash
gltfpack --version
```

## Build & Run

```bash
go build -o glb-optimizer .
./glb-optimizer
```

Opens automatically at [http://localhost:8787](http://localhost:8787).

### Options

```bash
./glb-optimizer --port 9000           # custom port
./glb-optimizer --dir /tmp/glb-work   # custom working directory (default: ~/.glb-optimizer)
```

## Usage

1. Drop `.glb` files onto the left panel (or click Browse)
2. Pick a preset (Max Quality / Balanced / Smallest) or tweak settings manually
3. Click **Process All**
4. Switch between Original and Optimized in the 3D preview to compare
5. Download individual files or all as a zip
