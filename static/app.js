import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { KTX2Loader } from 'three/addons/loaders/KTX2Loader.js';
import { MeshoptDecoder } from 'three/addons/libs/meshopt_decoder.module.js';
import { GLTFExporter } from 'three/addons/exporters/GLTFExporter.js';

// ── State ──
let files = [];
let selectedFileId = null;
let previewVersion = 'original';
let wireframeEnabled = false;
let scene, camera, renderer, controls, loader;
let currentModel = null;
let threeReady = false;
let stressInstances = [];
let stressActive = false;
let lastModelUrl = null;
let lastModelSize = 0;
let modelTriCount = 0;
let modelBBox = null; // cached bounding box of loaded model
let originalModelBBox = null; // bounding box from original version, used for stress test spacing
let blenderAvailable = false;

// ── DOM refs ──
const dropZone = document.getElementById('dropZone');
const browseBtn = document.getElementById('browseBtn');
const fileInput = document.getElementById('fileInput');
const fileList = document.getElementById('fileList');
const processAllBtn = document.getElementById('processAllBtn');
const downloadAllBtn = document.getElementById('downloadAllBtn');
const previewToolbar = document.getElementById('previewToolbar');
const previewCanvas = document.getElementById('preview-canvas');
const previewPlaceholder = document.getElementById('previewPlaceholder');
const previewStats = document.getElementById('previewStats');
const btnOriginal = document.getElementById('btnOriginal');
const btnOptimized = document.getElementById('btnOptimized');
const wireframeBtn = document.getElementById('wireframeBtn');
const statTriangles = document.getElementById('statTriangles');
const statVertices = document.getElementById('statVertices');
const statSize = document.getElementById('statSize');
const lodToggle = document.getElementById('lodToggle');
const generateLodsBtn = document.getElementById('generateLodsBtn');
const generateBillboardBtn = document.getElementById('generateBillboardBtn');
const generateBlenderLodsBtn = document.getElementById('generateBlenderLodsBtn');

const simplificationSlider = document.getElementById('simplification');
const simplificationValue = document.getElementById('simplificationValue');
const texQualityRow = document.getElementById('texQualityRow');
const textureQualitySlider = document.getElementById('textureQuality');
const texQualityValue = document.getElementById('texQualityValue');

// ── Helpers ──
function formatBytes(bytes) {
    if (bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + ' ' + units[i];
}

function getSettings() {
    return {
        simplification: parseFloat(simplificationSlider.value),
        compression: document.querySelector('input[name="compression"]:checked').value,
        texture_compression: document.querySelector('input[name="texCompression"]:checked').value,
        texture_quality: parseInt(textureQualitySlider.value),
        texture_size: parseInt(document.getElementById('textureSize').value),
        keep_nodes: document.getElementById('keepNodes').checked,
        keep_materials: document.getElementById('keepMaterials').checked,
        float_positions: document.getElementById('floatPositions').checked,
        aggressive_simplify: document.getElementById('aggressiveSimplify').checked,
        permissive_simplify: document.getElementById('permissiveSimplify').checked,
        lock_borders: document.getElementById('lockBorders').checked,
    };
}

function applyPreset(name) {
    const defaults = {
        keepNodes: false, keepMaterials: false, floatPositions: false,
        aggressive: false, permissive: false, lockBorders: false,
    };
    const presets = {
        quality: {
            ...defaults,
            simplification: 1.0, compression: 'cc', texCompression: '', texQuality: 8, texSize: '0',
        },
        balanced: {
            ...defaults,
            simplification: 0.75, compression: 'cc', texCompression: 'tw', texQuality: 8, texSize: '1024',
        },
        smallest: {
            ...defaults,
            simplification: 0.5, compression: 'cz', texCompression: 'tw', texQuality: 5, texSize: '512',
        },
        scene: {
            ...defaults,
            simplification: 0.25, compression: 'cc', texCompression: 'tw', texQuality: 5, texSize: '512',
            aggressive: true, permissive: true,
        },
        mobile100: {
            ...defaults,
            simplification: 0.05, compression: 'cc', texCompression: 'tw', texQuality: 3, texSize: '256',
            aggressive: true, permissive: true,
        },
    };
    const p = presets[name];
    if (!p) return;

    simplificationSlider.value = p.simplification;
    simplificationValue.textContent = p.simplification.toFixed(2);
    document.querySelector(`input[name="compression"][value="${p.compression}"]`).checked = true;
    document.querySelector(`input[name="texCompression"][value="${p.texCompression}"]`).checked = true;
    textureQualitySlider.value = p.texQuality;
    texQualityValue.textContent = p.texQuality;
    document.getElementById('textureSize').value = p.texSize;
    document.getElementById('keepNodes').checked = p.keepNodes;
    document.getElementById('keepMaterials').checked = p.keepMaterials;
    document.getElementById('floatPositions').checked = p.floatPositions;
    document.getElementById('aggressiveSimplify').checked = p.aggressive;
    document.getElementById('permissiveSimplify').checked = p.permissive;
    document.getElementById('lockBorders').checked = p.lockBorders;
    texQualityRow.style.display = p.texCompression ? 'block' : 'none';
}

// ── API ──
async function uploadFiles(fileObjects) {
    const formData = new FormData();
    for (const f of fileObjects) formData.append('files', f);
    try {
        const res = await fetch('/api/upload', { method: 'POST', body: formData });
        const newFiles = await res.json();
        if (Array.isArray(newFiles)) { files.push(...newFiles); renderFileList(); }
    } catch (err) { console.error('Upload failed:', err); }
}

async function refreshFiles() {
    try {
        const res = await fetch('/api/files');
        files = await res.json();
        if (!Array.isArray(files)) files = [];
        renderFileList();
    } catch (err) { console.error('Failed to fetch files:', err); }
}

async function processFile(id) {
    const settings = getSettings();
    store_update(id, f => f.status = 'processing');
    renderFileList();
    try {
        const res = await fetch(`/api/process/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings),
        });
        const result = await res.json();
        const idx = files.findIndex(f => f.id === id);
        if (idx !== -1) files[idx] = result;
    } catch (err) {
        console.error('Process failed:', err);
        store_update(id, f => f.status = 'error');
    }
    renderFileList();
    updatePreviewButtons();
}

function store_update(id, fn) {
    const f = files.find(f => f.id === id);
    if (f) fn(f);
}

window._processFile = processFile;

async function processAll() {
    const settings = getSettings();
    processAllBtn.disabled = true;
    processAllBtn.textContent = 'Processing...';
    files.forEach(f => { if (f.status === 'pending') f.status = 'processing'; });
    renderFileList();
    try {
        const res = await fetch('/api/process-all', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings),
        });
        const results = await res.json();
        if (Array.isArray(results)) {
            for (const r of results) {
                const idx = files.findIndex(f => f.id === r.id);
                if (idx !== -1) files[idx] = r;
            }
        }
    } catch (err) { console.error('Process all failed:', err); }
    processAllBtn.textContent = 'Process All';
    renderFileList();
    updatePreviewButtons();
}

async function deleteFile(id) {
    try { await fetch(`/api/files/${id}`, { method: 'DELETE' }); } catch (err) { console.error(err); }
    files = files.filter(f => f.id !== id);
    if (selectedFileId === id) { selectedFileId = null; hidePreview(); }
    renderFileList();
}

window._deleteFile = deleteFile;

// ── LOD Generation ──
async function generateLODs(id) {
    const settings = getSettings();
    generateLodsBtn.textContent = 'Generating...';
    generateLodsBtn.classList.add('generating');
    generateLodsBtn.disabled = true;
    try {
        const res = await fetch(`/api/generate-lods/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings),
        });
        const result = await res.json();
        const idx = files.findIndex(f => f.id === id);
        if (idx !== -1) files[idx] = result;
        updatePreviewButtons();
    } catch (err) { console.error('LOD generation failed:', err); }
    generateLodsBtn.textContent = 'LODs (gltfpack)';
    generateLodsBtn.classList.remove('generating');
    generateLodsBtn.disabled = false;
}

async function generateBlenderLODs(id) {
    generateBlenderLodsBtn.textContent = 'Remeshing...';
    generateBlenderLodsBtn.classList.add('generating');
    generateBlenderLodsBtn.disabled = true;
    try {
        const res = await fetch(`/api/generate-blender-lods/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({}),
        });
        const result = await res.json();
        const idx = files.findIndex(f => f.id === id);
        if (idx !== -1) files[idx] = result;
        renderFileList();
        updatePreviewButtons();
    } catch (err) { console.error('Blender LOD generation failed:', err); }
    generateBlenderLodsBtn.textContent = 'LODs (Blender)';
    generateBlenderLodsBtn.classList.remove('generating');
    generateBlenderLodsBtn.disabled = false;
}

// ── Billboard Impostor Generation ──
const BILLBOARD_ANGLES = 6; // render from 6 evenly-spaced angles
let billboardVariants = []; // stored { geometry, material, quadWidth, quadHeight } per angle

async function generateBillboard(id) {
    if (!currentModel || !threeReady) return;

    generateBillboardBtn.textContent = 'Rendering...';
    generateBillboardBtn.classList.add('generating');
    generateBillboardBtn.disabled = true;

    try {
        const glb = await renderMultiAngleBillboardGLB(currentModel, BILLBOARD_ANGLES);
        await fetch(`/api/upload-billboard/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: glb,
        });

        store_update(id, f => f.has_billboard = true);
        updatePreviewButtons();
    } catch (err) { console.error('Billboard generation failed:', err); }

    generateBillboardBtn.textContent = 'Billboard';
    generateBillboardBtn.classList.remove('generating');
    generateBillboardBtn.disabled = false;
}

function renderBillboardAngle(model, angleRad, resolution) {
    // Compute model bounds
    const box = new THREE.Box3().setFromObject(model);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.y, size.z);

    const offRenderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;

    // Ortho camera sized to fit model
    const halfH = size.y * 0.55;
    const halfW = Math.max(size.x, size.z) * 0.55;
    const offCamera = new THREE.OrthographicCamera(-halfW, halfW, halfH, -halfH, 0.01, maxDim * 10);

    // Position camera around the model at the given angle
    const dist = maxDim * 2;
    offCamera.position.set(
        center.x + Math.sin(angleRad) * dist,
        center.y,
        center.z + Math.cos(angleRad) * dist
    );
    offCamera.lookAt(center);

    const offScene = new THREE.Scene();
    offScene.add(new THREE.AmbientLight(0xffffff, 0.6));
    const dl = new THREE.DirectionalLight(0xffffff, 1.0);
    dl.position.set(5, 10, 7);
    offScene.add(dl);
    const dl2 = new THREE.DirectionalLight(0xffffff, 0.3);
    dl2.position.set(-5, 5, -5);
    offScene.add(dl2);

    const clone = model.clone(true);
    offScene.add(clone);
    offRenderer.render(offScene, offCamera);

    // Extract image as blob
    const canvas = offRenderer.domElement;
    // Create a copy of the canvas data before disposing
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);

    offRenderer.dispose();

    return { canvas: copyCanvas, quadWidth: halfW * 2, quadHeight: halfH * 2, center, boxMinY: box.min.y };
}

function renderBillboardTopDown(model, resolution) {
    const box = new THREE.Box3().setFromObject(model);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.z);

    const offRenderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;

    const halfW = size.x * 0.55;
    const halfD = size.z * 0.55;
    const half = Math.max(halfW, halfD);
    const offCamera = new THREE.OrthographicCamera(-half, half, half, -half, 0.01, size.y * 10);
    offCamera.position.set(center.x, center.y + size.y * 2, center.z);
    offCamera.lookAt(center);

    const offScene = new THREE.Scene();
    offScene.add(new THREE.AmbientLight(0xffffff, 0.7));
    const dl = new THREE.DirectionalLight(0xffffff, 0.8);
    dl.position.set(0, 10, 0);
    offScene.add(dl);

    const clone = model.clone(true);
    offScene.add(clone);
    offRenderer.render(offScene, offCamera);

    const canvas = offRenderer.domElement;
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);
    offRenderer.dispose();

    return { canvas: copyCanvas, quadSize: half * 2 };
}

function renderMultiAngleBillboardGLB(model, numAngles) {
    return new Promise((resolve, reject) => {
        const resolution = 512;
        const exportScene = new THREE.Scene();

        for (let i = 0; i < numAngles; i++) {
            const angle = (i / numAngles) * Math.PI * 2;
            const { canvas, quadWidth, quadHeight, center, boxMinY } = renderBillboardAngle(model, angle, resolution);

            const texture = new THREE.CanvasTexture(canvas);
            texture.colorSpace = THREE.SRGBColorSpace;

            // Geometry with origin at the BOTTOM edge so it sits on the ground
            const geometry = new THREE.PlaneGeometry(quadWidth, quadHeight);
            geometry.translate(0, quadHeight / 2, 0); // shift up so bottom = y:0

            const material = new THREE.MeshBasicMaterial({
                map: texture,
                transparent: true,
                side: THREE.DoubleSide,
                alphaTest: 0.1,
            });

            const quad = new THREE.Mesh(geometry, material);
            quad.name = `billboard_${i}`;
            // Space apart for preview; instancing ignores mesh position
            quad.position.set(i * quadWidth * 1.2, 0, 0);
            exportScene.add(quad);
        }

        // Add top-down cross quad (horizontal, for overhead viewing)
        const { canvas: topCanvas, quadSize } = renderBillboardTopDown(model, resolution);
        const topTex = new THREE.CanvasTexture(topCanvas);
        topTex.colorSpace = THREE.SRGBColorSpace;

        const topGeom = new THREE.PlaneGeometry(quadSize, quadSize);
        topGeom.rotateX(-Math.PI / 2); // lay flat on XZ plane

        const box = new THREE.Box3().setFromObject(model);
        const centerY = box.getCenter(new THREE.Vector3()).y;
        // Position at ~60% of model height for best visual blend
        topGeom.translate(0, 0, 0); // flat on ground plane at y=0

        const topMat = new THREE.MeshBasicMaterial({
            map: topTex,
            transparent: true,
            side: THREE.DoubleSide,
            alphaTest: 0.1,
        });

        const topQuad = new THREE.Mesh(topGeom, topMat);
        topQuad.name = 'billboard_top';
        // Place after the side variants for preview
        const lastVariant = exportScene.children[exportScene.children.length - 1];
        const previewOffset = lastVariant ? lastVariant.position.x + quadSize * 1.5 : 0;
        topQuad.position.set(previewOffset, 0, 0);
        exportScene.add(topQuad);

        const exporter = new GLTFExporter();
        exporter.parse(exportScene, (result) => {
            resolve(result);
        }, (err) => {
            reject(err);
        }, { binary: true });
    });
}

// ── File List Rendering ──
function renderFileList() {
    fileList.innerHTML = '';
    let hasPending = false;
    let hasDone = false;

    for (const f of files) {
        if (f.status === 'pending') hasPending = true;
        if (f.status === 'done') hasDone = true;

        const div = document.createElement('div');
        div.className = 'file-item' + (f.id === selectedFileId ? ' selected' : '');
        div.onclick = () => selectFile(f.id);

        let metaHTML = `<span class="status-badge status-${f.status}">${f.status}</span>`;
        metaHTML += ` <span>${formatBytes(f.original_size)}</span>`;

        let extraHTML = '';
        if (f.status === 'done' && f.output_size) {
            const ratio = ((1 - f.output_size / f.original_size) * 100).toFixed(0);
            extraHTML = `<div class="result-info">${formatBytes(f.output_size)} — ${ratio}% smaller</div>`;
        }
        if (f.status === 'error' && f.error) {
            extraHTML = `<div class="error-info" title="${f.error}">${f.error}</div>`;
        }
        if (f.status === 'processing') {
            metaHTML += ` <span class="spinner"></span>`;
        }

        // Show LOD info
        if (f.lods && f.lods.length > 0) {
            let lodInfo = f.lods.map((l, i) => `L${i}:${formatBytes(l.size)}`).join(' ');
            if (f.has_billboard) lodInfo += ' +BB';
            extraHTML += `<div class="result-info" style="color:var(--text-muted)">${lodInfo}</div>`;
        }

        let commandHTML = '';
        if (f.command) {
            commandHTML = `<div class="command-info" title="Click to copy" onclick="event.stopPropagation(); navigator.clipboard.writeText('${f.command.replace(/'/g, "\\'")}')">${f.command}</div>`;
        }

        let actionsHTML = '';
        if (f.status === 'pending' || f.status === 'done' || f.status === 'error') {
            actionsHTML = `<button class="file-process-btn" onclick="event.stopPropagation(); window._processFile('${f.id}')">${f.status === 'done' ? 'Reprocess' : 'Process'}</button>`;
        }

        div.innerHTML = `
            <div class="filename" title="${f.filename}">${f.filename}</div>
            <div class="file-meta">${metaHTML}</div>
            ${extraHTML}
            ${actionsHTML}
            ${commandHTML}
            <button class="remove-btn" onclick="event.stopPropagation(); window._deleteFile('${f.id}')">&times;</button>
        `;
        fileList.appendChild(div);
    }

    processAllBtn.disabled = !hasPending;
    downloadAllBtn.style.display = hasDone ? 'block' : 'none';
}

// ── Three.js Setup ──
function initThreeJS() {
    scene = new THREE.Scene();
    camera = new THREE.PerspectiveCamera(45, 1, 0.01, 1000);
    camera.position.set(2, 2, 2);

    renderer = new THREE.WebGLRenderer({ canvas: previewCanvas, antialias: true });
    renderer.setClearColor(0x1a1a2e);
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.outputColorSpace = THREE.SRGBColorSpace;
    renderer.toneMapping = THREE.ACESFilmicToneMapping;
    renderer.toneMappingExposure = 1.0;

    controls = new OrbitControls(camera, previewCanvas);
    controls.enableDamping = true;
    controls.dampingFactor = 0.1;

    scene.add(new THREE.AmbientLight(0xffffff, 0.5));
    const dirLight = new THREE.DirectionalLight(0xffffff, 1.0);
    dirLight.position.set(5, 10, 7);
    scene.add(dirLight);
    const dirLight2 = new THREE.DirectionalLight(0xffffff, 0.3);
    dirLight2.position.set(-5, 5, -5);
    scene.add(dirLight2);

    scene.add(new THREE.GridHelper(10, 20, 0x2a2a4a, 0x1a1a3e));

    loader = new GLTFLoader();
    loader.setMeshoptDecoder(MeshoptDecoder);

    const ktx2Loader = new KTX2Loader();
    ktx2Loader.setTranscoderPath('https://unpkg.com/three@0.160.0/examples/jsm/libs/basis/');
    ktx2Loader.detectSupport(renderer);
    loader.setKTX2Loader(ktx2Loader);

    threeReady = true;
    resizeRenderer();
    animate();
}

function resizeRenderer() {
    if (!renderer) return;
    const container = document.querySelector('.panel-center');
    const w = container.clientWidth;
    const h = container.clientHeight - (previewToolbar.offsetHeight || 0);
    renderer.setSize(w, h);
    camera.aspect = w / h;
    camera.updateProjectionMatrix();
}

let fpsFrames = 0;
let fpsLastTime = performance.now();

function animate() {
    requestAnimationFrame(animate);
    if (controls) controls.update();
    if (renderer && scene && camera) renderer.render(scene, camera);

    // Billboard updates: camera-facing + overhead visibility swap
    if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
        updateBillboardFacing();
        updateBillboardVisibility();
    }

    if (stressActive) {
        fpsFrames++;
        const now = performance.now();
        if (now - fpsLastTime >= 1000) {
            const fps = Math.round(fpsFrames * 1000 / (now - fpsLastTime));
            document.getElementById('fpsValue').textContent = fps;
            document.getElementById('drawCalls').textContent = renderer.info.render.calls.toLocaleString();

            const el = document.getElementById('fpsValue');
            if (fps >= 50) el.style.color = '#4caf50';
            else if (fps >= 25) el.style.color = '#ff9800';
            else el.style.color = '#f44336';

            fpsFrames = 0;
            fpsLastTime = now;
        }
    }
}

window.addEventListener('resize', resizeRenderer);

// ── Preview ──
function clearStressInstances() {
    for (const inst of stressInstances) {
        scene.remove(inst);
        if (inst.dispose) inst.dispose();
    }
    stressInstances = [];
    billboardInstances = [];
    billboardTopInstances = [];
    stressActive = false;
    if (currentModel) currentModel.visible = true;
    document.getElementById('fpsOverlay').style.display = 'none';
    document.getElementById('stressCount').value = 1;
    document.getElementById('stressCountValue').textContent = '1x';
}

function getModelStats(model) {
    let triangles = 0, vertices = 0;
    model.traverse((child) => {
        if (child.isMesh && child.geometry) {
            if (child.geometry.index) triangles += child.geometry.index.count / 3;
            else if (child.geometry.attributes.position) triangles += child.geometry.attributes.position.count / 3;
            if (child.geometry.attributes.position) vertices += child.geometry.attributes.position.count;
        }
    });
    return { triangles, vertices };
}

function frameCamera(model) {
    const box = new THREE.Box3().setFromObject(model);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.y, size.z);
    const fov = camera.fov * (Math.PI / 180);
    let cameraZ = maxDim / (2 * Math.tan(fov / 2)) * 1.5;

    camera.position.set(center.x + cameraZ * 0.5, center.y + cameraZ * 0.3, center.z + cameraZ);
    controls.target.copy(center);
    camera.near = maxDim * 0.001;
    camera.far = maxDim * 1000;
    camera.updateProjectionMatrix();
    controls.update();
    modelBBox = { box, center, size, maxDim };
}

function loadModel(url, fileSize) {
    if (!threeReady) return;
    clearStressInstances();
    if (currentModel) { scene.remove(currentModel); currentModel = null; }

    lastModelUrl = url;
    lastModelSize = fileSize;

    loader.load(url, (gltf) => {
        currentModel = gltf.scene;
        scene.add(currentModel);
        frameCamera(currentModel);

        // Cache original bbox for stress test spacing
        if (previewVersion === 'original') {
            originalModelBBox = { ...modelBBox };
        }

        const stats = getModelStats(currentModel);
        modelTriCount = stats.triangles;
        statTriangles.textContent = stats.triangles.toLocaleString();
        statVertices.textContent = stats.vertices.toLocaleString();
        statSize.textContent = formatBytes(fileSize);
        previewStats.style.display = 'block';

        if (wireframeEnabled) setWireframe(true);
    });
}

// ── Stress Test ──
// seededRandom gives deterministic "random" per index so stress test is reproducible
function seededRandom(i) {
    let x = Math.sin(i * 127.1 + 311.7) * 43758.5453;
    return x - Math.floor(x);
}

function createInstancedFromModel(model, count, positions, randomRotateY = false) {
    const meshes = [];
    model.traverse((child) => { if (child.isMesh) meshes.push(child); });

    const dummy = new THREE.Object3D();
    const modelInverse = new THREE.Matrix4().copy(model.matrixWorld).invert();
    const created = [];

    for (const mesh of meshes) {
        const instancedMesh = new THREE.InstancedMesh(mesh.geometry, mesh.material, count);
        instancedMesh.frustumCulled = false;
        const localMatrix = new THREE.Matrix4().multiplyMatrices(modelInverse, mesh.matrixWorld.clone());

        for (let i = 0; i < count; i++) {
            dummy.position.copy(positions[i]);
            dummy.rotation.set(0, randomRotateY ? seededRandom(i) * Math.PI * 2 : 0, 0);
            dummy.scale.set(1, 1, 1);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, new THREE.Matrix4().multiplyMatrices(dummy.matrix, localMatrix));
        }
        instancedMesh.instanceMatrix.needsUpdate = true;
        scene.add(instancedMesh);
        created.push(instancedMesh);
    }
    return created;
}

// Billboard instances that need camera-facing updates each frame
let billboardInstances = []; // { mesh: InstancedMesh, positions: Vector3[] } — vertical, camera-facing
let billboardTopInstances = []; // { mesh: InstancedMesh } — horizontal top-down quads

function createBillboardInstances(model, positions) {
    // Billboard GLB has N side quads + 1 top-down quad (named 'billboard_top').
    // Side quads: random variant per instance, Y-axis camera facing.
    // Top quad: horizontal, visible when camera is overhead.
    const sideQuads = [];
    let topQuad = null;
    model.traverse((child) => {
        if (!child.isMesh) return;
        if (child.name === 'billboard_top') topQuad = child;
        else sideQuads.push(child);
    });
    if (sideQuads.length === 0) return [];

    const numVariants = sideQuads.length;
    const variantPositions = Array.from({ length: numVariants }, () => []);
    for (let i = 0; i < positions.length; i++) {
        const variant = Math.floor(seededRandom(i + 9999) * numVariants);
        variantPositions[variant].push(positions[i]);
    }

    const created = [];
    const dummy = new THREE.Object3D();

    // Create side billboard instances (camera-facing)
    // geometry.translate() survives GLTF roundtrip — bottom is already at y=0, no offset needed
    for (let v = 0; v < numVariants; v++) {
        const posArr = variantPositions[v];
        if (posArr.length === 0) continue;

        const quad = sideQuads[v];
        const geom = quad.geometry.clone();
        const mat = quad.material.clone();
        mat.depthWrite = true;
        mat.alphaTest = 0.5;
        mat.transparent = false;
        const instancedMesh = new THREE.InstancedMesh(geom, mat, posArr.length);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < posArr.length; i++) {
            dummy.position.copy(posArr[i]);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, dummy.matrix);
        }
        instancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(instancedMesh);
        created.push(instancedMesh);
        billboardInstances.push({ mesh: instancedMesh, positions: posArr });
    }

    // Create top-down instances (horizontal, for overhead view)
    if (topQuad) {
        const topGeom = topQuad.geometry.clone();
        const topMat = topQuad.material.clone();
        topMat.depthWrite = true;
        topMat.alphaTest = 0.5;
        topMat.transparent = false;
        const topInstancedMesh = new THREE.InstancedMesh(topGeom, topMat, positions.length);
        topInstancedMesh.frustumCulled = false;

        for (let i = 0; i < positions.length; i++) {
            dummy.position.copy(positions[i]);
            // Random Y rotation for variety on the top-down view too
            dummy.rotation.set(0, seededRandom(i + 5555) * Math.PI * 2, 0);
            dummy.scale.set(1, 1, 1);
            dummy.updateMatrix();
            topInstancedMesh.setMatrixAt(i, dummy.matrix);
        }
        topInstancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(topInstancedMesh);
        created.push(topInstancedMesh);
        billboardTopInstances.push({ mesh: topInstancedMesh });
    }

    // Immediately face camera + set correct visibility
    updateBillboardFacing();
    updateBillboardVisibility();
    return created;
}

function updateBillboardFacing() {
    if (billboardInstances.length === 0) return;
    const camPos = camera.position;
    const dummy = new THREE.Object3D();

    for (const { mesh, positions } of billboardInstances) {
        for (let i = 0; i < positions.length; i++) {
            const pos = positions[i];
            dummy.position.copy(pos);
            dummy.rotation.set(0, Math.atan2(camPos.x - pos.x, camPos.z - pos.z), 0);
            dummy.scale.set(1, 1, 1);
            dummy.updateMatrix();
            mesh.setMatrixAt(i, dummy.matrix);
        }
        mesh.instanceMatrix.needsUpdate = true;
    }
}

function updateBillboardVisibility() {
    if (billboardInstances.length === 0 && billboardTopInstances.length === 0) return;

    const camDir = new THREE.Vector3();
    camera.getWorldDirection(camDir);
    const lookDownAmount = Math.abs(camDir.dot(new THREE.Vector3(0, -1, 0)));

    // Crossfade zone: 0.55–0.75 (side fades out, top fades in)
    const fadeStart = 0.55;
    const fadeEnd = 0.75;
    const t = THREE.MathUtils.clamp((lookDownAmount - fadeStart) / (fadeEnd - fadeStart), 0, 1);
    // Smooth easing
    const topOpacity = t * t * (3 - 2 * t);     // smoothstep
    const sideOpacity = 1 - topOpacity;

    for (const { mesh } of billboardInstances) {
        mesh.visible = sideOpacity > 0.01;
        if (mesh.visible) {
            mesh.material.opacity = sideOpacity;
            mesh.material.transparent = sideOpacity < 1;
        }
    }
    for (const { mesh } of billboardTopInstances) {
        mesh.visible = topOpacity > 0.01;
        if (mesh.visible) {
            mesh.material.opacity = topOpacity;
            mesh.material.transparent = topOpacity < 1;
        }
    }
}

function runStressTest(count, useLods, quality = 0.5) {
    if (!currentModel) return;
    clearStressInstances();
    stressActive = true;
    currentModel.visible = false;

    // Use original model bbox for consistent spacing regardless of which version is displayed
    const refBBox = originalModelBBox || modelBBox;
    const size = refBBox ? refBBox.size : new THREE.Box3().setFromObject(currentModel).getSize(new THREE.Vector3());
    const spacing = Math.max(size.x, size.z) * 1.3;
    const cols = Math.ceil(Math.sqrt(count));
    const gridWidth = cols * spacing;
    const totalRows = Math.ceil(count / cols);
    const gridDepth = totalRows * spacing;

    // Compute all grid positions
    const positions = [];
    for (let i = 0; i < count; i++) {
        const row = Math.floor(i / cols);
        const col = i % cols;
        positions.push(new THREE.Vector3(
            col * spacing - gridWidth / 2 + spacing / 2,
            0,
            row * spacing - gridDepth / 2 + spacing / 2
        ));
    }

    if (!useLods) {
        if (previewVersion === 'billboard') {
            // Billboard mode: always face camera
            const instances = createBillboardInstances(currentModel, positions);
            stressInstances.push(...instances);
        } else {
            // Regular mode: random Y rotation for variety
            const instances = createInstancedFromModel(currentModel, count, positions, true);
            stressInstances.push(...instances);
        }
    } else {
        // LOD mode: load LOD models and assign by distance from center
        runLodStressTest(count, positions, gridWidth, gridDepth, quality);
    }

    // Pull camera back
    const camDist = Math.max(gridWidth, gridDepth) * 1.2;
    camera.position.set(0, camDist * 0.6, camDist * 0.8);
    controls.target.set(0, 0, 0);
    camera.far = camDist * 10;
    camera.updateProjectionMatrix();
    controls.update();

    // Stats
    document.getElementById('fpsOverlay').style.display = 'block';
    document.getElementById('instanceCount').textContent = count.toLocaleString();
    document.getElementById('totalTris').textContent = (modelTriCount * count).toLocaleString();
    document.getElementById('estMemory').textContent = formatBytes(lastModelSize * count);
}

async function runLodStressTest(count, positions, gridWidth, gridDepth, quality = 0.5) {
    const file = files.find(f => f.id === selectedFileId);
    if (!file || !file.lods || file.lods.length === 0) {
        const instances = createInstancedFromModel(currentModel, count, positions, true);
        stressInstances.push(...instances);
        return;
    }

    // Quality slider controls LOD distribution:
    // quality=1.0 → almost all LOD0 (high detail)
    // quality=0.5 → even distribution
    // quality=0.0 → almost all billboard/LOD3 (low detail)
    // Thresholds shift based on quality: higher quality → more instances get higher-detail LODs
    const q = quality;
    const t0 = 0.05 + q * 0.45;  // LOD0 boundary: 0.05 (q=0) to 0.50 (q=1)
    const t1 = t0 + 0.1 + (1 - q) * 0.15; // LOD1 boundary
    const t2 = t1 + 0.1 + (1 - q) * 0.15; // LOD2 boundary
    const t3 = t2 + 0.1 + (1 - q) * 0.1;  // LOD3 boundary, rest = billboard

    const maxDist = Math.max(gridWidth, gridDepth) / 2;
    const buckets = { lod0: [], lod1: [], lod2: [], lod3: [], billboard: [] };

    for (let i = 0; i < positions.length; i++) {
        const dist = positions[i].length();
        const ratio = dist / (maxDist || 1);

        if (ratio < t0) buckets.lod0.push(positions[i]);
        else if (ratio < t1) buckets.lod1.push(positions[i]);
        else if (ratio < t2) buckets.lod2.push(positions[i]);
        else if (ratio < t3 || !file.has_billboard) buckets.lod3.push(positions[i]);
        else buckets.billboard.push(positions[i]);
    }

    let totalTris = 0;
    let totalMem = 0;

    const lodVersions = ['lod0', 'lod1', 'lod2', 'lod3', 'billboard'];
    for (const version of lodVersions) {
        const bucket = buckets[version];
        if (bucket.length === 0) continue;

        if (version === 'billboard' && !file.has_billboard) continue;
        if (version.startsWith('lod')) {
            const lodIdx = parseInt(version[3]);
            if (!file.lods[lodIdx] || file.lods[lodIdx].error) continue;
        }

        try {
            const url = `/api/preview/${selectedFileId}?version=${version}&t=${Date.now()}`;
            const gltf = await new Promise((resolve, reject) => {
                loader.load(url, resolve, undefined, reject);
            });

            const model = gltf.scene;
            model.updateMatrixWorld(true);
            const stats = getModelStats(model);

            let instances;
            if (version === 'billboard') {
                // Billboard: camera-facing instances with variant selection
                instances = createBillboardInstances(model, bucket);
            } else {
                // Mesh LOD: random Y rotation for visual variety
                instances = createInstancedFromModel(model, bucket.length, bucket, true);
            }
            stressInstances.push(...instances);

            totalTris += stats.triangles * bucket.length;
            if (version === 'billboard') {
                totalMem += 50000 * bucket.length;
            } else {
                const lodIdx = parseInt(version[3]);
                totalMem += (file.lods[lodIdx]?.size || 0) * bucket.length;
            }
        } catch (err) {
            console.error(`Failed to load ${version}:`, err);
        }
    }

    document.getElementById('totalTris').textContent = totalTris.toLocaleString();
    document.getElementById('estMemory').textContent = formatBytes(totalMem);
}

// ── Preview Controls ──
function showPreview() {
    previewPlaceholder.style.display = 'none';
    previewCanvas.style.display = 'block';
    previewToolbar.style.display = 'flex';
    resizeRenderer();
}

function hidePreview() {
    previewPlaceholder.style.display = 'flex';
    previewCanvas.style.display = 'none';
    previewToolbar.style.display = 'none';
    previewStats.style.display = 'none';
}

function selectFile(id) {
    selectedFileId = id;
    previewVersion = 'original';
    renderFileList();
    updatePreviewButtons();
    showPreview();
    const file = files.find(f => f.id === id);
    if (file) loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
}

function updatePreviewButtons() {
    const file = files.find(f => f.id === selectedFileId);
    btnOriginal.classList.toggle('active', previewVersion === 'original');
    btnOptimized.classList.toggle('active', previewVersion === 'optimized');
    btnOptimized.disabled = !(file && file.status === 'done');

    // LOD buttons
    const hasLods = file && file.lods && file.lods.length > 0;
    lodToggle.style.display = hasLods || (file && file.has_billboard) ? 'flex' : 'none';

    lodToggle.querySelectorAll('button').forEach(btn => {
        const lod = btn.dataset.lod;
        btn.classList.toggle('active', previewVersion === lod);
        if (lod === 'billboard') {
            btn.disabled = !(file && file.has_billboard);
        } else if (lod.startsWith('lod')) {
            const idx = parseInt(lod[3]);
            btn.disabled = !(hasLods && file.lods[idx] && !file.lods[idx].error);
        }
    });

    // Generate buttons: enabled when file exists
    generateLodsBtn.disabled = !file;
    generateBlenderLodsBtn.disabled = !file;
    generateBillboardBtn.disabled = !file || !currentModel;
}

function setWireframe(enabled) {
    if (!currentModel) return;
    currentModel.traverse((child) => {
        if (child.isMesh && child.material) {
            const mats = Array.isArray(child.material) ? child.material : [child.material];
            mats.forEach(m => m.wireframe = enabled);
        }
    });
}

// ── Event Listeners ──

fileInput.addEventListener('change', () => {
    if (fileInput.files.length > 0) { uploadFiles(Array.from(fileInput.files)); fileInput.value = ''; }
});

browseBtn.addEventListener('click', (e) => { e.preventDefault(); e.stopPropagation(); fileInput.click(); });

dropZone.addEventListener('dragover', (e) => { e.preventDefault(); dropZone.classList.add('drag-over'); });
dropZone.addEventListener('dragleave', () => { dropZone.classList.remove('drag-over'); });
dropZone.addEventListener('drop', (e) => {
    e.preventDefault();
    dropZone.classList.remove('drag-over');
    const droppedFiles = Array.from(e.dataTransfer.files).filter(f => f.name.toLowerCase().endsWith('.glb'));
    if (droppedFiles.length > 0) uploadFiles(droppedFiles);
});

processAllBtn.addEventListener('click', processAll);
downloadAllBtn.addEventListener('click', () => { window.location.href = '/api/download-all'; });

// Preview version toggle
btnOriginal.addEventListener('click', () => {
    previewVersion = 'original';
    updatePreviewButtons();
    const file = files.find(f => f.id === selectedFileId);
    if (file) loadModel(`/api/preview/${selectedFileId}?version=original&t=${Date.now()}`, file.original_size);
});

btnOptimized.addEventListener('click', () => {
    const file = files.find(f => f.id === selectedFileId);
    if (file && file.status === 'done') {
        previewVersion = 'optimized';
        updatePreviewButtons();
        loadModel(`/api/preview/${selectedFileId}?version=optimized&t=${Date.now()}`, file.output_size);
    }
});

// LOD toggle buttons
lodToggle.querySelectorAll('button').forEach(btn => {
    btn.addEventListener('click', () => {
        const version = btn.dataset.lod;
        const file = files.find(f => f.id === selectedFileId);
        if (!file) return;

        previewVersion = version;
        updatePreviewButtons();

        let fileSize = 0;
        if (version === 'billboard') {
            fileSize = 50000; // estimate
        } else {
            const idx = parseInt(version[3]);
            fileSize = file.lods?.[idx]?.size || 0;
        }
        loadModel(`/api/preview/${selectedFileId}?version=${version}&t=${Date.now()}`, fileSize);
    });
});

wireframeBtn.addEventListener('click', () => {
    wireframeEnabled = !wireframeEnabled;
    wireframeBtn.classList.toggle('active', wireframeEnabled);
    setWireframe(wireframeEnabled);
});

// Generate LODs & Billboard
generateLodsBtn.addEventListener('click', () => {
    if (selectedFileId) generateLODs(selectedFileId);
});

generateBlenderLodsBtn.addEventListener('click', () => {
    if (selectedFileId) generateBlenderLODs(selectedFileId);
});

generateBillboardBtn.addEventListener('click', () => {
    if (selectedFileId) generateBillboard(selectedFileId);
});

// Settings
simplificationSlider.addEventListener('input', () => {
    simplificationValue.textContent = parseFloat(simplificationSlider.value).toFixed(2);
});

textureQualitySlider.addEventListener('input', () => {
    texQualityValue.textContent = textureQualitySlider.value;
});

document.querySelectorAll('input[name="texCompression"]').forEach(radio => {
    radio.addEventListener('change', () => { texQualityRow.style.display = radio.value ? 'block' : 'none'; });
});

document.querySelectorAll('.preset-btn').forEach(btn => {
    btn.addEventListener('click', () => applyPreset(btn.dataset.preset));
});

// Stress test
const stressSlider = document.getElementById('stressCount');
const stressValueEl = document.getElementById('stressCountValue');
const stressBtn = document.getElementById('stressBtn');
const stressUseLods = document.getElementById('stressUseLods');
const lodQualitySlider = document.getElementById('lodQuality');
const lodQualityValue = document.getElementById('lodQualityValue');
const lodQualityLabel = document.getElementById('lodQualityLabel');

stressSlider.addEventListener('input', () => { stressValueEl.textContent = stressSlider.value + 'x'; });
lodQualitySlider.addEventListener('input', () => { lodQualityValue.textContent = lodQualitySlider.value + '%'; });

// Show/hide quality slider when LOD checkbox changes
stressUseLods.addEventListener('change', () => {
    const show = stressUseLods.checked;
    lodQualitySlider.style.display = show ? '' : 'none';
    lodQualityValue.style.display = show ? '' : 'none';
    lodQualityLabel.style.display = show ? '' : 'none';
});

stressBtn.addEventListener('click', () => {
    const count = parseInt(stressSlider.value);
    if (count <= 1) {
        clearStressInstances();
        if (currentModel) { currentModel.position.set(0, 0, 0); frameCamera(currentModel); }
    } else {
        const quality = parseInt(lodQualitySlider.value) / 100;
        runStressTest(count, stressUseLods.checked, quality);
    }
});

// ── Init ──
console.log('GLB Optimizer frontend loaded');
initThreeJS();
refreshFiles();

// Check server capabilities (Blender availability)
fetch('/api/status').then(r => r.json()).then(status => {
    if (status.blender && status.blender.available) {
        blenderAvailable = true;
        generateBlenderLodsBtn.style.display = '';
        console.log(`Blender ${status.blender.version} available for remesh LODs`);
    }
}).catch(() => {});
