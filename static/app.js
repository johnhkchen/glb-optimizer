import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { KTX2Loader } from 'three/addons/loaders/KTX2Loader.js';
import { MeshoptDecoder } from 'three/addons/libs/meshopt_decoder.module.js';
import { GLTFExporter } from 'three/addons/exporters/GLTFExporter.js';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';
import {
    LIGHTING_PRESETS,
    getLightingPreset,
    listLightingPresets,
    PRESET_FIELD_MAP,
} from './presets/lighting.js';
import { HELP_TEXT } from './help_text.js';

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
let pmremGenerator = null;
let defaultEnvironment = null;
let referenceEnvironment = null; // PMREM texture from user-provided reference image
let referencePalette = null; // { bright, mid, dark } colors used to tint bake lights
let bakeStale = false; // T-007-02: set true when preset changes after last regenerate
let currentSettings = null; // per-asset bake/tuning settings, populated by selectFile()
let _saveSettingsTimer = null; // debounce handle for saveSettings()
let groundPlane = null; // T-006-02: optional brown ground plane mesh
// T-008-03: sticky session flag — flips to true on the first
// non-empty file list and never flips back, so the first-run hint
// stays hidden after the user uploads (even if they delete every
// file later in the same session).
let firstRunHintDismissed = false;

// ── DOM refs ──
const dropZone = document.getElementById('dropZone');
const browseBtn = document.getElementById('browseBtn');
const fileInput = document.getElementById('fileInput');
const fileList = document.getElementById('fileList');
// T-008-02: processAllBtn removed from the UI; per-asset prep now
// flows through prepareForScene. The /api/process-all route stays
// reachable from devtools/curl for ad-hoc batch use.
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
const generateVolumetricBtn = document.getElementById('generateVolumetricBtn');
const generateVolumetricLodsBtn = document.getElementById('generateVolumetricLodsBtn');
const generateProductionBtn = document.getElementById('generateProductionBtn');
const buildPackBtn = document.getElementById('buildPackBtn');
// T-008-02: the toolbar #uploadReferenceBtn was removed. The hidden
// #referenceFileInput stays — it is still triggered by the
// in-tuning-panel "Upload reference image" button (T-005-03/T-007-03).
const referenceFileInput = document.getElementById('referenceFileInput');
const testLightingBtn = document.getElementById('testLightingBtn');
const generateBlenderLodsBtn = document.getElementById('generateBlenderLodsBtn');
// T-008-01: Prepare-for-scene primary action + progress block.
const prepareForSceneBtn = document.getElementById('prepareForSceneBtn');
const prepareProgress    = document.getElementById('prepareProgress');
const prepareStages      = document.getElementById('prepareStages');
const prepareError       = document.getElementById('prepareError');
const viewInSceneBtn     = document.getElementById('viewInSceneBtn');
// T-006-02: scene preview picker, instance count input, ground toggle.
const sceneTemplateSelect = document.getElementById('sceneTemplateSelect');
const sceneInstanceCount = document.getElementById('sceneInstanceCount');
const sceneGroundToggle = document.getElementById('sceneGroundToggle');

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

// ── Asset Settings ──
// Per-asset bake/tuning settings. Loaded from /api/settings/:id when a
// file is selected. Bake/preview functions read from currentSettings
// directly (see e.g. setupBakeLights). T-002-03 will wire UI sliders into
// saveSettings(). Until then, saveSettings is reachable only from the
// console for manual round-trip checks.

async function loadSettings(id) {
    try {
        const res = await fetch(`/api/settings/${id}`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        currentSettings = await res.json();
        normalizeTiltedFadeFields(currentSettings);
    } catch (err) {
        console.warn(`loadSettings(${id}) failed, using defaults:`, err);
        applyDefaults();
    }
    return currentSettings;
}

// T-009-03: Legacy on-disk settings written before T-009-03 don't have
// the three tilted-fade fields; the Go side marshals them as zero with
// `omitempty`. Backfill any missing/zero value with the canonical
// makeDefaults so the live preview math doesn't degenerate to a hard
// cut at lookDown=0.
function normalizeTiltedFadeFields(s) {
    if (!s) return;
    if (!s.tilted_fade_low_start)  s.tilted_fade_low_start  = 0.30;
    if (!s.tilted_fade_low_end)    s.tilted_fade_low_end    = 0.55;
    if (!s.tilted_fade_high_start) s.tilted_fade_high_start = 0.75;
}

function saveSettings(id) {
    if (_saveSettingsTimer) clearTimeout(_saveSettingsTimer);
    _saveSettingsTimer = setTimeout(async () => {
        _saveSettingsTimer = null;
        if (!currentSettings) return;
        try {
            const res = await fetch(`/api/settings/${id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(currentSettings),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
        } catch (err) {
            console.warn(`saveSettings(${id}) failed:`, err);
        }
    }, 300);
}

// makeDefaults returns a fresh canonical defaults object. Mirror of
// DefaultSettings() in settings.go — keep in sync by hand.
function makeDefaults() {
    return {
        schema_version: 1,
        volumetric_layers: 4,
        volumetric_resolution: 512,
        dome_height_factor: 0.5,
        bake_exposure: 1.0,
        ambient_intensity: 0.5,
        hemisphere_intensity: 1.0,
        key_light_intensity: 1.4,
        bottom_fill_intensity: 0.4,
        env_map_intensity: 1.2,
        alpha_test: 0.10,
        // T-009-03: tunable thresholds for the three-state production
        // crossfade. Mirror DefaultSettings() in settings.go.
        tilted_fade_low_start: 0.30,
        tilted_fade_low_end: 0.55,
        tilted_fade_high_start: 0.75,
        lighting_preset: 'default',
        slice_distribution_mode: 'visual-density',
        slice_axis: 'y',
        ground_align: true,
        reference_image_path: '',
        scene_template_id: 'grid',
        scene_instance_count: 100,
        scene_ground_plane: false,
    };
}

function applyDefaults() {
    currentSettings = makeDefaults();
    populateScenePreviewUI();
    return currentSettings;
}

// ── Lighting presets (T-007-01) ──
// Populate the lighting preset dropdown from the registry. Idempotent;
// safe to call more than once. The select is intentionally empty in
// index.html so this function is the single source of truth.
// T-006-02: populate the scene preview <select> from SCENE_TEMPLATES.
// Adding a template to the registry doesn't require HTML changes.
// Idempotent; safe to call more than once.
function populateScenePreviewSelect() {
    if (!sceneTemplateSelect) return;
    sceneTemplateSelect.innerHTML = '';
    for (const id of Object.keys(SCENE_TEMPLATES)) {
        const opt = document.createElement('option');
        opt.value = id;
        opt.textContent = SCENE_TEMPLATES[id].name;
        sceneTemplateSelect.appendChild(opt);
    }
}

// T-006-02: hydrate the picker / count input / ground toggle from
// currentSettings, and apply them to JS state (active template id,
// ground plane visibility). Called from selectFile (after
// loadSettings) and from applyDefaults so cold-start state is
// populated before any user interaction.
function populateScenePreviewUI() {
    if (!currentSettings) return;
    const tplId = currentSettings.scene_template_id || 'grid';
    const count = currentSettings.scene_instance_count || 100;
    const ground = !!currentSettings.scene_ground_plane;
    if (sceneTemplateSelect) sceneTemplateSelect.value = tplId;
    if (sceneInstanceCount) sceneInstanceCount.value = count;
    if (sceneGroundToggle) sceneGroundToggle.checked = ground;
    setSceneTemplate(tplId);
    if (groundPlane) groundPlane.visible = ground;
}

function populateLightingPresetSelect() {
    const sel = document.getElementById('tuneLightingPreset');
    if (!sel) return;
    sel.innerHTML = '';
    for (const p of listLightingPresets()) {
        const opt = document.createElement('option');
        opt.value = p.id;
        opt.textContent = p.name;
        opt.title = p.description;
        sel.appendChild(opt);
    }
}

// Apply a lighting preset to currentSettings, refresh the UI, save,
// and emit a single preset_applied analytics event capturing the
// cascade. Falls back to the 'default' preset if the id is unknown
// (corrupt settings file, hand-edit). Picking a preset is treated as
// one user intent, NOT N rapid setting_changed events — see
// design.md decision D4.
function applyLightingPreset(id) {
    const preset = getLightingPreset(id) || getLightingPreset('default');
    if (!preset || !currentSettings) return;
    const before = { lighting_preset: currentSettings.lighting_preset };
    const after = { lighting_preset: preset.id };
    const changed = {};
    for (const [presetKey, settingsKey] of Object.entries(PRESET_FIELD_MAP)) {
        const newVal = preset.bake_config[presetKey];
        const oldVal = currentSettings[settingsKey];
        if (newVal !== oldVal) {
            changed[settingsKey] = { from: oldVal, to: newVal };
            currentSettings[settingsKey] = newVal;
        }
    }
    currentSettings.lighting_preset = preset.id;
    populateTuningUI();
    // T-007-02: refresh the live three.js scene lights and mark the
    // on-disk bake as stale until the user regenerates. The follow-up
    // applyTuningToLiveScene call applies per-slider intensity values
    // on top of the preset's color tinting so the live preview matches
    // the slider state immediately.
    applyPresetToLiveScene();
    applyTuningToLiveScene();
    setBakeStale(true);
    // T-007-03: the `from-reference-image` preset id is the new
    // discriminator for reference-image calibration. Picking it (or
    // switching away from it) drives the same loadReferenceEnvironment
    // / tear-down branch that the legacy color_calibration_mode toggle
    // used to call. applyColorCalibration is idempotent and safe on
    // assets without an uploaded image — it just falls through to the
    // tear-down branch in that case.
    if (selectedFileId) applyColorCalibration(selectedFileId);
    if (selectedFileId) saveSettings(selectedFileId);
    logEvent('preset_applied', {
        from: before.lighting_preset,
        to: after.lighting_preset,
        changed,
    }, selectedFileId);
}

// ── Analytics (T-003-01 / T-003-02) ──
// Per-session event capture. T-003-01 shipped the envelope, the JSONL
// writer, and these helpers. T-003-02 wires them into the actual
// selection / tuning / regenerate flows.
//
// Sessions wrap a user's interaction with one asset. selectFile() ends
// the previous session (outcome=switched) and asks the backend to
// resume-or-start a session for the new asset; beforeunload ends the
// current session via sendBeacon (outcome=closed). Setting changes and
// regenerate clicks fire setting_changed / regenerate events
// automatically — no per-control instrumentation.
let analyticsSessionId = null;
let analyticsAssetId = null;        // asset the current session belongs to
let lastSettingChangeTs = null;     // performance.now() of last setting_changed

async function startAnalyticsSession(assetId) {
    if (!assetId) return null;
    try {
        const res = await fetch('/api/analytics/start-session', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ asset_id: assetId }),
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const body = await res.json();
        analyticsSessionId = body.session_id;
        analyticsAssetId = assetId;
        lastSettingChangeTs = null;
        return analyticsSessionId;
    } catch (err) {
        // Fall back to client-mint UUID + best-effort session_start so the
        // UI is never blocked by analytics. This mirrors the v1 (T-003-01)
        // behavior and is safe because the on-disk format is identical.
        console.warn('start-session failed, falling back to client mint:', err);
        analyticsSessionId = (typeof crypto !== 'undefined' && crypto.randomUUID)
            ? crypto.randomUUID()
            : fallbackUUID();
        analyticsAssetId = assetId;
        lastSettingChangeTs = null;
        logEvent('session_start', { trigger: 'open_asset' }, assetId);
        return analyticsSessionId;
    }
}

function endAnalyticsSession(outcome) {
    if (!analyticsSessionId) return;
    const assetId = analyticsAssetId;
    logEvent('session_end', {
        outcome: outcome || 'leave',
        final_settings: currentSettings || null,
    }, assetId);
    analyticsSessionId = null;
    analyticsAssetId = null;
    lastSettingChangeTs = null;
}

// endAnalyticsSessionBeacon ships a session_end via navigator.sendBeacon.
// Used only by the beforeunload listener — fetch() is cancelled by the
// browser during unload, sendBeacon is queued and survives.
function endAnalyticsSessionBeacon(outcome) {
    if (!analyticsSessionId) return;
    const envelope = {
        schema_version: 1,
        event_type: 'session_end',
        timestamp: new Date().toISOString(),
        session_id: analyticsSessionId,
        asset_id: analyticsAssetId || '',
        payload: {
            outcome: outcome || 'closed',
            final_settings: currentSettings || null,
        },
    };
    try {
        const blob = new Blob([JSON.stringify(envelope)], { type: 'application/json' });
        navigator.sendBeacon('/api/analytics/event', blob);
    } catch (err) {
        // Nothing useful to do here — page is unloading.
    }
    analyticsSessionId = null;
    analyticsAssetId = null;
    lastSettingChangeTs = null;
}

async function logEvent(type, payload, assetId) {
    if (!analyticsSessionId) return;
    const envelope = {
        schema_version: 1,
        event_type: type,
        timestamp: new Date().toISOString(),
        session_id: analyticsSessionId,
        asset_id: assetId || analyticsAssetId || '',
        payload: payload || {},
    };
    try {
        const res = await fetch('/api/analytics/event', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(envelope),
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
        console.warn('logEvent failed:', err);
    }
}

// fallbackUUID returns an RFC 4122 v4 UUID for environments where
// crypto.randomUUID is unavailable (e.g. older Safari, non-secure contexts).
function fallbackUUID() {
    const b = new Uint8Array(16);
    crypto.getRandomValues(b);
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    const h = [...b].map(x => x.toString(16).padStart(2, '0')).join('');
    return `${h.slice(0,8)}-${h.slice(8,12)}-${h.slice(12,16)}-${h.slice(16,20)}-${h.slice(20,32)}`;
}

window.startAnalyticsSession = startAnalyticsSession;
window.endAnalyticsSession = endAnalyticsSession;
window.endAnalyticsSessionBeacon = endAnalyticsSessionBeacon;
window.logEvent = logEvent;

// ── T-004-04: Multi-strategy comparison UI ──
//
// When the classifier (T-004-02) returns confidence < 0.7, or when the
// user clicks "Reclassify…" in the tuning panel, we open a modal that
// renders the asset under 2-3 candidate strategies side-by-side. The
// user's pick is the most valuable training signal in S-004 — every
// resolved comparison becomes a labeled training example linking
// (asset features → human-preferred strategy).
//
// The candidate ranking comes from the classifier's
// `features.candidates` field (Python side) and round-trips through
// the /api/classify response. Picks are POSTed back as
// /api/classify/:id?override=<category>; the Go side stamps the
// strategy and emits a `classification_override` analytics event.
//
// The auto-confidence threshold and the human-confirmed sentinel
// (1.0) are documented in docs/knowledge/settings-schema.md under
// shape_confidence.

// JS-side mirror of strategy.go's shapeStrategyTable. Kept in sync by
// hand — same staleness risk as the existing Python ↔ Go duplication
// of validShapeCategories. Used by renderCandidateThumbnail to
// temporarily mutate currentSettings before each candidate bake.
const STRATEGY_TABLE = {
    'round-bush':   { slice_axis: 'y',               slice_distribution_mode: 'visual-density', volumetric_layers: 4, instance_orientation_rule: 'random-y' },
    'directional':  { slice_axis: 'auto-horizontal', slice_distribution_mode: 'equal-height',   volumetric_layers: 4, instance_orientation_rule: 'fixed' },
    'tall-narrow':  { slice_axis: 'y',               slice_distribution_mode: 'equal-height',   volumetric_layers: 6, instance_orientation_rule: 'random-y' },
    'planar':       { slice_axis: 'auto-thin',       slice_distribution_mode: 'equal-height',   volumetric_layers: 3, instance_orientation_rule: 'aligned-to-row' },
    'hard-surface': { slice_axis: 'n/a',             slice_distribution_mode: 'n/a',             volumetric_layers: 0, instance_orientation_rule: 'fixed' },
    'unknown':      { slice_axis: 'y',               slice_distribution_mode: 'visual-density', volumetric_layers: 4, instance_orientation_rule: 'random-y' },
};

// T-004-05: derive whether stress-test instances should get a random
// per-instance Y rotation, from the strategy router's
// instance_orientation_rule. `fixed` (directional, hard-surface) and
// `aligned-to-row` (planar, eventually controlled per-row by S-006)
// both want every instance pointing the same way. Anything else
// (including unknown / missing) falls back to the historical
// random-y behavior so the rose / round-bush case is unchanged.
function shouldRandomRotateInstances() {
    const cat = currentSettings && currentSettings.shape_category;
    const rule = (STRATEGY_TABLE[cat] && STRATEGY_TABLE[cat].instance_orientation_rule) || 'random-y';
    return rule !== 'fixed' && rule !== 'aligned-to-row';
}

const COMPARISON_THUMB_SIZE = 256;
const COMPARISON_AUTO_THRESHOLD = 0.7;

// fetchClassification calls POST /api/classify/:id with an optional
// override category. Returns {settings, candidates}; throws on HTTP
// failure. Single source of truth for the endpoint shape.
async function fetchClassification(id, overrideCategory) {
    const url = overrideCategory
        ? `/api/classify/${id}?override=${encodeURIComponent(overrideCategory)}`
        : `/api/classify/${id}`;
    const res = await fetch(url, { method: 'POST' });
    if (!res.ok) {
        const text = await res.text().catch(() => '');
        throw new Error(`POST ${url} → ${res.status}: ${text}`);
    }
    return await res.json();
}

// openComparisonModal builds the slot DOM (one per candidate, capped
// at 3) and sequentially renders each thumbnail. Closing the modal
// without a pick is allowed — the asset's classifier-derived state is
// preserved and the auto-open will fire again next time.
async function openComparisonModal(id, candidates, originalCategory, originalConfidence) {
    if (!Array.isArray(candidates) || candidates.length === 0) return;
    if (!currentModel) {
        // The model hasn't loaded yet — no point rendering thumbnails.
        // Caller is responsible for sequencing this after loadModel.
        console.warn('openComparisonModal: no currentModel, skipping');
        return;
    }
    const modal = document.getElementById('comparisonModal');
    const slotsEl = document.getElementById('comparisonSlots');
    const subtitle = document.getElementById('comparisonSubtitle');
    const errEl = document.getElementById('comparisonError');
    if (!modal || !slotsEl) return;

    modal.dataset.assetId = id;
    modal.dataset.originalCategory = originalCategory || '';
    modal.dataset.originalConfidence = String(originalConfidence ?? 0);
    if (errEl) errEl.textContent = '';
    if (subtitle) {
        const confTxt = (originalConfidence != null && originalConfidence > 0)
            ? ` (classifier picked ${originalCategory} @ ${originalConfidence.toFixed(2)})`
            : '';
        subtitle.textContent = `Pick how this asset should be processed${confTxt}.`;
    }

    // Build slots up front so the user sees the layout immediately.
    slotsEl.innerHTML = '';
    const top = candidates.slice(0, 3);
    const slotEls = top.map((cand) => {
        const slot = document.createElement('div');
        slot.className = 'comparison-slot';
        const thumb = document.createElement('div');
        thumb.className = 'slot-thumb';
        thumb.textContent = 'Rendering…';
        const label = document.createElement('div');
        label.className = 'slot-label';
        label.textContent = cand.category;
        const score = document.createElement('div');
        score.className = 'slot-score';
        score.textContent = `score ${cand.score.toFixed(2)}`;
        const btn = document.createElement('button');
        btn.textContent = `Pick ${cand.category}`;
        btn.disabled = true; // enabled once the thumbnail finishes
        btn.addEventListener('click', () => pickCandidate(id, cand.category));
        slot.appendChild(thumb);
        slot.appendChild(label);
        slot.appendChild(score);
        slot.appendChild(btn);
        slotsEl.appendChild(slot);
        return { slot, thumb, btn, candidate: cand };
    });

    modal.style.display = 'flex';

    // Render thumbnails sequentially. Each slot mutates currentSettings
    // around its bake call (see renderCandidateThumbnail) so concurrent
    // bakes would race on the shared object.
    for (const { thumb, btn, candidate } of slotEls) {
        try {
            await renderCandidateThumbnail(thumb, candidate);
        } catch (err) {
            console.error(`renderCandidateThumbnail(${candidate.category}) failed:`, err);
            thumb.textContent = 'Render failed';
        }
        btn.disabled = false;
    }
}

// renderCandidateThumbnail bakes the asset under one candidate
// strategy and stuffs the result into the slot's thumbnail container.
// The hard-surface special case skips the bake entirely (slice_axis is
// the `n/a` sentinel — feeding it to the slice resolver would crash).
async function renderCandidateThumbnail(thumbEl, candidate) {
    if (!currentModel) throw new Error('no currentModel');
    const strategy = STRATEGY_TABLE[candidate.category];
    if (!strategy) throw new Error(`unknown strategy ${candidate.category}`);

    if (candidate.category === 'hard-surface') {
        // Hard-surface routes to the parametric pipeline (S-001), not
        // the slice bake. Show the raw model with a caption explaining
        // why there is no slice preview.
        const canvas = renderModelToCanvas(currentModel, COMPARISON_THUMB_SIZE);
        _replaceThumbWithCanvas(thumbEl, canvas, '(parametric — no slicing)');
        return;
    }

    // Snapshot, mutate, restore. Sequential by construction; no race.
    const saved = {
        slice_axis: currentSettings.slice_axis,
        slice_distribution_mode: currentSettings.slice_distribution_mode,
        volumetric_layers: currentSettings.volumetric_layers,
    };
    currentSettings.slice_axis = strategy.slice_axis;
    currentSettings.slice_distribution_mode = strategy.slice_distribution_mode;
    currentSettings.volumetric_layers = strategy.volumetric_layers;

    let glbBuf;
    try {
        glbBuf = await renderHorizontalLayerGLB(currentModel, strategy.volumetric_layers, COMPARISON_THUMB_SIZE);
    } finally {
        currentSettings.slice_axis = saved.slice_axis;
        currentSettings.slice_distribution_mode = saved.slice_distribution_mode;
        currentSettings.volumetric_layers = saved.volumetric_layers;
    }

    const blob = new Blob([glbBuf], { type: 'model/gltf-binary' });
    const url = URL.createObjectURL(blob);
    let loaded;
    try {
        const reloadLoader = new GLTFLoader();
        loaded = await new Promise((resolve, reject) => {
            reloadLoader.load(url, resolve, undefined, reject);
        });
    } finally {
        URL.revokeObjectURL(url);
    }

    const canvas = renderModelToCanvas(loaded.scene, COMPARISON_THUMB_SIZE);
    _replaceThumbWithCanvas(thumbEl, canvas, '');
}

// renderModelToCanvas frames a model in a fresh offscreen renderer
// and returns a 2D canvas snapshot at the requested size. Used for
// both the bake-roundtrip path and the hard-surface "raw model"
// fallback in the comparison modal.
function renderModelToCanvas(model, resolution) {
    const offRenderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;
    offRenderer.toneMappingExposure = 1.0;

    const offScene = new THREE.Scene();
    offScene.add(new THREE.AmbientLight(0xffffff, 0.7));
    const dl = new THREE.DirectionalLight(0xffffff, 1.2);
    dl.position.set(2, 4, 2);
    offScene.add(dl);

    // We attach the live model to a temporary parent for framing math
    // and then detach so the live preview isn't disturbed. matrixWorld
    // updates are forced so the bbox math is accurate.
    const wrapper = new THREE.Group();
    const originalParent = model.parent;
    if (originalParent) originalParent.remove(model);
    wrapper.add(model);
    offScene.add(wrapper);
    wrapper.updateMatrixWorld(true);

    const box = new THREE.Box3().setFromObject(wrapper);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const radius = Math.max(size.x, size.y, size.z) * 0.85 + 0.001;

    const offCamera = new THREE.PerspectiveCamera(45, 1, 0.01, radius * 20);
    const dist = radius * 2.4;
    offCamera.position.set(center.x + dist, center.y + dist * 0.5, center.z + dist);
    offCamera.lookAt(center);

    offRenderer.render(offScene, offCamera);

    const src = offRenderer.domElement;
    const out = document.createElement('canvas');
    out.width = resolution;
    out.height = resolution;
    out.getContext('2d').drawImage(src, 0, 0);

    // Restore the live preview parenting.
    wrapper.remove(model);
    if (originalParent) originalParent.add(model);

    offRenderer.dispose();
    return out;
}

function _replaceThumbWithCanvas(thumbEl, canvas, caption) {
    thumbEl.innerHTML = '';
    thumbEl.style.background = 'none';
    const img = document.createElement('img');
    img.src = canvas.toDataURL('image/png');
    img.alt = caption || '';
    img.width = COMPARISON_THUMB_SIZE;
    img.height = COMPARISON_THUMB_SIZE;
    thumbEl.appendChild(img);
    if (caption) {
        const cap = document.createElement('div');
        cap.style.fontSize = '10px';
        cap.style.color = '#888';
        cap.style.marginTop = '4px';
        cap.textContent = caption;
        thumbEl.appendChild(cap);
    }
}

// pickCandidate POSTs the override and, on success, swaps the new
// settings into currentSettings, refreshes the tuning panel, and
// closes the modal. The classification_override analytics event is
// emitted server-side; JS does not double-fire.
async function pickCandidate(id, category) {
    const errEl = document.getElementById('comparisonError');
    if (errEl) errEl.textContent = '';
    try {
        const { settings } = await fetchClassification(id, category);
        currentSettings = settings;
        populateTuningUI();
        // Refresh the file list so the dirty indicator updates.
        if (typeof refreshFiles === 'function') {
            refreshFiles().catch(() => {});
        }
        closeComparisonModal();
    } catch (err) {
        console.error('pickCandidate failed:', err);
        if (errEl) errEl.textContent = `Override failed: ${err.message || err}`;
    }
}

function closeComparisonModal() {
    const modal = document.getElementById('comparisonModal');
    if (!modal) return;
    modal.style.display = 'none';
    const slotsEl = document.getElementById('comparisonSlots');
    if (slotsEl) slotsEl.innerHTML = '';
    delete modal.dataset.assetId;
    delete modal.dataset.originalCategory;
    delete modal.dataset.originalConfidence;
}

window.openComparisonModal = openComparisonModal;
window.closeComparisonModal = closeComparisonModal;

// ── Tuning UI (T-002-03) ──
// One control per AssetSettings field. populateTuningUI runs after every
// loadSettings; wireTuningUI runs once at module init. The dirty dot
// highlights when currentSettings diverges from makeDefaults().
const TUNING_SPEC = [
    { field: 'volumetric_layers',     id: 'tuneVolumetricLayers',     parse: v => parseInt(v, 10), fmt: v => String(v) },
    { field: 'volumetric_resolution', id: 'tuneVolumetricResolution', parse: v => parseInt(v, 10), fmt: v => String(v) },
    { field: 'dome_height_factor',    id: 'tuneDomeHeightFactor',     parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'bake_exposure',         id: 'tuneBakeExposure',         parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'ambient_intensity',     id: 'tuneAmbientIntensity',     parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'hemisphere_intensity',  id: 'tuneHemisphereIntensity',  parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'key_light_intensity',   id: 'tuneKeyLightIntensity',    parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'bottom_fill_intensity', id: 'tuneBottomFillIntensity',  parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'env_map_intensity',     id: 'tuneEnvMapIntensity',      parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'alpha_test',            id: 'tuneAlphaTest',            parse: parseFloat,           fmt: v => v.toFixed(3) },
    // T-009-03: tunable thresholds for the three-state production
    // crossfade (horizontal → tilted → dome). Auto-instrumented for
    // setting_changed analytics by wireTuningUI.
    { field: 'tilted_fade_low_start',  id: 'tuneTiltedFadeLowStart',  parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'tilted_fade_low_end',    id: 'tuneTiltedFadeLowEnd',    parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'tilted_fade_high_start', id: 'tuneTiltedFadeHighStart', parse: parseFloat,           fmt: v => v.toFixed(2) },
    { field: 'lighting_preset',       id: 'tuneLightingPreset',       parse: v => v,               fmt: v => v },
    // T-005-01: enrolled for setting_changed analytics. DOM ids are
    // reserved here so the auto-instrumentation in wireTuningUI picks
    // them up the moment T-005-02 lands the controls; populate/wire
    // both short-circuit harmlessly when the elements are absent.
    { field: 'slice_distribution_mode', id: 'tuneSliceDistributionMode', parse: v => v,             fmt: v => v },
    // T-004-03: slice_axis is populated by the strategy router on
    // classification. The DOM id is reserved here so a future tuning
    // control gets auto-instrumented; populate/wire short-circuit
    // harmlessly when the element is absent.
    { field: 'slice_axis',              id: 'tuneSliceAxis',             parse: v => v,             fmt: v => v },
    { field: 'ground_align',            id: 'tuneGroundAlign',           parse: v => v === true || v === 'true',
                                                                          fmt: v => String(v) },
];

function populateTuningUI() {
    if (!currentSettings) return;
    for (const spec of TUNING_SPEC) {
        const el = document.getElementById(spec.id);
        if (!el) continue;
        const v = currentSettings[spec.field];
        // T-005-02: checkboxes carry state on .checked, not .value.
        if (el.type === 'checkbox') {
            el.checked = !!v;
        } else {
            el.value = v;
        }
        const valEl = document.getElementById(spec.id + 'Value');
        if (valEl) valEl.textContent = spec.fmt(v);
    }
    updateTuningDirty();
    syncReferenceImageRow();
    // T-004-04: refresh the shape-category hint next to the
    // Reclassify… button so the user knows what they'd be overriding.
    const hint = document.getElementById('tuneShapeCategoryHint');
    if (hint) {
        const cat = currentSettings.shape_category || 'unknown';
        const conf = currentSettings.shape_confidence;
        hint.textContent = (conf > 0)
            ? `${cat} @ ${conf.toFixed(2)}`
            : cat;
    }
}

function wireTuningUI() {
    for (const spec of TUNING_SPEC) {
        const el = document.getElementById(spec.id);
        if (!el) continue;
        el.addEventListener('input', () => {
            if (!currentSettings || !selectedFileId) return;
            const oldValue = currentSettings[spec.field];
            // T-005-02: checkboxes carry state on .checked, not .value.
            const raw = el.type === 'checkbox' ? el.checked : el.value;
            const v = spec.parse(raw);
            if (v === oldValue) return;
            // T-007-01: lighting_preset is a cascade trigger, not a
            // plain field write. applyLightingPreset rewrites the
            // dependent intensity sliders, calls populateTuningUI()
            // to refresh, saves, and emits a single preset_applied
            // analytics event in lieu of N setting_changed events.
            if (spec.field === 'lighting_preset') {
                applyLightingPreset(v);
                return;
            }
            currentSettings[spec.field] = v;
            const valEl = document.getElementById(spec.id + 'Value');
            if (valEl) valEl.textContent = spec.fmt(v);
            updateTuningDirty();
            saveSettings(selectedFileId);
            // Real-time live-preview feedback. Schedule via rAF so we
            // batch into the next animation frame instead of running
            // synchronously inside the input handler — this avoids
            // any chance of the lights/materials being mutated mid-
            // frame during a wheel/orbit interaction.
            scheduleLiveTuningRefresh();
            // Auto-instrumentation: fire setting_changed alongside the
            // debounced PUT. ms_since_prev is the gap from the previous
            // setting_changed in this session (null on the first one),
            // which T-003-04 will use to detect "fast revert" patterns.
            const now = (typeof performance !== 'undefined' && performance.now)
                ? performance.now()
                : Date.now();
            const msSincePrev = lastSettingChangeTs == null
                ? null
                : Math.round(now - lastSettingChangeTs);
            lastSettingChangeTs = now;
            logEvent('setting_changed', {
                key: spec.field,
                old_value: oldValue,
                new_value: v,
                ms_since_prev: msSincePrev,
            }, selectedFileId);
        });
    }
    const refBtn = document.getElementById('tuneReferenceImageBtn');
    if (refBtn) {
        refBtn.addEventListener('click', () => {
            if (selectedFileId) referenceFileInput.click();
        });
    }
    const resetBtn = document.getElementById('tuneResetBtn');
    if (resetBtn) {
        resetBtn.addEventListener('click', () => {
            if (!selectedFileId) return;
            // T-007-03: route reset through applyLightingPreset so it
            // tears down any active reference-image calibration AND
            // refreshes the live scene + bake-stale hint via the same
            // cascade a manual preset pick uses. applyDefaults() seeds
            // currentSettings; applyLightingPreset('default') then
            // rewrites sliders, saves, and emits preset_applied.
            applyDefaults();
            applyLightingPreset('default');
        });
    }
    // T-004-04: manual Reclassify… opens the comparison modal
    // regardless of confidence. Always re-runs the classifier.
    const reclassBtn = document.getElementById('tuneReclassifyBtn');
    if (reclassBtn) {
        reclassBtn.addEventListener('click', async () => {
            if (!selectedFileId || !currentModel) return;
            reclassBtn.disabled = true;
            try {
                const { settings, candidates } = await fetchClassification(selectedFileId);
                currentSettings = settings;
                populateTuningUI();
                if (candidates && candidates.length > 0) {
                    await openComparisonModal(
                        selectedFileId,
                        candidates,
                        settings.shape_category,
                        settings.shape_confidence,
                    );
                }
            } catch (err) {
                console.warn('manual reclassify failed:', err);
            } finally {
                reclassBtn.disabled = false;
            }
        });
    }
    // T-004-04: cancel + backdrop click both close the modal without
    // recording an analytics event. The asset's classifier-derived
    // state is preserved.
    const cancelBtn = document.getElementById('comparisonCancelBtn');
    if (cancelBtn) cancelBtn.addEventListener('click', closeComparisonModal);
    const backdrop = document.getElementById('comparisonBackdrop');
    if (backdrop) backdrop.addEventListener('click', closeComparisonModal);
}

function updateTuningDirty() {
    const dot = document.getElementById('tuningDirtyDot');
    if (!dot || !currentSettings) return;
    const defs = makeDefaults();
    let dirty = false;
    for (const spec of TUNING_SPEC) {
        if (currentSettings[spec.field] !== defs[spec.field]) {
            dirty = true;
            break;
        }
    }
    dot.classList.toggle('dirty', dirty);
}

// ── Profiles (T-003-03) ──
// Named, commented snapshots of currentSettings the user can save and
// reuse across assets. Backed by /api/profiles + ~/.glb-optimizer/profiles/.
const PROFILE_NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
let profilesList = []; // ProfileMetadata[] from server

const profileSelect        = document.getElementById('profileSelect');
const profileApplyBtn      = document.getElementById('profileApplyBtn');
const profileDeleteBtn     = document.getElementById('profileDeleteBtn');
const profileSaveOpenBtn   = document.getElementById('profileSaveOpenBtn');
const profileSaveForm      = document.getElementById('profileSaveForm');
const profileNameInput     = document.getElementById('profileNameInput');
const profileNameError     = document.getElementById('profileNameError');
const profileCommentInput  = document.getElementById('profileCommentInput');
const profileSaveSubmitBtn = document.getElementById('profileSaveSubmitBtn');
const profileSaveCancelBtn = document.getElementById('profileSaveCancelBtn');

async function loadProfileList() {
    try {
        const res = await fetch('/api/profiles');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        profilesList = await res.json();
        if (!Array.isArray(profilesList)) profilesList = [];
    } catch (err) {
        console.warn('loadProfileList failed:', err);
        profilesList = [];
    }
    redrawProfileSelect();
}

function redrawProfileSelect() {
    const previous = profileSelect.value;
    // Clear all but the placeholder option.
    while (profileSelect.options.length > 1) profileSelect.remove(1);
    for (const m of profilesList) {
        const opt = document.createElement('option');
        opt.value = m.name;
        opt.textContent = m.comment ? `${m.name} — ${m.comment}` : m.name;
        opt.title = m.comment || m.name;
        profileSelect.appendChild(opt);
    }
    // Restore prior selection if it still exists.
    if (previous && profilesList.some(m => m.name === previous)) {
        profileSelect.value = previous;
    } else {
        profileSelect.value = '';
    }
    updateProfileButtons();
}

function updateProfileButtons() {
    const has = profileSelect.value !== '';
    profileApplyBtn.disabled = !has || !selectedFileId;
    profileDeleteBtn.disabled = !has;
}

async function applySelectedProfile() {
    const name = profileSelect.value;
    if (!name || !selectedFileId) return;
    try {
        const res = await fetch(`/api/profiles/${encodeURIComponent(name)}`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const profile = await res.json();
        if (!profile || !profile.settings) throw new Error('profile missing settings');
        // PUT the profile settings to the asset's settings endpoint and
        // then re-read so currentSettings reflects what's actually on disk.
        const putRes = await fetch(`/api/settings/${selectedFileId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(profile.settings),
        });
        if (!putRes.ok) throw new Error(`PUT settings HTTP ${putRes.status}`);
        await loadSettings(selectedFileId);
        populateTuningUI();
        logEvent('profile_applied', { profile_name: name }, selectedFileId);
    } catch (err) {
        console.error('applySelectedProfile failed:', err);
    }
}

async function deleteSelectedProfile() {
    const name = profileSelect.value;
    if (!name) return;
    if (!confirm(`Delete profile "${name}"?`)) return;
    try {
        const res = await fetch(`/api/profiles/${encodeURIComponent(name)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        await loadProfileList();
    } catch (err) {
        console.error('deleteSelectedProfile failed:', err);
    }
}

function openSaveProfileForm() {
    profileNameInput.value = '';
    profileCommentInput.value = '';
    profileNameError.textContent = '';
    profileNameInput.classList.remove('invalid');
    profileSaveForm.style.display = '';
    profileNameInput.focus();
}

function closeSaveProfileForm() {
    profileSaveForm.style.display = 'none';
    profileNameError.textContent = '';
    profileNameInput.classList.remove('invalid');
}

async function submitSaveProfile() {
    const name = profileNameInput.value.trim();
    const comment = profileCommentInput.value;
    profileNameError.textContent = '';
    profileNameInput.classList.remove('invalid');

    if (!PROFILE_NAME_RE.test(name) || name.length > 64) {
        profileNameError.textContent = 'Name must be kebab-case (a-z0-9 with single dashes), 1–64 chars.';
        profileNameInput.classList.add('invalid');
        return;
    }
    if (!currentSettings) {
        profileNameError.textContent = 'No settings to save — select an asset first.';
        return;
    }
    try {
        const res = await fetch('/api/profiles', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name,
                comment,
                settings: currentSettings,
                source_asset_id: selectedFileId || '',
            }),
        });
        if (!res.ok) {
            let msg = `HTTP ${res.status}`;
            try { const j = await res.json(); if (j && j.error) msg = j.error; } catch (_) {}
            profileNameError.textContent = msg;
            profileNameInput.classList.add('invalid');
            return;
        }
        closeSaveProfileForm();
        await loadProfileList();
        profileSelect.value = name;
        updateProfileButtons();
        logEvent('profile_saved', { profile_name: name }, selectedFileId);
    } catch (err) {
        console.error('submitSaveProfile failed:', err);
        profileNameError.textContent = String(err);
    }
}

// ── Accepted (T-003-04) ──
// "Mark as Accepted" snapshots the asset's current saved settings as the
// canonical training-grade configuration, captures a 256px JPEG of the
// preview canvas, and emits an `accept` analytics event. The server reads
// settings from disk, so the saveSettings() debounce flushing is not on
// the critical path here. See docs/active/work/T-003-04/design.md.
const acceptCommentInput = document.getElementById('acceptCommentInput');
const acceptBtn          = document.getElementById('acceptBtn');
const acceptStatus       = document.getElementById('acceptStatus');

function setAcceptStatus(text, kind) {
    acceptStatus.textContent = text;
    acceptStatus.classList.remove('ok', 'err');
    if (kind === 'ok') acceptStatus.classList.add('ok');
    if (kind === 'err') acceptStatus.classList.add('err');
}

// capturePreviewThumbnail returns a data URL string for a 256px JPEG of
// the current preview canvas, or '' if no model is loaded. The renderer
// is constructed without preserveDrawingBuffer; calling render() right
// before reading guarantees the GL backbuffer is valid for this tick.
function capturePreviewThumbnail() {
    if (!currentModel || !renderer) return '';
    try {
        renderer.render(scene, camera);
        const src = previewCanvas;
        const longest = Math.max(src.width, src.height) || 1;
        const scale = Math.min(1, 256 / longest);
        const w = Math.max(1, Math.round(src.width * scale));
        const h = Math.max(1, Math.round(src.height * scale));
        const off = document.createElement('canvas');
        off.width = w;
        off.height = h;
        off.getContext('2d').drawImage(src, 0, 0, w, h);
        return off.toDataURL('image/jpeg', 0.85);
    } catch (err) {
        console.warn('capturePreviewThumbnail failed:', err);
        return '';
    }
}

async function populateAcceptedUI(id) {
    if (!id) {
        acceptCommentInput.value = '';
        setAcceptStatus('', '');
        acceptBtn.disabled = true;
        return;
    }
    acceptBtn.disabled = false;
    try {
        const res = await fetch(`/api/accept/${id}`);
        if (res.status === 404) {
            acceptCommentInput.value = '';
            setAcceptStatus('Not accepted yet', '');
            return;
        }
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const a = await res.json();
        acceptCommentInput.value = a.comment || '';
        setAcceptStatus('Accepted ✓', 'ok');
    } catch (err) {
        console.warn('populateAcceptedUI failed:', err);
        setAcceptStatus('', '');
    }
}

async function markAccepted() {
    if (!selectedFileId) return;
    const id = selectedFileId;
    acceptBtn.disabled = true;
    setAcceptStatus('Saving…', '');
    const dataUrl = capturePreviewThumbnail();
    try {
        const res = await fetch(`/api/accept/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                comment: acceptCommentInput.value || '',
                thumbnail_b64: dataUrl,
            }),
        });
        if (!res.ok) {
            let msg = `HTTP ${res.status}`;
            try { const j = await res.json(); if (j && j.error) msg = j.error; } catch (_) {}
            throw new Error(msg);
        }
        const snap = await res.json();
        const f = files.find(x => x.id === id);
        if (f) {
            f.is_accepted = true;
            renderFileList();
        }
        setAcceptStatus('Accepted ✓', 'ok');
        logEvent('accept', {
            settings: snap.settings,
            thumbnail_path: snap.thumbnail_path || '',
        }, id);
    } catch (err) {
        console.error('markAccepted failed:', err);
        setAcceptStatus(String(err.message || err), 'err');
    } finally {
        acceptBtn.disabled = !selectedFileId;
    }
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

// T-008-02: processAll() removed along with the left-panel button.
// The /api/process-all route is still mounted server-side.

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
    generateLodsBtn.textContent = 'Building…';
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
    generateLodsBtn.textContent = 'Build LOD chain';
    generateLodsBtn.classList.remove('generating');
    generateLodsBtn.disabled = false;
}

async function generateBlenderLODs(id) {
    generateBlenderLodsBtn.textContent = 'Building…';
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
    generateBlenderLodsBtn.textContent = 'Build LOD chain (Blender remesh)';
    generateBlenderLodsBtn.classList.remove('generating');
    generateBlenderLodsBtn.disabled = false;
}

// ── Billboard Impostor Generation ──
const BILLBOARD_ANGLES = 6; // render from 6 evenly-spaced angles
// T-009-01: Tilted-camera billboard bake. Side variants only — uses an
// elevated viewing angle so the runtime (T-009-02) can crossfade
// between horizontal sides, top-down, and these tilted sides as the
// camera pitches up.
const TILTED_BILLBOARD_ANGLES        = 6;
const TILTED_BILLBOARD_ELEVATION_RAD = Math.PI / 6; // 30°
const TILTED_BILLBOARD_RESOLUTION    = 512;
let billboardVariants = []; // stored { geometry, material, quadWidth, quadHeight } per angle

async function generateBillboard(id) {
    if (!currentModel || !threeReady) return;

    generateBillboardBtn.textContent = 'Building…';
    generateBillboardBtn.classList.add('generating');
    generateBillboardBtn.disabled = true;

    let success = false;
    try {
        const glb = await renderMultiAngleBillboardGLB(currentModel, BILLBOARD_ANGLES);
        await fetch(`/api/upload-billboard/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: glb,
        });

        store_update(id, f => f.has_billboard = true);
        updatePreviewButtons();
        success = true;
        setBakeStale(false); // T-007-02
    } catch (err) { console.error('Billboard generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'billboard', success }, id);
    }

    generateBillboardBtn.textContent = 'Build camera-facing impostor';
    generateBillboardBtn.classList.remove('generating');
    generateBillboardBtn.disabled = false;
}

// T-009-01: Tilted-camera billboard bake. Devtools-only entry point;
// no toolbar button this ticket. Bakes N side variants from an
// elevated camera and uploads the resulting GLB to the new
// `/api/upload-billboard-tilted/:id` endpoint. Runtime loading +
// crossfade are T-009-02 / T-009-03.
async function generateTiltedBillboard(id) {
    if (!currentModel || !threeReady) return;

    let success = false;
    try {
        const glb = await renderTiltedBillboardGLB(
            currentModel,
            TILTED_BILLBOARD_ANGLES,
            TILTED_BILLBOARD_ELEVATION_RAD,
            TILTED_BILLBOARD_RESOLUTION,
        );
        await fetch(`/api/upload-billboard-tilted/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: glb,
        });

        store_update(id, f => f.has_billboard_tilted = true);
        updatePreviewButtons();
        success = true;
        setBakeStale(false);
    } catch (err) { console.error('Tilted billboard generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'billboard_tilted', success }, id);
    }
}

// T-009-01: `elevationRad` defaults to 0, in which case every
// expression below algebraically reduces to the legacy zero-elevation
// math (cos(0)=1, sin(0)=0 are exact in IEEE-754, so multiplying by
// them is a no-op). When elevationRad > 0, the camera is lifted by
// `dist*sin(elev)` and pulled in horizontally by `dist*cos(elev)`,
// keeping the radial distance constant so the model's apparent size
// matches the horizontal bake. The ortho frustum's vertical extent
// expands to `size.y*cos + maxHoriz*sin` so the rendered silhouette
// fits the captured quad without stretching at any elevation.
function renderBillboardAngle(model, angleRad, resolution, elevationRad = 0) {
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
    offRenderer.toneMappingExposure = currentSettings.bake_exposure;

    // Ortho camera sized to fit model
    const cosE = Math.cos(elevationRad);
    const sinE = Math.sin(elevationRad);
    const maxHoriz = Math.max(size.x, size.z);
    const halfH = (size.y * cosE + maxHoriz * sinE) * 0.55;
    const halfW = maxHoriz * 0.55;
    const offCamera = new THREE.OrthographicCamera(-halfW, halfW, halfH, -halfH, 0.01, maxDim * 10);

    // Position camera around the model at the given angle. When
    // elevationRad === 0 this collapses to the legacy horizontal
    // orbit; otherwise the camera lifts above the model's Y center.
    const dist = maxDim * 2;
    offCamera.position.set(
        center.x + Math.sin(angleRad) * dist * cosE,
        center.y + dist * sinE,
        center.z + Math.cos(angleRad) * dist * cosE
    );
    offCamera.lookAt(center);

    const offScene = new THREE.Scene();
    // Generate a PMREM env on THIS renderer so PBR materials get proper IBL.
    // Tinted by the reference palette when one is loaded.
    const bakeEnv = createBakeEnvironment(offRenderer);
    offScene.environment = bakeEnv;
    setupBakeLights(offScene);

    const clone = cloneModelForBake(model);
    offScene.add(clone);
    offRenderer.render(offScene, offCamera);

    const canvas = offRenderer.domElement;
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);

    bakeEnv.dispose();
    offRenderer.dispose();

    return { canvas: copyCanvas, quadWidth: halfW * 2, quadHeight: halfH * 2, center, boxMinY: box.min.y };
}

// T-007-02: Pure helper. Given a preset config object (bake_config or
// preview_config), return the {bright, mid, dark, key, fill} palette
// the light-setup helpers consume. Reused by getActiveBakePalette and
// getActivePreviewPalette so the tuple-to-object dance lives in one
// place.
function resolvePresetColors(cfg) {
    const tup = ([r, g, b]) => ({ r, g, b });
    return {
        bright: tup(cfg.hemisphere_sky),
        mid:    tup(cfg.key_color),
        dark:   tup(cfg.hemisphere_ground),
        key:    tup(cfg.key_color),
        fill:   tup(cfg.fill_color),
    };
}

// T-007-01: Resolve the active bake palette in priority order:
//   1. referencePalette — explicit user calibration always wins.
//   2. Active lighting preset's bake_config colors.
//   3. Neutral white/dark fallback (the legacy default).
// Returns a {bright, mid, dark} object using {r,g,b} 0..1 components,
// matching the shape setupBakeLights and renderLayerTopDown already use.
// (T-007-02: also returns key/fill for parity with getActivePreviewPalette;
// existing bake call sites simply ignore those fields.)
function getActiveBakePalette() {
    if (referencePalette) return referencePalette;
    const id = currentSettings && currentSettings.lighting_preset;
    const preset = getLightingPreset(id) || getLightingPreset('default');
    if (!preset) {
        return {
            bright: { r: 1, g: 1, b: 1 },
            mid:    { r: 1, g: 1, b: 1 },
            dark:   { r: 0.27, g: 0.27, b: 0.27 },
            key:    { r: 1, g: 1, b: 1 },
            fill:   { r: 1, g: 1, b: 1 },
        };
    }
    return resolvePresetColors(preset.bake_config);
}

// T-007-02: Live-preview palette resolution. Same priority order as
// getActiveBakePalette but reads preview_config so per-preset live
// attenuations can diverge from the bake without losing color
// identity.
function getActivePreviewPalette() {
    if (referencePalette) return referencePalette;
    const id = currentSettings && currentSettings.lighting_preset;
    const preset = getLightingPreset(id) || getLightingPreset('default');
    if (!preset) {
        return {
            bright: { r: 1, g: 1, b: 1 },
            mid:    { r: 1, g: 1, b: 1 },
            dark:   { r: 0.19, g: 0.19, b: 0.25 },
            key:    { r: 1, g: 1, b: 1 },
            fill:   { r: 1, g: 1, b: 1 },
        };
    }
    return resolvePresetColors(preset.preview_config);
}

// T-007-02: Apply the active preset's preview_config to the live
// three.js scene's lights in place. The live scene has 1 Ambient + 1
// Hemisphere + 3 Directional lights set up in initThreeJS(). We do
// not rebuild — we walk the scene and mutate color/intensity. If the
// scene hasn't been initialized yet, this is a no-op.
//
// Directional-light role is discriminated by position (set in
// initThreeJS): the original key is at +x +y, dirLight2 is at -x,
// dirLight3 is at -y.
function applyPresetToLiveScene() {
    if (!scene || !currentSettings) return;
    const palette = getActivePreviewPalette();
    const id = currentSettings.lighting_preset;
    const preset = getLightingPreset(id) || getLightingPreset('default');
    if (!preset) return;
    const pc = preset.preview_config;
    const sky    = new THREE.Color(palette.bright.r, palette.bright.g, palette.bright.b);
    const ground = new THREE.Color(palette.dark.r,   palette.dark.g,   palette.dark.b);
    const key    = new THREE.Color(palette.key.r,    palette.key.g,    palette.key.b);
    const fill   = new THREE.Color(palette.fill.r,   palette.fill.g,   palette.fill.b);

    scene.traverse((obj) => {
        if (obj.isAmbientLight) {
            obj.color.copy(sky);
            obj.intensity = pc.ambient;
        } else if (obj.isHemisphereLight) {
            obj.color.copy(sky);
            obj.groundColor.copy(ground);
            obj.intensity = pc.hemisphere_intensity;
        } else if (obj.isDirectionalLight) {
            if (obj.position.y < 0) {
                // Under-fill (dirLight3 in initThreeJS).
                obj.color.copy(fill);
                obj.intensity = pc.fill_intensity;
            } else if (obj.position.x < 0) {
                // Back/rim (dirLight2). Soft sky tint at attenuated key.
                obj.color.copy(sky);
                obj.intensity = pc.key_intensity * 0.55;
            } else {
                // Main key (dirLight).
                obj.color.copy(key);
                obj.intensity = pc.key_intensity;
            }
        }
    });
}

// Real-time tuning feedback: every slider in the right panel writes
// directly to the live preview's renderer + lights + materials so the
// user can see what the bake will look like as they drag.
//
// Bake-only fields (volumetric_layers, slice_distribution_mode,
// dome_height_factor, ground_align, color_calibration_mode) cannot be
// previewed without a re-bake. Those are skipped here; the bake-stale
// hint is the user's signal that they need to re-run a generate
// action to see the effect of those settings.
//
// applyPresetToLiveScene (T-007-02) sets up the *colors* of each
// light from the active preset's palette. This function applies the
// per-slider *intensity* values on top, mutating in place. Order
// matters: preset first, tuning second, so slider drags override the
// preset's defaults without losing the preset's color tinting.
// Coalesce multiple slider input events into a single per-frame apply.
// Avoids running heavy material/light walks synchronously from inside
// a slider input listener, which had been suspected of competing with
// orbit-controls input handling on the same frame.
let tuningRefreshScheduled = false;
function scheduleLiveTuningRefresh() {
    if (tuningRefreshScheduled) return;
    tuningRefreshScheduled = true;
    requestAnimationFrame(() => {
        tuningRefreshScheduled = false;
        applyTuningToLiveScene();
    });
}

function applyTuningToLiveScene() {
    if (!scene || !renderer || !currentSettings) return;

    // Numeric guard: any undefined / NaN slider value falls back to a
    // sensible non-zero default rather than propagating NaN through
    // Three.js's shading equations (NaN intensities can cause black
    // pixels or worse, depending on the driver).
    const num = (v, fallback) => (typeof v === 'number' && Number.isFinite(v)) ? v : fallback;

    renderer.toneMappingExposure = num(currentSettings.bake_exposure, 1.0);

    const ambient    = num(currentSettings.ambient_intensity,     0.4);
    const hemisphere = num(currentSettings.hemisphere_intensity,  0.5);
    const key        = num(currentSettings.key_light_intensity,   1.4);
    const fill       = num(currentSettings.bottom_fill_intensity, 0.4);

    // Walk the scene's lights and apply per-role intensity.
    // Role discrimination matches applyPresetToLiveScene: ambient =
    // ambient, hemisphere = hemisphere, directional with y<0 = under
    // fill, directional with x<0 = back/rim (uses key * 0.55 same as
    // preset path), other directional = main key.
    scene.traverse((obj) => {
        if (obj.isAmbientLight) {
            obj.intensity = ambient;
        } else if (obj.isHemisphereLight) {
            obj.intensity = hemisphere;
        } else if (obj.isDirectionalLight) {
            if (obj.position.y < 0) {
                obj.intensity = fill;
            } else if (obj.position.x < 0) {
                obj.intensity = key * 0.55;
            } else {
                obj.intensity = key;
            }
        }
    });

    // env_map_intensity affects PBR materials on the currently loaded
    // model. Walk the model and update each PBR material's
    // envMapIntensity in place. Cheap, no allocations.
    if (currentModel) {
        const envIntensity = num(currentSettings.env_map_intensity, 1.2);
        currentModel.traverse((child) => {
            if (!child.isMesh || !child.material) return;
            const mats = Array.isArray(child.material) ? child.material : [child.material];
            for (const m of mats) {
                if (m.isMeshStandardMaterial || m.isMeshPhysicalMaterial) {
                    m.envMapIntensity = envIntensity;
                }
            }
        });
    }
}

// T-007-02: Toggle the "bake out of date" hint. Idempotent. The hint
// is purely client-side, in-memory state per design.md D6 — not
// persisted across reloads.
function setBakeStale(stale) {
    bakeStale = !!stale;
    const el = document.getElementById('bakeStaleHint');
    if (el) el.style.display = bakeStale ? '' : 'none';
}

// Add bake lights to an offscreen scene. Tinted by the reference palette if
// one is loaded, otherwise by the active lighting preset (T-007-01).
// The offscreen scene also gets its own PMREM env (see
// createBakeEnvironment) so PBR materials have IBL — these direct lights
// supply the highlights and shadows on top.
// Omnidirectional bake lighting — rotationally symmetric around the Y axis so
// every billboard angle gets the same illumination. No side-biased directional
// lights. Palette colors are used as TINTS (near-white normalized colors), so
// light intensities can stay high without darkening.
function setupBakeLights(offScene) {
    const palette = getActiveBakePalette();
    const sky    = new THREE.Color(palette.bright.r, palette.bright.g, palette.bright.b);
    const fill   = new THREE.Color(palette.mid.r,    palette.mid.g,    palette.mid.b);
    const ground = new THREE.Color(palette.dark.r,   palette.dark.g,   palette.dark.b);

    offScene.add(new THREE.AmbientLight(sky, currentSettings.ambient_intensity));
    // Hemisphere light gives top→bottom color gradient without horizontal bias
    offScene.add(new THREE.HemisphereLight(sky, ground, currentSettings.hemisphere_intensity));
    // Pure top-down key — illuminates everything regardless of viewing angle
    const dlTop = new THREE.DirectionalLight(sky, currentSettings.key_light_intensity);
    dlTop.position.set(0, 10, 0);
    offScene.add(dlTop);
    // Subtle bottom fill so the underside of foliage isn't black
    const dlBottom = new THREE.DirectionalLight(fill, currentSettings.bottom_fill_intensity);
    dlBottom.position.set(0, -10, 0);
    offScene.add(dlBottom);
}

// Build a vertical-gradient equirectangular env texture from three RGB
// stops (top, mid, bottom). Each stop is either {r,g,b} 0..1 or [r,g,b].
// Used by createBakeEnvironment for both the reference-palette path and
// the active-preset path (T-007-01).
function buildGradientEnvTexture(stops, pmrem) {
    const norm = (s) => Array.isArray(s) ? { r: s[0], g: s[1], b: s[2] } : s;
    const top = norm(stops[0]);
    const mid = norm(stops[1]);
    const bot = norm(stops[2]);
    const w = 256, h = 128;
    const cv = document.createElement('canvas');
    cv.width = w; cv.height = h;
    const ctx = cv.getContext('2d');
    const grad = ctx.createLinearGradient(0, 0, 0, h);
    const c = (col) => `rgb(${Math.round(col.r * 255)},${Math.round(col.g * 255)},${Math.round(col.b * 255)})`;
    grad.addColorStop(0,   c(top));
    grad.addColorStop(0.5, c(mid));
    grad.addColorStop(1,   c(bot));
    ctx.fillStyle = grad;
    ctx.fillRect(0, 0, w, h);

    const tex = new THREE.CanvasTexture(cv);
    tex.mapping = THREE.EquirectangularReflectionMapping;
    tex.colorSpace = THREE.SRGBColorSpace;
    tex.needsUpdate = true;
    const env = pmrem.fromEquirectangular(tex).texture;
    tex.dispose();
    return env;
}

// Build a PMREM environment map ON THE GIVEN RENDERER. This is the key fix
// for the bake — we can't reuse the main renderer's PMREM texture in an
// offscreen context, so each offscreen render generates its own env using
// its own PMREMGenerator. Source priority (T-007-01):
//   1. Reference palette if loaded.
//   2. Active lighting preset's env_gradient (skipped for 'default' so
//      the legacy RoomEnvironment is preserved as the neutral baseline).
//   3. RoomEnvironment fallback.
function createBakeEnvironment(renderer) {
    const pmrem = new THREE.PMREMGenerator(renderer);
    pmrem.compileEquirectangularShader();

    let env;
    if (referencePalette) {
        env = buildGradientEnvTexture(
            [referencePalette.bright, referencePalette.mid, referencePalette.dark],
            pmrem,
        );
    } else {
        const presetId = currentSettings && currentSettings.lighting_preset;
        const preset = getLightingPreset(presetId);
        if (preset && preset.id !== 'default' && preset.bake_config.env_gradient) {
            env = buildGradientEnvTexture(preset.bake_config.env_gradient, pmrem);
        } else {
            // Neutral default: RoomEnvironment, generated on this renderer
            env = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;
        }
    }

    pmrem.dispose();
    return env;
}

// Clone a model for offscreen baking. Clones materials so the offscreen
// renderer can drive IBL via scene.environment without interfering with the
// originals. envMap is left null and envMapIntensity boosted — Three.js will
// auto-bind scene.environment to PBR materials at render time.
function cloneModelForBake(model) {
    const clone = model.clone(true);
    clone.traverse((child) => {
        if (!child.isMesh || !child.material) return;
        const mats = Array.isArray(child.material) ? child.material : [child.material];
        const cloned = mats.map((m) => {
            const c = m.clone();
            if ('envMap' in c) c.envMap = null;
            if ('envMapIntensity' in c) c.envMapIntensity = currentSettings.env_map_intensity;
            c.needsUpdate = true;
            return c;
        });
        child.material = Array.isArray(child.material) ? cloned : cloned[0];
    });
    return clone;
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
    offRenderer.toneMappingExposure = currentSettings.bake_exposure;

    const halfW = size.x * 0.55;
    const halfD = size.z * 0.55;
    const half = Math.max(halfW, halfD);
    const offCamera = new THREE.OrthographicCamera(-half, half, half, -half, 0.01, size.y * 10);
    offCamera.position.set(center.x, center.y + size.y * 2, center.z);
    offCamera.lookAt(center);

    const offScene = new THREE.Scene();
    const bakeEnv = createBakeEnvironment(offRenderer);
    offScene.environment = bakeEnv;
    setupBakeLights(offScene);

    const clone = cloneModelForBake(model);
    offScene.add(clone);
    offRenderer.render(offScene, offCamera);

    const canvas = offRenderer.domElement;
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);
    bakeEnv.dispose();
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
                alphaTest: currentSettings.alpha_test,
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
            alphaTest: currentSettings.alpha_test,
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

// T-009-01: Tilted-bake counterpart of `renderMultiAngleBillboardGLB`.
// Side variants only — no `billboard_top` quad — captured from an
// elevated camera (`elevationRad` radians above horizontal). Quad
// naming is preserved (`billboard_${i}`); the runtime loader (T-009-02)
// will discriminate by file path, not by quad name.
function renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution) {
    return new Promise((resolve, reject) => {
        const exportScene = new THREE.Scene();

        for (let i = 0; i < numAngles; i++) {
            const angle = (i / numAngles) * Math.PI * 2;
            const { canvas, quadWidth, quadHeight } = renderBillboardAngle(model, angle, resolution, elevationRad);

            const texture = new THREE.CanvasTexture(canvas);
            texture.colorSpace = THREE.SRGBColorSpace;

            const geometry = new THREE.PlaneGeometry(quadWidth, quadHeight);
            geometry.translate(0, quadHeight / 2, 0); // bottom edge at y=0

            const material = new THREE.MeshBasicMaterial({
                map: texture,
                transparent: true,
                side: THREE.DoubleSide,
                alphaTest: currentSettings.alpha_test,
            });

            const quad = new THREE.Mesh(geometry, material);
            quad.name = `billboard_${i}`;
            quad.position.set(i * quadWidth * 1.2, 0, 0);
            exportScene.add(quad);
        }

        const exporter = new GLTFExporter();
        exporter.parse(exportScene, (result) => {
            resolve(result);
        }, (err) => {
            reject(err);
        }, { binary: true });
    });
}

// T-009-01: Devtools reach. `app.js` is loaded as a module, so its
// top-level bindings are not on `window`. Expose the new generator
// (and a getter for the currently-selected file id) so manual
// verification can run `await generateTiltedBillboard(selectedFileId)`
// from the console without source edits.
window.generateTiltedBillboard = generateTiltedBillboard;
Object.defineProperty(window, 'selectedFileId', {
    get() { return selectedFileId; },
    configurable: true,
});

// ── Volumetric Distillation (Horizontal Layer Peeling) ──
// Slices the model into horizontal bands and renders each from above with
// everything above that band clipped away. The result is stacked horizontal
// quads at their respective heights — a "top-down MRI" of the plant.
// Note: layer count and resolution are now read from currentSettings
// (volumetric_layers / volumetric_resolution). T-002-01 made these
// per-asset and T-002-02 wired them in.

async function generateVolumetric(id) {
    if (!currentModel || !threeReady) return;

    generateVolumetricBtn.textContent = 'Building…';
    generateVolumetricBtn.disabled = true;

    let success = false;
    try {
        const glb = await renderHorizontalLayerGLB(currentModel, currentSettings.volumetric_layers, currentSettings.volumetric_resolution);
        await fetch(`/api/upload-volumetric/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: glb,
        });

        store_update(id, f => f.has_volumetric = true);
        updatePreviewButtons();
        success = true;
        setBakeStale(false); // T-007-02
    } catch (err) { console.error('Volumetric generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'volumetric', success }, id);
    }

    generateVolumetricBtn.textContent = 'Build volumetric dome slices';
    generateVolumetricBtn.disabled = false;
}

// Render a top-down view of the model with a clipping height ceiling.
// Only geometry at or below ceilingY is visible — everything above is clipped away.
function renderLayerTopDown(model, resolution, floorY, ceilingY) {
    const box = new THREE.Box3().setFromObject(model);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const halfExtent = Math.max(size.x, size.z) * 0.55;

    const offRenderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;
    offRenderer.toneMappingExposure = currentSettings.bake_exposure;
    offRenderer.localClippingEnabled = true;

    // Ortho camera looking straight down. Position above the model, far enough
    // that the full visible range (floor to ceiling) is captured.
    const camHeight = ceilingY + size.y * 2;
    const offCamera = new THREE.OrthographicCamera(
        -halfExtent, halfExtent, halfExtent, -halfExtent,
        0.01, camHeight - floorY + 0.01
    );
    offCamera.position.set(center.x, camHeight, center.z);
    offCamera.lookAt(center.x, floorY, center.z);

    // Clipping plane: remove everything above ceilingY
    const clipPlane = new THREE.Plane(new THREE.Vector3(0, -1, 0), ceilingY);

    const offScene = new THREE.Scene();
    // PMREM env on this renderer for proper PBR IBL, tinted by reference palette
    const bakeEnv = createBakeEnvironment(offRenderer);
    offScene.environment = bakeEnv;

    // T-007-01: same active palette resolution as setupBakeLights.
    const palette = getActiveBakePalette();
    const sky    = new THREE.Color(palette.bright.r, palette.bright.g, palette.bright.b);
    const fill   = new THREE.Color(palette.mid.r,    palette.mid.g,    palette.mid.b);
    const ground = new THREE.Color(palette.dark.r,   palette.dark.g,   palette.dark.b);
    // Omni: ambient + hemisphere + pure top-down key. No side-biased lights so
    // each layer renders symmetrically.
    offScene.add(new THREE.AmbientLight(sky, currentSettings.ambient_intensity));
    offScene.add(new THREE.HemisphereLight(sky, ground, currentSettings.hemisphere_intensity));
    const dlTop = new THREE.DirectionalLight(sky, currentSettings.key_light_intensity);
    dlTop.position.set(center.x, ceilingY + 20, center.z);
    offScene.add(dlTop);

    const clone = cloneModelForBake(model);
    clone.traverse((child) => {
        if (child.isMesh && child.material) {
            // Add the clipping plane to the already-cloned bake material
            const mats = Array.isArray(child.material) ? child.material : [child.material];
            for (const m of mats) {
                m.clippingPlanes = [clipPlane];
                m.clipShadows = true;
                m.needsUpdate = true;
            }
        }
    });
    offScene.add(clone);
    offRenderer.render(offScene, offCamera);

    const canvas = offRenderer.domElement;
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);
    bakeEnv.dispose();
    offRenderer.dispose();

    return { canvas: copyCanvas, quadSize: halfExtent * 2 };
}

// Build an umbrella/dome geometry — a subdivided plane with a parabolic Y bulge
// at the center. UVs come from the planar projection so top-down textures map correctly.
function createDomeGeometry(size, domeHeight, segments = 6) {
    const geom = new THREE.PlaneGeometry(size, size, segments, segments);
    geom.rotateX(-Math.PI / 2); // lay flat on XZ plane
    const pos = geom.attributes.position;
    const half = size / 2;
    for (let i = 0; i < pos.count; i++) {
        const x = pos.getX(i);
        const z = pos.getZ(i);
        // Normalized distance from center: 0 at center, 1 at edge
        const dist = Math.min(1, Math.sqrt(x * x + z * z) / half);
        // Parabolic dome: peak in middle, zero at edges
        const y = (1 - dist * dist) * domeHeight;
        pos.setY(i, y);
    }
    pos.needsUpdate = true;
    geom.computeVertexNormals();
    return geom;
}

// T-005-01: simple linear-interpolation boundaries across the model's
// world-space bounding box. This is the legacy "equal-height" mode,
// retained for backwards compatibility on assets that need uniform
// slabs (no vertex-density adaptation, no foliage weighting).
function computeEqualHeightBoundaries(model, numLayers) {
    const box = new THREE.Box3().setFromObject(model);
    const minY = box.min.y, maxY = box.max.y;
    const out = [];
    for (let i = 0; i <= numLayers; i++) {
        out.push(minY + (i / numLayers) * (maxY - minY));
    }
    return out;
}

// T-005-01: visual-density boundaries. Two changes vs. the legacy
// vertex-quantile picker:
//   1. Trunk filter — discard the bottom 10% of bounding-box height
//      so dense stems don't pull all the boundaries downward.
//   2. Radial weight — vertices farther from the central vertical
//      axis count more, so a quantile of *visible foliage* (rather
//      than raw vertex count) decides where the boundaries fall.
//
// The outer boundaries (index 0 and N) come from the unfiltered
// bounding box so the bake camera framing matches the other modes;
// only the interior boundaries shift.
function computeVisualDensityBoundaries(model, numLayers) {
    const ys = [];
    const xs = [];
    const zs = [];
    let minY = Infinity, maxY = -Infinity, maxRadius = 0;
    const v = new THREE.Vector3();
    model.traverse((child) => {
        if (!child.isMesh || !child.geometry) return;
        const pos = child.geometry.attributes.position;
        if (!pos) return;
        child.updateMatrixWorld(true);
        const worldMatrix = child.matrixWorld;
        for (let i = 0; i < pos.count; i++) {
            v.fromBufferAttribute(pos, i).applyMatrix4(worldMatrix);
            ys.push(v.y);
            xs.push(v.x);
            zs.push(v.z);
            if (v.y < minY) minY = v.y;
            if (v.y > maxY) maxY = v.y;
            const r = Math.sqrt(v.x * v.x + v.z * v.z);
            if (r > maxRadius) maxRadius = r;
        }
    });

    if (ys.length === 0) {
        return computeEqualHeightBoundaries(model, numLayers);
    }
    if (maxRadius === 0) maxRadius = 1; // axis-only model; avoid div-by-zero

    const trunkY = minY + 0.10 * (maxY - minY);

    // Filter and weight in lockstep.
    const pairs = [];
    let totalWeight = 0;
    for (let i = 0; i < ys.length; i++) {
        if (ys[i] < trunkY) continue;
        const r = Math.sqrt(xs[i] * xs[i] + zs[i] * zs[i]);
        let w = r / maxRadius;
        if (w < 0.05) w = 0.05;
        if (w > 1.0) w = 1.0;
        pairs.push({ y: ys[i], w });
        totalWeight += w;
    }

    if (pairs.length === 0 || totalWeight === 0) {
        // Pathological asset (entire model in the bottom 10%, or all
        // canopy weights collapsed). Fall back to the unfiltered
        // vertex-quantile picker.
        console.warn('visual-density: trunk filter discarded all vertices, falling back to vertex-quantile');
        return computeAdaptiveSliceBoundaries(model, numLayers);
    }

    pairs.sort((a, b) => a.y - b.y);
    const cum = new Array(pairs.length);
    let acc = 0;
    for (let i = 0; i < pairs.length; i++) {
        acc += pairs[i].w;
        cum[i] = acc;
    }

    const boundaries = [minY];
    for (let i = 1; i < numLayers; i++) {
        const target = (i / numLayers) * totalWeight;
        // Smallest k such that cum[k] >= target. Linear scan is fine
        // at the vertex counts we deal with (a binary search would
        // shave microseconds and is not worth the complexity).
        let k = 0;
        while (k < cum.length - 1 && cum[k] < target) k++;
        boundaries.push(pairs[k].y);
    }
    boundaries.push(maxY);
    return boundaries;
}

// Find slice boundaries that distribute vertices evenly across N layers.
// A bottom-heavy bush will get more slices near the bottom and fewer up top
// because that's where the polygon density actually lives. Returns an array
// of N+1 Y values (floor of slice 0 ... ceiling of slice N-1).
function computeAdaptiveSliceBoundaries(model, numLayers) {
    const ys = [];
    const v = new THREE.Vector3();
    model.traverse((child) => {
        if (!child.isMesh || !child.geometry) return;
        const pos = child.geometry.attributes.position;
        if (!pos) return;
        child.updateMatrixWorld(true);
        const worldMatrix = child.matrixWorld;
        for (let i = 0; i < pos.count; i++) {
            v.fromBufferAttribute(pos, i).applyMatrix4(worldMatrix);
            ys.push(v.y);
        }
    });

    if (ys.length === 0) {
        const box = new THREE.Box3().setFromObject(model);
        const minY = box.min.y, maxY = box.max.y;
        const out = [];
        for (let i = 0; i <= numLayers; i++) {
            out.push(minY + (i / numLayers) * (maxY - minY));
        }
        return out;
    }

    ys.sort((a, b) => a - b);
    const boundaries = [ys[0]];
    for (let i = 1; i < numLayers; i++) {
        const idx = Math.floor((i / numLayers) * ys.length);
        boundaries.push(ys[idx]);
    }
    boundaries.push(ys[ys.length - 1]);
    return boundaries;
}

// Pick how many layers to use based on the model's aspect ratio.
// Tall thin models need more slices to capture vertical structure;
// short wide models do fine with fewer.
function pickAdaptiveLayerCount(model, baseLayers) {
    const box = new THREE.Box3().setFromObject(model);
    const size = box.getSize(new THREE.Vector3());
    const heightToWidth = size.y / Math.max(size.x, size.z, 0.0001);
    if (heightToWidth > 2.5) return baseLayers + 2;
    if (heightToWidth > 1.5) return baseLayers + 1;
    return baseLayers;
}

// T-004-03: resolve a slice-axis sentinel against the model's
// world-space bounding box and return a {rotation, inverse} pair of
// THREE.Quaternions. Slicing always operates on a Y-up frame, so for
// non-Y axes we rotate the model into a Y-aligned working frame,
// slice as before, then apply the inverse rotation to the export
// scene root so the produced GLB lives in the original world frame.
//
//   - 'y'                : identity (current behavior).
//   - 'auto-horizontal'  : whichever of X / Z is the longer extent.
//   - 'auto-thin'        : whichever of X / Y / Z is the shortest.
//
// Unknown / empty modes fall through to identity so a stale setting
// can never crash the bake.
function resolveSliceAxisRotation(model, mode) {
    const identity = { rotation: new THREE.Quaternion(), inverse: new THREE.Quaternion() };
    if (!mode || mode === 'y') return identity;

    const box = new THREE.Box3().setFromObject(model);
    const size = box.getSize(new THREE.Vector3());

    let pickAxis;
    if (mode === 'auto-horizontal') {
        pickAxis = (size.x >= size.z) ? 'x' : 'z';
    } else if (mode === 'auto-thin') {
        const m = Math.min(size.x, size.y, size.z);
        if (m === size.x) pickAxis = 'x';
        else if (m === size.z) pickAxis = 'z';
        else pickAxis = 'y';
    } else {
        return identity;
    }

    if (pickAxis === 'y') return identity;

    // Build a quaternion that rotates the chosen unit axis to +Y.
    const from = new THREE.Vector3(
        pickAxis === 'x' ? 1 : 0,
        0,
        pickAxis === 'z' ? 1 : 0,
    );
    const to = new THREE.Vector3(0, 1, 0);
    const rotation = new THREE.Quaternion().setFromUnitVectors(from, to);
    const inverse = rotation.clone().invert();
    return { rotation, inverse };
}

function renderHorizontalLayerGLB(model, numLayers, resolution) {
    return new Promise((resolve, reject) => {
        const exportScene = new THREE.Scene();

        // T-004-03: if the strategy router selected a non-Y slice
        // axis (directional → auto-horizontal, planar → auto-thin),
        // wrap the model in a temporary group whose rotation aligns
        // the chosen axis with +Y. The slicing pipeline below is
        // unchanged — it always cuts along Y in its working frame.
        // The inverse rotation is applied to the exportScene root at
        // the bottom so the produced GLB sits in the original world
        // frame.
        const sliceAxisMode = currentSettings.slice_axis || 'y';
        const { rotation, inverse } = resolveSliceAxisRotation(model, sliceAxisMode);
        let workModel = model;
        if (rotation.w !== 1 || rotation.x !== 0 || rotation.y !== 0 || rotation.z !== 0) {
            const wrapper = new THREE.Group();
            wrapper.quaternion.copy(rotation);
            wrapper.add(model);
            wrapper.updateMatrixWorld(true);
            workModel = wrapper;
        }

        // Adaptive: pick layer count by aspect ratio, then dispatch on
        // the per-asset slice_distribution_mode (T-005-01). The default
        // case falls through to the legacy vertex-quantile picker so a
        // missing/stale setting degrades to the prior behavior, never
        // a crash.
        const actualLayers = pickAdaptiveLayerCount(workModel, numLayers);
        const mode = currentSettings.slice_distribution_mode;
        let boundaries;
        switch (mode) {
            case 'equal-height':
                boundaries = computeEqualHeightBoundaries(workModel, actualLayers);
                break;
            case 'visual-density':
                boundaries = computeVisualDensityBoundaries(workModel, actualLayers);
                break;
            case 'vertex-quantile':
            default:
                boundaries = computeAdaptiveSliceBoundaries(workModel, actualLayers);
                break;
        }

        for (let i = 0; i < actualLayers; i++) {
            const floorY = boundaries[i];
            const ceilingY = boundaries[i + 1];
            const layerThickness = Math.max(ceilingY - floorY, 0.001);

            const { canvas, quadSize } = renderLayerTopDown(workModel, resolution, floorY, ceilingY);

            const texture = new THREE.CanvasTexture(canvas);
            texture.colorSpace = THREE.SRGBColorSpace;

            // Dome height scales with this layer's actual thickness.
            // dome_height_factor wired through currentSettings (T-002-02);
            // T-005-01 is the ticket that exposes it as a tunable end-to-end.
            const domeHeight = layerThickness * currentSettings.dome_height_factor;
            const geometry = createDomeGeometry(quadSize, domeHeight, 6);

            const material = new THREE.MeshBasicMaterial({
                map: texture,
                transparent: true,
                side: THREE.DoubleSide,
                alphaTest: currentSettings.alpha_test,
            });

            const quad = new THREE.Mesh(geometry, material);
            const baseMm = Math.round(floorY * 1000);
            quad.name = `vol_layer_${i}_h${baseMm}`;
            quad.position.set(0, floorY, 0);
            exportScene.add(quad);
        }

        // T-005-01: ground alignment. Translate the export scene root
        // so the floor of the bottom slice (boundaries[0]) sits exactly
        // at Y=0. The bake textures and per-quad inter-spacing are
        // unchanged; only the GLTF root node transform shifts.
        if (currentSettings.ground_align) {
            exportScene.position.y = -boundaries[0];
        }

        // T-004-03: if we sliced in a rotated working frame, apply
        // the inverse rotation to the export root so the produced
        // GLB sits in the original world frame. The slice quads are
        // built in the rotated frame and inherit this rotation; the
        // ground-align translation above happens in the rotated
        // frame too, which is what we want (the bottom slice ends
        // up at Y=0 of the *rotated* space, then the wrapping
        // rotation re-orients the whole stack).
        //
        // Detach the original model from the temporary wrapper so
        // future bake calls in the same session see it in its
        // pristine parent.
        if (workModel !== model) {
            workModel.remove(model);
            const wrap = new THREE.Group();
            wrap.quaternion.copy(inverse);
            wrap.add(exportScene);
            // GLTFExporter expects a Scene at the top; transplant
            // the wrapped result into a fresh export scene.
            const reframed = new THREE.Scene();
            reframed.add(wrap);
            // Replace exportScene with the reframed one for the
            // exporter call below.
            // eslint-disable-next-line no-param-reassign
            exportScene.parent = null;
            // Use a local alias so the exporter call line stays
            // identical to the pre-T-004-03 shape.
            const exporter = new GLTFExporter();
            exporter.parse(reframed, (result) => {
                resolve(result);
            }, (err) => {
                reject(err);
            }, { binary: true });
            return;
        }

        const exporter = new GLTFExporter();
        exporter.parse(exportScene, (result) => {
            resolve(result);
        }, (err) => {
            reject(err);
        }, { binary: true });
    });
}

// ── Volumetric LOD Chain ──
// LODs reduce layer count and resolution
const VOLUMETRIC_LOD_CONFIGS = [
    { level: 0, layers: 4, resolution: 512, label: 'vlod0' },
    { level: 1, layers: 3, resolution: 256, label: 'vlod1' },
    { level: 2, layers: 2, resolution: 256, label: 'vlod2' },
    { level: 3, layers: 1, resolution: 128, label: 'vlod3' },
];

async function generateVolumetricLODs(id) {
    if (!currentModel || !threeReady) return;

    generateVolumetricLodsBtn.textContent = 'Building…';
    generateVolumetricLodsBtn.classList.add('generating');
    generateVolumetricLodsBtn.disabled = true;

    let success = false;
    try {
        for (const config of VOLUMETRIC_LOD_CONFIGS) {
            const glb = await renderHorizontalLayerGLB(currentModel, config.layers, config.resolution);
            await fetch(`/api/upload-volumetric-lod/${id}?level=${config.level}`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/octet-stream' },
                body: glb,
            });
        }
        await refreshFiles();
        updatePreviewButtons();
        success = true;
        setBakeStale(false); // T-007-02
    } catch (err) { console.error('Volumetric LOD generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'volumetric_lods', success }, id);
    }

    generateVolumetricLodsBtn.textContent = 'Build volumetric LOD chain';
    generateVolumetricLodsBtn.classList.remove('generating');
    generateVolumetricLodsBtn.disabled = false;
}

// ── Production Asset (Hybrid) ──
// One-click generation: billboard for ~horizontal views, volumetric (horizontal
// dome slices) for top-down views, with crossfade at ~45° in the runtime preview.
async function generateProductionAsset(id, onSubstage = () => {}) {
    if (!currentModel || !threeReady) return;

    generateProductionBtn.textContent = 'Rendering via Blender…';
    generateProductionBtn.classList.add('generating');
    generateProductionBtn.disabled = true;

    onSubstage('rendering');

    let success = false;
    try {
        // T-014-05: single API call replaces three client-side render+upload sequences.
        const cat = currentSettings && currentSettings.shape_category;
        const qs = cat ? `?category=${encodeURIComponent(cat)}` : '';
        const resp = await fetch(`/api/build-production/${id}${qs}`, {
            method: 'POST',
        });

        if (!resp.ok) {
            const body = await resp.json().catch(() => ({}));
            throw new Error(body.error || `server error ${resp.status}`);
        }

        const result = await resp.json();

        store_update(id, f => {
            f.has_billboard = result.billboard;
            f.has_billboard_tilted = result.tilted;
            f.has_volumetric = result.volumetric;
        });

        // T-011-03: stamp bake metadata (server endpoint doesn't do this).
        await fetch(`/api/bake-complete/${id}`, { method: 'POST' });

        await refreshFiles();
        updatePreviewButtons();
        success = true;
        setBakeStale(false); // T-007-02
    } catch (err) { console.error('Production asset generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'production', success }, id);
    }

    generateProductionBtn.textContent = 'Build hybrid impostor';
    generateProductionBtn.classList.remove('generating');
    generateProductionBtn.disabled = false;
}

// ── Build Asset Pack (T-010-03) ──
//
// Wires the existing /api/pack/:id endpoint into the toolbar. The
// endpoint runs CombinePack against the three intermediates that the
// "Build hybrid impostor" flow has already uploaded and writes a
// finished `dist/plants/{species}.glb`.
//
// Status / error surfaces:
//   200 → success line in #prepareError
//   413 → canonical "Pack exceeds 5 MB" message in #prepareError
//   any other non-2xx → server's error string in #prepareError
//
// We always emit a `pack_built` analytics event, encoding failures
// as size: 0 — same convention as the existing `regenerate` events.
async function buildAssetPack(id) {
    if (!id) return;
    const file = files.find(f => f.id === id);
    if (!file) return;

    buildPackBtn.disabled = true;
    const previousLabel = buildPackBtn.textContent;
    buildPackBtn.textContent = 'Packing…';
    buildPackBtn.classList.add('generating');
    if (prepareError) prepareError.textContent = '';

    let species = '';
    let size = 0;
    try {
        const res = await fetch(`/api/pack/${id}`, { method: 'POST' });
        let body = null;
        try { body = await res.json(); } catch (_) { body = null; }

        if (res.ok && body) {
            species = body.species || '';
            size = body.size || 0;
            if (prepareError) {
                prepareError.textContent = `Pack built: ${species}.glb (${size} bytes)`;
            }
        } else if (res.status === 413) {
            if (prepareError) {
                prepareError.textContent = 'Pack exceeds 5 MB — reduce variant count or texture resolution and re-bake';
            }
        } else {
            const msg = (body && body.error) ? body.error : `HTTP ${res.status}`;
            if (prepareError) {
                prepareError.textContent = `Pack failed: ${msg}`;
            }
        }
    } catch (err) {
        console.error('Build asset pack failed:', err);
        if (prepareError) {
            prepareError.textContent = `Pack failed: ${err.message || err}`;
        }
    } finally {
        logEvent('pack_built', {
            species,
            size,
            has_tilted: !!file.has_billboard_tilted,
            has_dome:   !!file.has_volumetric,
        }, id);
    }

    buildPackBtn.textContent = previousLabel;
    buildPackBtn.classList.remove('generating');
    buildPackBtn.disabled = false;
    updatePreviewButtons();
}

// ── Prepare for Scene (T-008-01) ──
//
// Single primary action that runs the existing pipeline stages in order
// against the selected asset: gltfpack optimize → classify → LOD chain →
// production asset (billboard + volumetric). Stops on the first failure.
// Emits a `prepare_for_scene` analytics event summarizing the run; the
// underlying functions still emit their own per-stage `regenerate` events.
//
// Failure detection: each stage adapter sniffs the file record / settings
// after the existing function returns, since those functions swallow
// errors internally and only signal via DOM/store mutations. This is
// intentionally lightweight per design.md option A.

const PREPARE_STAGES = [
    { id: 'gltfpack',   label: 'Optimize' },
    { id: 'classify',   label: 'Classify' },
    { id: 'lods',       label: 'LOD' },
    { id: 'production', label: 'Production asset' },
];

function setPrepareStages(stages) {
    if (!prepareStages) return;
    prepareStages.innerHTML = '';
    for (const s of stages) {
        const li = document.createElement('li');
        li.dataset.stage = s.id;
        li.className = 'pending';
        li.textContent = `[ ] ${s.label}`;
        prepareStages.appendChild(li);
    }
}

function markPrepareStage(stageId, status, msg) {
    if (!prepareStages) return;
    const li = prepareStages.querySelector(`li[data-stage="${stageId}"]`);
    if (!li) return;
    const label = PREPARE_STAGES.find(s => s.id === stageId)?.label || stageId;
    const glyph = status === 'ok' ? '✓' : status === 'error' ? '✗' : status === 'running' ? '•' : ' ';
    li.className = status;
    li.textContent = `[${glyph}] ${label}${msg ? ' — ' + msg : ''}`;
}

async function prepareForScene(id) {
    if (!id) return;
    const file = files.find(f => f.id === id);
    if (!file) return;
    if (!currentModel) {
        console.warn('prepareForScene: no currentModel loaded');
        return;
    }

    prepareForSceneBtn.disabled = true;
    const originalLabel = prepareForSceneBtn.textContent;
    prepareForSceneBtn.textContent = 'Preparing…';
    prepareProgress.style.display = 'flex';
    prepareError.textContent = '';
    viewInSceneBtn.style.display = 'none';
    setPrepareStages(PREPARE_STAGES);

    const t0 = performance.now();
    const stagesRun = [];
    let success = true;
    let failedStage = null;
    let failedError = null;

    try {
        // Stage 1: gltfpack cleanup. Skip if already optimized.
        {
            const f = files.find(x => x.id === id);
            if (f && f.status === 'done') {
                markPrepareStage('gltfpack', 'ok', 'already optimized');
            } else {
                markPrepareStage('gltfpack', 'running');
                stagesRun.push('gltfpack');
                await processFile(id);
                const after = files.find(x => x.id === id);
                if (!after || after.status !== 'done') {
                    throw new Error(`gltfpack failed (status: ${after && after.status})`);
                }
                markPrepareStage('gltfpack', 'ok');
            }
        }

        // Stage 2: classify shape. Skip if already classified.
        {
            const conf = currentSettings && currentSettings.shape_confidence;
            if (conf && conf > 0) {
                markPrepareStage('classify', 'ok', `${currentSettings.shape_category}`);
            } else {
                markPrepareStage('classify', 'running');
                stagesRun.push('classify');
                const { settings } = await fetchClassification(id);
                if (settings) {
                    currentSettings = settings;
                    populateTuningUI();
                }
                markPrepareStage('classify', 'ok', currentSettings && currentSettings.shape_category);
            }
        }

        // Stage 3: LOD chain via gltfpack.
        {
            markPrepareStage('lods', 'running');
            stagesRun.push('lods');
            await generateLODs(id);
            const after = files.find(x => x.id === id);
            const ok = after && Array.isArray(after.lods) && after.lods.length > 0
                && !after.lods.some(l => l && l.error);
            if (!ok) throw new Error('LOD generation failed');
            markPrepareStage('lods', 'ok');
        }

        // Stage 4: production asset (billboard + tilted billboard + volumetric).
        {
            markPrepareStage('production', 'running');
            stagesRun.push('production');
            // T-009-03: surface each sub-bake in the running label so
            // the user sees progress through the three impostor passes.
            await generateProductionAsset(id, (substage) => {
                markPrepareStage('production', 'running', `${substage} bake…`);
            });
            const after = files.find(x => x.id === id);
            if (!after || !after.has_billboard || !after.has_billboard_tilted || !after.has_volumetric) {
                throw new Error('production asset failed');
            }
            markPrepareStage('production', 'ok');
        }
    } catch (err) {
        success = false;
        failedStage = stagesRun[stagesRun.length - 1] || 'unknown';
        failedError = (err && err.message) || String(err);
        markPrepareStage(failedStage, 'error', failedError);
        prepareError.textContent = failedError;
        console.error('prepareForScene failed:', err);
    } finally {
        const totalDurationMs = Math.round(performance.now() - t0);
        const payload = {
            stages_run: stagesRun,
            total_duration_ms: totalDurationMs,
            success,
        };
        if (!success) {
            payload.failed_stage = failedStage;
            payload.error = failedError;
        }
        logEvent('prepare_for_scene', payload, id);

        prepareForSceneBtn.textContent = originalLabel;
        prepareForSceneBtn.disabled = !(selectedFileId && currentModel);
        if (success) {
            viewInSceneBtn.style.display = '';
        }
    }
}

window.prepareForScene = prepareForScene;

// ── Lighting Diagnostic ──
// Run a fresh offscreen render with: a known-good red sphere (left half) and
// the current model (right half), using the exact same bake light setup as the
// production asset generation. Read back pixels and report what's actually
// happening — distinguishes "scene is broken" from "model materials are broken".
function testLighting() {
    if (!currentModel) { console.warn('No model loaded'); return; }

    const resolution = 256;
    const offRenderer = new THREE.WebGLRenderer({ antialias: true, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;
    offRenderer.toneMappingExposure = 1.0;

    const box = new THREE.Box3().setFromObject(currentModel);
    const center = box.getCenter(new THREE.Vector3());
    const size = box.getSize(new THREE.Vector3());
    const maxDim = Math.max(size.x, size.y, size.z);

    // Wide ortho camera so both the sphere (placed to the left of the model)
    // and the model (centered) are visible
    const halfH = size.y * 0.6;
    const halfW = maxDim * 1.2;
    const offCamera = new THREE.OrthographicCamera(-halfW, halfW, halfH, -halfH, 0.01, maxDim * 10);
    offCamera.position.set(center.x, center.y, center.z + maxDim * 3);
    offCamera.lookAt(center.x, center.y, center.z);

    const offScene = new THREE.Scene();
    setupBakeLights(offScene);

    // Known-good test sphere — bright red MeshStandardMaterial.
    // If lighting is working at all, this should render bright red.
    const sphereGeom = new THREE.SphereGeometry(maxDim * 0.25, 32, 32);
    const sphereMat = new THREE.MeshStandardMaterial({
        color: 0xff2200,
        roughness: 0.5,
        metalness: 0.0,
    });
    const sphere = new THREE.Mesh(sphereGeom, sphereMat);
    sphere.position.set(center.x - maxDim * 0.7, center.y, center.z);
    offScene.add(sphere);

    // The model under test
    const modelClone = cloneModelForBake(currentModel);
    offScene.add(modelClone);

    offRenderer.render(offScene, offCamera);

    // Read pixel data
    const canvas = offRenderer.domElement;
    const ctx = document.createElement('canvas').getContext('2d');
    ctx.canvas.width = resolution;
    ctx.canvas.height = resolution;
    ctx.drawImage(canvas, 0, 0);
    const data = ctx.getImageData(0, 0, resolution, resolution).data;

    // Sample two regions: left third = sphere, right two-thirds = model
    const sphereStats = sampleRegion(data, resolution, 0, resolution / 3);
    const modelStats = sampleRegion(data, resolution, resolution / 3, resolution);
    const fullStats = sampleRegion(data, resolution, 0, resolution);

    // Report
    const report = [
        '═══ LIGHTING TEST REPORT ═══',
        `Reference loaded: ${referencePalette ? 'YES' : 'no'}`,
        '',
        '── Known-good red sphere (left third) ──',
        `  opaque pixels: ${sphereStats.opaqueCount} / ${sphereStats.total}`,
        `  avg RGB:       ${sphereStats.avgR.toFixed(0)}, ${sphereStats.avgG.toFixed(0)}, ${sphereStats.avgB.toFixed(0)}`,
        `  max RGB:       ${sphereStats.maxR}, ${sphereStats.maxG}, ${sphereStats.maxB}`,
        `  brightness:    ${sphereStats.avgLuminance.toFixed(0)} / 255`,
        '',
        '── Current model (right two-thirds) ──',
        `  opaque pixels: ${modelStats.opaqueCount} / ${modelStats.total}`,
        `  avg RGB:       ${modelStats.avgR.toFixed(0)}, ${modelStats.avgG.toFixed(0)}, ${modelStats.avgB.toFixed(0)}`,
        `  max RGB:       ${modelStats.maxR}, ${modelStats.maxG}, ${modelStats.maxB}`,
        `  brightness:    ${modelStats.avgLuminance.toFixed(0)} / 255`,
        '',
        '── Diagnosis ──',
    ];

    if (sphereStats.opaqueCount === 0) {
        report.push('  ✗ Sphere not rendered at all — offscreen render pipeline is broken');
    } else if (sphereStats.maxR < 50) {
        report.push('  ✗ Sphere is dark — bake lights are not reaching geometry');
    } else if (sphereStats.maxR > 150) {
        report.push('  ✓ Sphere is bright red — lighting works correctly');
    } else {
        report.push('  ⚠ Sphere is dim — lights work but underexposed');
    }

    if (modelStats.opaqueCount === 0) {
        report.push('  ✗ Model not rendered — model not in scene or all transparent');
    } else if (modelStats.avgLuminance < 10) {
        report.push('  ✗ Model is pitch black — model materials problem (not lighting)');
    } else if (modelStats.avgLuminance < 40) {
        report.push('  ⚠ Model is dim');
    } else {
        report.push('  ✓ Model is rendering with visible brightness');
    }

    offRenderer.dispose();

    // ── Stage 2: pipeline roundtrip test ──
    // Take the bake canvas → CanvasTexture → MeshBasicMaterial quad → GLTFExporter
    // → GLTFLoader → render to a fresh canvas → read pixels.
    // This is the EXACT path the production asset takes. If the result is dim,
    // we know which stage dropped brightness.
    runPipelineRoundtrip(canvas, modelStats).then((roundtripStats) => {
        report.push('');
        report.push('── Pipeline roundtrip (bake → GLB → reload → render) ──');
        report.push(`  bake brightness:      ${modelStats.avgLuminance.toFixed(0)} / 255`);
        report.push(`  after GLB roundtrip:  ${roundtripStats.avgLuminance.toFixed(0)} / 255`);
        report.push(`  delta:                ${(roundtripStats.avgLuminance - modelStats.avgLuminance).toFixed(0)}`);
        if (roundtripStats.avgLuminance < modelStats.avgLuminance * 0.3) {
            report.push('  ✗ MAJOR brightness loss in GLB pipeline — encoding or material issue');
        } else if (roundtripStats.opaqueCount === 0) {
            report.push('  ✗ Roundtrip texture is fully transparent — alpha problem');
        } else {
            report.push('  ✓ Pipeline roundtrip preserves brightness');
        }

        const finalText = report.join('\n');
        console.log(finalText);
        showTestImage(canvas, finalText, roundtripStats.canvas);
    });
}

// Take a baked canvas, wrap it as a textured quad, export to GLB, reload via
// GLTFLoader, render the loaded quad to a fresh canvas, and read pixels.
async function runPipelineRoundtrip(bakeCanvas, _bakeStats) {
    // Build an export scene with one textured quad (same as production asset)
    const exportScene = new THREE.Scene();
    const tex = new THREE.CanvasTexture(bakeCanvas);
    tex.colorSpace = THREE.SRGBColorSpace;
    const geom = new THREE.PlaneGeometry(2, 2);
    const mat = new THREE.MeshBasicMaterial({
        map: tex,
        transparent: true,
        side: THREE.DoubleSide,
        alphaTest: 0.1,
    });
    const quad = new THREE.Mesh(geom, mat);
    exportScene.add(quad);

    // Export to GLB
    const exporter = new GLTFExporter();
    const glbArrayBuffer = await new Promise((resolve, reject) => {
        exporter.parse(exportScene, resolve, reject, { binary: true });
    });

    // Reload via GLTFLoader from the exported GLB
    const blob = new Blob([glbArrayBuffer], { type: 'model/gltf-binary' });
    const url = URL.createObjectURL(blob);
    const reloadLoader = new GLTFLoader();
    const loaded = await new Promise((resolve, reject) => {
        reloadLoader.load(url, resolve, undefined, reject);
    });
    URL.revokeObjectURL(url);

    // Render the reloaded quad to a fresh offscreen canvas
    const resolution = 256;
    const offRenderer = new THREE.WebGLRenderer({ antialias: false, alpha: true, preserveDrawingBuffer: true });
    offRenderer.setSize(resolution, resolution);
    offRenderer.setClearColor(0x000000, 0);
    offRenderer.outputColorSpace = THREE.SRGBColorSpace;
    offRenderer.toneMapping = THREE.ACESFilmicToneMapping;
    offRenderer.toneMappingExposure = 1.0;

    const offScene = new THREE.Scene();
    offScene.add(loaded.scene);

    const offCamera = new THREE.OrthographicCamera(-1.1, 1.1, 1.1, -1.1, 0.1, 10);
    offCamera.position.set(0, 0, 3);
    offCamera.lookAt(0, 0, 0);

    offRenderer.render(offScene, offCamera);

    const canvas = offRenderer.domElement;
    const copyCanvas = document.createElement('canvas');
    copyCanvas.width = resolution;
    copyCanvas.height = resolution;
    copyCanvas.getContext('2d').drawImage(canvas, 0, 0);

    const data = copyCanvas.getContext('2d').getImageData(0, 0, resolution, resolution).data;
    const stats = sampleRegion(data, resolution, 0, resolution);
    stats.canvas = copyCanvas;

    offRenderer.dispose();
    return stats;
}

function sampleRegion(data, width, xStart, xEnd) {
    let r = 0, g = 0, b = 0, count = 0;
    let maxR = 0, maxG = 0, maxB = 0;
    let total = 0;
    const xS = Math.floor(xStart);
    const xE = Math.floor(xEnd);
    for (let y = 0; y < width; y++) {
        for (let x = xS; x < xE; x++) {
            const idx = (y * width + x) * 4;
            total++;
            if (data[idx + 3] < 10) continue;
            r += data[idx];
            g += data[idx + 1];
            b += data[idx + 2];
            count++;
            if (data[idx] > maxR) maxR = data[idx];
            if (data[idx + 1] > maxG) maxG = data[idx + 1];
            if (data[idx + 2] > maxB) maxB = data[idx + 2];
        }
    }
    const avgR = count > 0 ? r / count : 0;
    const avgG = count > 0 ? g / count : 0;
    const avgB = count > 0 ? b / count : 0;
    return {
        opaqueCount: count,
        total,
        avgR, avgG, avgB,
        maxR, maxG, maxB,
        avgLuminance: 0.2126 * avgR + 0.7152 * avgG + 0.0722 * avgB,
    };
}

function showTestImage(canvas, reportText, roundtripCanvas) {
    const img1 = canvas.toDataURL('image/png');
    const img2 = roundtripCanvas ? roundtripCanvas.toDataURL('image/png') : null;
    const w = window.open('', '_blank', 'width=900,height=900');
    if (!w) return;
    const cb = `linear-gradient(45deg, #333 25%, transparent 25%), linear-gradient(-45deg, #333 25%, transparent 25%), linear-gradient(45deg, transparent 75%, #333 75%), linear-gradient(-45deg, transparent 75%, #333 75%)`;
    w.document.write(`
        <html><head><title>Lighting Test</title>
        <style>
            body { background: #222; color: #ddd; font-family: monospace; padding: 20px; }
            .row { display: flex; gap: 16px; margin-bottom: 16px; }
            .panel h3 { margin: 0 0 8px 0; color: #aaa; font-weight: normal; }
            img { border: 1px solid #555; image-rendering: pixelated; width: 360px; height: 360px; background: ${cb}; background-size: 16px 16px; background-position: 0 0, 0 8px, 8px -8px, -8px 0; }
            pre { background: #111; padding: 10px; border: 1px solid #444; white-space: pre-wrap; font-size: 11px; }
        </style></head><body>
        <h2>Lighting Test</h2>
        <div class="row">
            <div class="panel">
                <h3>Stage 1: Direct bake (sphere + model)</h3>
                <img src="${img1}">
            </div>
            ${img2 ? `<div class="panel"><h3>Stage 2: After GLB roundtrip</h3><img src="${img2}"></div>` : ''}
        </div>
        <pre>${reportText}</pre>
        </body></html>
    `);
}

// ── Reference Image (Environment Map Calibration) ──
// User uploads a reference photo. We use it as an equirectangular environment
// map for IBL — its dominant colors leak into the indirect lighting on the model,
// so leaves get green light cast and warm tones bring out yellows/oranges.
async function uploadReferenceImage(id, file) {
    const formData = new FormData();
    formData.append('image', file);

    try {
        await fetch(`/api/upload-reference/${id}`, { method: 'POST', body: formData });
        const ext = file.name.toLowerCase().endsWith('.jpg') ? '.jpg' : '.png';
        store_update(id, f => { f.has_reference = true; f.reference_ext = ext; });
        // T-005-03: persist the path tag in the asset's settings so the
        // calibration mode has a referent on disk. Only when the upload
        // is for the currently-selected asset (which is always true via
        // the toolbar/in-panel button paths, but defensive).
        if (currentSettings && selectedFileId === id) {
            currentSettings.reference_image_path = `outputs/${id}_reference${ext}`;
            saveSettings(id);
        }
        // T-005-03: only mutate the live scene when the user has
        // actually opted into reference-image calibration. Uploading
        // while mode is "none" stages the image for later but does not
        // change the current preview.
        if (currentSettings && currentSettings.lighting_preset === 'from-reference-image') {
            await loadReferenceEnvironment(id);
            const f = files.find(x => x.id === id);
            if (f && currentModel) {
                const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
                loadModel(url, lastModelSize);
            }
        }
    } catch (err) {
        console.error('Reference image upload failed:', err);
    }
}

// Sample bright + mid-luminance colors from a reference image and synthesize
// a calibrated environment that PBR materials will use for IBL.
//
// The reference photo is NOT used as an equirectangular env map directly —
// that only works for HDR panoramas and gives bad results with portrait photos
// (dark background = dark scene). Instead we:
//   1. Sample dominant bright color → use as "sky" (top half of synthetic env)
//   2. Sample dominant mid color → use as "ground" (bottom half)
//   3. Build a vertical gradient canvas, PMREM-encode it as the env map
//   4. Tint the ambient/hemisphere lights with the same colors
function loadReferenceEnvironment(id) {
    return new Promise((resolve) => {
        const img = new Image();
        img.crossOrigin = 'anonymous';
        img.onload = () => {
            const palette = extractPalette(img);
            referencePalette = palette;
            buildSyntheticEnvironment(palette);
            applyReferenceTint(palette);
            if (currentModel) applyEnvironmentToModel(currentModel);
            resolve();
        };
        img.onerror = () => { console.error('Failed to load reference image'); resolve(); };
        img.src = `/api/reference/${id}?t=${Date.now()}`;
    });
}

// Extract calibration colors from a reference image.
//
// Strategy:
//   1. Sample corners to estimate background color
//   2. Build a binary subject mask: keep only pixels far from the background
//   3. From subject pixels ONLY, extract:
//        - bright: top-luminance highlights (the natural sunlit color)
//        - mid:    median-luminance pixels (the dominant subject color)
//        - dark:   bottom-luminance pixels but clamped (genuine subject shadows)
//   4. Convert to "tints": colors are normalized so their max channel is 1.0
//      and used as MULTIPLIERS on bright neutral white lighting. This means
//      the calibration shifts hue without dimming the scene.
function extractPalette(img) {
    const sampleSize = 128;
    const canvas = document.createElement('canvas');
    canvas.width = sampleSize;
    canvas.height = sampleSize;
    const ctx = canvas.getContext('2d');
    ctx.drawImage(img, 0, 0, sampleSize, sampleSize);
    const data = ctx.getImageData(0, 0, sampleSize, sampleSize).data;

    const bgColor = sampleCorners(data, sampleSize);

    // Step 2: build the subject mask. A pixel is "subject" if it's far from
    // the background color in RGB space. The threshold is generous to catch
    // edges/anti-aliased pixels that are slightly closer to the bg.
    const subjectPixels = []; // { r, g, b, lum }
    for (let i = 0; i < data.length; i += 4) {
        const r = data[i];
        const g = data[i + 1];
        const b = data[i + 2];
        const a = data[i + 3];
        if (a < 128) continue;

        const dr = r - bgColor.r;
        const dg = g - bgColor.g;
        const db = b - bgColor.b;
        const bgDist = Math.sqrt(dr * dr + dg * dg + db * db);
        if (bgDist < 50) continue; // background

        const rf = r / 255, gf = g / 255, bf = b / 255;
        const lum = 0.2126 * rf + 0.7152 * gf + 0.0722 * bf;
        subjectPixels.push({ r: rf, g: gf, b: bf, lum });
    }

    if (subjectPixels.length === 0) {
        return {
            bright: { r: 1, g: 1, b: 1 },
            mid: { r: 1, g: 1, b: 1 },
            dark: { r: 1, g: 1, b: 1 },
        };
    }

    // Sort subject by luminance and slice into tonal regions
    subjectPixels.sort((a, b) => a.lum - b.lum);
    const n = subjectPixels.length;

    // Bright: top 15% by luminance (highlights)
    // Mid:    middle 30% (the bulk of the subject)
    // Dark:   bottom 15% (genuine shadows)
    const brightSlice = subjectPixels.slice(Math.floor(n * 0.85));
    const midSlice = subjectPixels.slice(Math.floor(n * 0.35), Math.floor(n * 0.65));
    const darkSlice = subjectPixels.slice(0, Math.floor(n * 0.15));

    function avg(arr) {
        if (arr.length === 0) return { r: 1, g: 1, b: 1 };
        let r = 0, g = 0, b = 0;
        for (const p of arr) { r += p.r; g += p.g; b += p.b; }
        return { r: r / arr.length, g: g / arr.length, b: b / arr.length };
    }

    const brightRaw = avg(brightSlice);
    const midRaw = avg(midSlice);
    const darkRaw = avg(darkSlice);

    // Convert each color to a "tint" by normalizing its max channel to 1.0.
    // This preserves the hue but discards brightness — the lights themselves
    // provide the brightness. A warm yellow tint becomes (1.0, 0.92, 0.75)
    // instead of (0.6, 0.55, 0.45) so it shifts color without darkening.
    function toTint(c, minBrightness = 0.7) {
        const max = Math.max(c.r, c.g, c.b);
        if (max < 0.01) return { r: 1, g: 1, b: 1 };
        // Normalize so max channel = 1
        let r = c.r / max;
        let g = c.g / max;
        let b = c.b / max;
        // Optionally pull each channel up toward 1 a bit so the tint stays
        // gentle and doesn't oversaturate
        const blend = (1 - minBrightness);
        r = r * minBrightness + blend;
        g = g * minBrightness + blend;
        b = b * minBrightness + blend;
        return { r, g, b };
    }

    return {
        bright: toTint(brightRaw, 0.85), // keep highlights closer to true white
        mid: toTint(midRaw, 0.75),       // moderate tint for fill
        dark: toTint(darkRaw, 0.7),      // strongest tint for ground/shadow
    };
}

// Clamp a color to a maximum luminance, preserving hue.
// Sample the four corners of an image to estimate background color.
function sampleCorners(data, size) {
    const patch = 8; // 8x8 patch in each corner
    const positions = [
        [0, 0],
        [size - patch, 0],
        [0, size - patch],
        [size - patch, size - patch],
    ];
    let r = 0, g = 0, b = 0, n = 0;
    for (const [x0, y0] of positions) {
        for (let dy = 0; dy < patch; dy++) {
            for (let dx = 0; dx < patch; dx++) {
                const idx = ((y0 + dy) * size + (x0 + dx)) * 4;
                if (data[idx + 3] < 128) continue;
                r += data[idx];
                g += data[idx + 1];
                b += data[idx + 2];
                n++;
            }
        }
    }
    if (n === 0) return { r: 0, g: 0, b: 0 };
    return { r: r / n, g: g / n, b: b / n };
}

// Build a vertical gradient (bright top → mid bottom) and use it as a synthetic
// equirectangular env map. PMREM encoding turns it into a proper IBL texture.
function buildSyntheticEnvironment(palette) {
    const w = 256;
    const h = 128;
    const canvas = document.createElement('canvas');
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext('2d');
    const gradient = ctx.createLinearGradient(0, 0, 0, h);
    const c = (col) => `rgb(${Math.round(col.r * 255)},${Math.round(col.g * 255)},${Math.round(col.b * 255)})`;
    gradient.addColorStop(0, c(palette.bright));
    gradient.addColorStop(0.5, c(palette.mid));
    gradient.addColorStop(1, c(palette.dark));
    ctx.fillStyle = gradient;
    ctx.fillRect(0, 0, w, h);

    const tex = new THREE.CanvasTexture(canvas);
    tex.mapping = THREE.EquirectangularReflectionMapping;
    tex.colorSpace = THREE.SRGBColorSpace;
    tex.needsUpdate = true;

    if (referenceEnvironment) referenceEnvironment.dispose();
    referenceEnvironment = pmremGenerator.fromEquirectangular(tex).texture;
    tex.dispose();

    scene.environment = referenceEnvironment;
}

// Tint the scene's lights with the reference palette so direct
// lighting picks up the calibrated colors. T-007-02: extended to
// also tint the three DirectionalLights — without this, the live
// preview's diffuse highlights stayed neutral white even when the
// reference image had clearly warm or cool colors. Existing
// directional intensities are left intact.
function applyReferenceTint(palette) {
    const sky = new THREE.Color(palette.bright.r, palette.bright.g, palette.bright.b);
    const mid = new THREE.Color(palette.mid.r, palette.mid.g, palette.mid.b);
    scene.traverse((obj) => {
        if (obj.isHemisphereLight) {
            obj.color.copy(sky);
            obj.groundColor.copy(mid);
            obj.intensity = 1.0;
        } else if (obj.isAmbientLight) {
            obj.color.copy(sky);
            obj.intensity = 0.6;
        } else if (obj.isDirectionalLight) {
            if (obj.position.y < 0) {
                // Under-fill: warmer mid tone.
                obj.color.copy(mid);
            } else {
                // Key + back/rim: bright sky tone.
                obj.color.copy(sky);
            }
        }
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
        if (f.is_accepted) {
            metaHTML += ` <span class="accept-mark" title="Accepted">✓</span>`;
        }

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
            if (f.has_volumetric) lodInfo += ' +Vol';
            extraHTML += `<div class="result-info" style="color:var(--text-muted)">${lodInfo}</div>`;
        }

        // Show volumetric LOD info
        if (f.volumetric_lods && f.volumetric_lods.length > 0) {
            let vlodInfo = f.volumetric_lods.map((l, i) => `VL${i}:${formatBytes(l.size)}`).join(' ');
            if (f.volumetric_lod_meta) vlodInfo += ` (${formatBytes(f.volumetric_lod_meta.total_size)} total)`;
            extraHTML += `<div class="result-info" style="color:var(--text-muted)">${vlodInfo}</div>`;
        }

        let commandHTML = '';
        if (f.command) {
            commandHTML = `<div class="command-info" title="Click to copy" onclick="event.stopPropagation(); navigator.clipboard.writeText('${f.command.replace(/'/g, "\\'")}')">${f.command}</div>`;
        }

        let actionsHTML = '';
        if (f.status === 'pending' || f.status === 'done' || f.status === 'error') {
            actionsHTML = `<button class="file-process-btn" onclick="event.stopPropagation(); window._processFile('${f.id}')">${f.status === 'done' ? 'Reprocess' : 'Process'}</button>`;
        }

        // T-005-02: small marker beside the filename when the asset's
        // saved settings diverge from defaults. Server populates
        // settings_dirty in main.go (scan) and handlers.go (PUT).
        const dirtyMark = f.settings_dirty
            ? '<span class="settings-dirty-mark" title="Tuned away from defaults">●</span>'
            : '';

        div.innerHTML = `
            <div class="filename" title="${f.filename}">${f.filename}${dirtyMark}</div>
            <div class="file-meta">${metaHTML}</div>
            ${extraHTML}
            ${actionsHTML}
            ${commandHTML}
            <button class="remove-btn" onclick="event.stopPropagation(); window._deleteFile('${f.id}')">&times;</button>
        `;
        fileList.appendChild(div);
    }

    downloadAllBtn.style.display = hasDone ? 'block' : 'none';
    // T-008-03: keep the first-run hint in sync with the file list.
    updatePlaceholderState();
}

// Proportional wheel-zoom handler that integrates deltaY magnitude
// instead of treating every wheel event as one fixed step. Fixes the
// "trackpad makes one scroll feel like ten" problem.
//
// 5-whys diagnosis (recorded for the next person who hits this):
//
//   Q1: Why does one scroll on a trackpad jump the camera so far?
//   A1: Because OrbitControls' built-in wheel handler treats every
//       wheel event as one fixed dolly step, regardless of deltaY.
//
//   Q2: Why does one physical scroll generate many wheel events?
//   A2: macOS trackpads emit 30+ wheel events per gesture, each with
//       a tiny deltaY. Browsers don't coalesce them.
//
//   Q3: Why doesn't OrbitControls handle this?
//   A3: Three.js 0.160's handler was written for line-mode mouse
//       wheels (one click = one event of deltaY ≈ 100). It only
//       checks the SIGN of deltaY, never the magnitude.
//
//   Q4: Why does lowering controls.zoomSpeed not fix it?
//   A4: It only changes the per-event proportion. With 30 events
//       per gesture, even a small per-event factor compounds to a
//       big total (1.03^30 ≈ 2.4x). The compounding is the problem.
//
//   Q5: Why does integrating deltaY fix it?
//   A5: Sum of deltaY across the 30 micro-events equals the total
//       physical scroll distance, so a single exponential scaling
//       on the cumulative deltaY produces a smooth, predictable
//       per-gesture dolly that's identical regardless of how many
//       events the OS chooses to fire.
//
// Sensitivity is tuned so a normal mouse wheel notch (deltaY ≈ 100)
// produces a ~10% dolly, matching what users expect from a "click."
function installProportionalWheelZoom(domElement, cam, ctrls) {
    const SENSITIVITY = 0.001;       // dolly factor per unit of deltaY
    const MIN_DISTANCE = 0.05;
    const MAX_DISTANCE = 5000;
    const offset = new THREE.Vector3();

    domElement.addEventListener('wheel', (event) => {
        event.preventDefault();
        // Capture: true on the listener means we run before any
        // descendant listeners, but OrbitControls listens on the same
        // domElement, so we also call stopImmediatePropagation to
        // make sure its handler does NOT run on the same event.
        event.stopImmediatePropagation();

        // Exponential scaling so a deltaY of +100 dollies out 10.5%
        // (factor = e^0.1) and a deltaY of -100 dollies in 9.5%
        // (factor = e^-0.1). Sums correctly across many small events.
        const factor = Math.exp(event.deltaY * SENSITIVITY);

        offset.copy(cam.position).sub(ctrls.target).multiplyScalar(factor);
        const newDist = offset.length();
        if (newDist < MIN_DISTANCE || newDist > MAX_DISTANCE) return;
        cam.position.copy(ctrls.target).add(offset);
        ctrls.update();
    }, { passive: false, capture: true });
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
    renderer.toneMappingExposure = 1.3;

    // PBR materials need an environment map for proper indirect lighting.
    // Without one, leaves and other organic materials look flat and dark even
    // with strong direct lights. RoomEnvironment is a neutral default.
    pmremGenerator = new THREE.PMREMGenerator(renderer);
    pmremGenerator.compileEquirectangularShader();
    defaultEnvironment = pmremGenerator.fromScene(new RoomEnvironment(), 0.04).texture;
    scene.environment = defaultEnvironment;

    controls = new OrbitControls(camera, previewCanvas);
    controls.enableDamping = true;
    controls.dampingFactor = 0.1;
    // OrbitControls' built-in wheel handler is broken for trackpad
    // input: it treats every wheel event as one fixed-size dolly step
    // regardless of deltaY magnitude. macOS trackpad scrolls generate
    // 30+ events per gesture, each with a small deltaY, so a single
    // physical scroll compounds into a huge dolly (1.05^30 ≈ 4.7x).
    // We disable OrbitControls' zoom and install a custom handler
    // below that uses deltaY magnitude proportionally — many small
    // events sum to the same total zoom as one big event.
    controls.enableZoom = false;
    controls.minDistance = 0.05;
    controls.maxDistance = 5000;
    installProportionalWheelZoom(previewCanvas, camera, controls);

    scene.add(new THREE.AmbientLight(0xffffff, 0.4));
    const dirLight = new THREE.DirectionalLight(0xffffff, 1.5);
    dirLight.position.set(5, 10, 7);
    scene.add(dirLight);
    const dirLight2 = new THREE.DirectionalLight(0xffffff, 0.8);
    dirLight2.position.set(-5, 5, -5);
    scene.add(dirLight2);
    const dirLight3 = new THREE.DirectionalLight(0xffffff, 0.5);
    dirLight3.position.set(0, -3, 5);
    scene.add(dirLight3);
    const hemiLight = new THREE.HemisphereLight(0xffffff, 0x303040, 0.5);
    scene.add(hemiLight);

    scene.add(new THREE.GridHelper(10, 20, 0x2a2a4a, 0x1a1a3e));

    // T-006-02: optional ground plane. Created once, kept hidden by
    // default, toggled visible by the toolbar checkbox. Uses
    // MeshStandardMaterial so it picks up the live scene lighting
    // (consistency with the bake preset, S-007). 100×100 m covers
    // any practical template footprint.
    const groundGeom = new THREE.PlaneGeometry(100, 100);
    const groundMat = new THREE.MeshStandardMaterial({
        color: 0x6b5544, roughness: 0.95, metalness: 0,
    });
    groundPlane = new THREE.Mesh(groundGeom, groundMat);
    groundPlane.rotation.x = -Math.PI / 2;
    groundPlane.position.y = 0;
    groundPlane.visible = false;
    groundPlane.frustumCulled = false;
    scene.add(groundPlane);

    loader = new GLTFLoader();
    loader.setMeshoptDecoder(MeshoptDecoder);

    const ktx2Loader = new KTX2Loader();
    ktx2Loader.setTranscoderPath('https://unpkg.com/three@0.160.0/examples/jsm/libs/basis/');
    ktx2Loader.detectSupport(renderer);
    loader.setKTX2Loader(ktx2Loader);

    threeReady = true;
    // DEBUG: expose scene state for devtools poking. The module-scope
    // variables are not on window because this is a module script;
    // surface them here so console diagnostics are possible.
    window.__three = {
        get scene() { return scene; },
        get camera() { return camera; },
        get controls() { return controls; },
        get currentModel() { return currentModel; },
        get currentSettings() { return currentSettings; },
    };
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

    // T-009-03: in production-hybrid mode, the four arrays (billboard
    // side/top, tilted, volumetric) share one unified visibility pass.
    // Outside that mode, the legacy 2-state crossfades still drive the
    // standalone Billboard / Tilted / Volumetric preview buttons.
    if (stressActive && productionHybridFade) {
        if (billboardInstances.length > 0) updateBillboardFacing();
        if (tiltedBillboardInstances.length > 0) updateTiltedBillboardFacing();
        updateHybridVisibility();
    } else {
        // Billboard updates: camera-facing + overhead visibility swap
        if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
            updateBillboardFacing();
            updateBillboardVisibility();
        }
        // T-009-02: tilted-camera billboards have their own facing pass.
        // Gated separately so we don't run the side/top crossfade against
        // a tilted set, which has no top quad to fade to.
        if (stressActive && tiltedBillboardInstances.length > 0) {
            updateTiltedBillboardFacing();
        }
        // Volumetric slices: in hybrid mode, fade based on camera tilt angle
        if (stressActive && volumetricInstances.length > 0 && volumetricHybridFade) {
            updateVolumetricVisibility();
        }
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
    tiltedBillboardInstances = [];
    billboardTopInstances = [];
    volumetricInstances = [];
    volumetricHybridFade = false;
    productionHybridFade = false;
    stressActive = false;
    if (currentModel) currentModel.visible = true;
    document.getElementById('fpsOverlay').style.display = 'none';
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

// Returns a Promise that resolves once the model is loaded and added to
// the scene (T-004-04 callers — selectFile auto-reclassify path — need
// to wait for currentModel to be set before triggering an offscreen
// bake). Existing call sites ignore the return value, which is fine.
function loadModel(url, fileSize) {
    if (!threeReady) return Promise.resolve();
    clearStressInstances();
    if (currentModel) { scene.remove(currentModel); currentModel = null; }

    lastModelUrl = url;
    lastModelSize = fileSize;

    return new Promise((resolve, reject) => {
        loader.load(url, (gltf) => {
            currentModel = gltf.scene;
            // Boost env map intensity on all PBR materials so the scene environment
            // contributes strongly to indirect lighting. Many GLTF exports have this
            // set conservatively (~1.0); cranking it up brings out leaf greens.
            applyEnvironmentToModel(currentModel);
            scene.add(currentModel);
            frameCamera(currentModel);
            // Apply the asset's saved env_map_intensity (and any other
            // material-affecting tuning) on top of the env binding so
            // a freshly loaded model reflects the slider state.
            applyTuningToLiveScene();

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
            resolve();
        }, undefined, reject);
    });
}

// Walk a model and explicitly bind the current scene environment to PBR
// materials, with boosted envMapIntensity. Required because cloned/instanced
// materials don't always inherit scene.environment automatically, and many
// GLTF exports have a low default envMapIntensity.
//
// CRITICAL: Only touch MeshStandardMaterial / MeshPhysicalMaterial. On
// MeshBasicMaterial (used by billboard/volumetric quads), envMap is a
// reflection multiplier that BLENDS with the diffuse texture — setting it
// to anything makes the textured quad render dark or wrong.
function applyEnvironmentToModel(model) {
    const env = scene.environment;
    model.traverse((child) => {
        if (!child.isMesh || !child.material) return;
        const mats = Array.isArray(child.material) ? child.material : [child.material];
        for (const m of mats) {
            // Skip non-PBR materials (MeshBasicMaterial, MeshLambertMaterial, etc.)
            if (!m.isMeshStandardMaterial && !m.isMeshPhysicalMaterial) continue;
            m.envMap = env;
            m.envMapIntensity = 2.0;
            m.needsUpdate = true;
        }
    });
}

// ── Stress Test ──
// seededRandom gives deterministic "random" per index so stress test is reproducible
function seededRandom(i) {
    let x = Math.sin(i * 127.1 + 311.7) * 43758.5453;
    return x - Math.floor(x);
}

// ── Scene Templates (T-006-01) ──
// Pluggable layout templates for the stress test. Each template
// produces an array of InstanceSpec — { position, rotationY, scale } —
// which the placement helpers below consume. The legacy 100x grid is
// preserved as the `benchmark` template; `debug-scatter` validates
// the framework with 20 randomly-scattered instances.
//
// Note on AC deviation: spec uses scalar `rotationY` (radians) instead
// of a full Quaternion. Every existing helper only writes Y rotation
// (dummy.rotation.set(0, y, 0)) and our orientation rules are all
// Y-axis. Trivial to upgrade to Quaternion later if needed.

function makeInstanceSpec(position, rotationY = 0, scale = 1) {
    return { position, rotationY, scale };
}

// scatterRandomly: rejection-sampled random points within an XZ
// rectangle, with a min-distance constraint. Deterministic per seed.
// Returns Vector3[] (Y=0). Templates wrap into InstanceSpec.
function scatterRandomly(boundsXZ, count, seed = 0, minDistance = 0) {
    const points = [];
    const minSq = minDistance * minDistance;
    const w = boundsXZ.maxX - boundsXZ.minX;
    const d = boundsXZ.maxZ - boundsXZ.minZ;
    const budget = Math.max(count * 30, 30);
    let attempt = 0;
    while (points.length < count && attempt < budget) {
        const rx = seededRandom(seed * 1000 + attempt * 2);
        const rz = seededRandom(seed * 1000 + attempt * 2 + 1);
        attempt++;
        const x = boundsXZ.minX + rx * w;
        const z = boundsXZ.minZ + rz * d;
        let ok = true;
        if (minSq > 0) {
            for (const p of points) {
                const dx = p.x - x, dz = p.z - z;
                if (dx * dx + dz * dz < minSq) { ok = false; break; }
            }
        }
        if (ok) points.push(new THREE.Vector3(x, 0, z));
    }
    if (points.length < count) {
        console.warn(`scatterRandomly: only placed ${points.length}/${count} points (min-distance too large?)`);
    }
    return points;
}

// scatterInRow: linearly interpolated points between start and end
// with optional per-axis jitter. Used by row-planted templates
// (T-006-02). Returns Vector3[].
function scatterInRow(start, end, count, jitter = 0, seed = 0) {
    const points = [];
    for (let i = 0; i < count; i++) {
        const t = count <= 1 ? 0 : i / (count - 1);
        const x = start.x + (end.x - start.x) * t;
        const y = start.y + (end.y - start.y) * t;
        const z = start.z + (end.z - start.z) * t;
        const jx = (seededRandom(seed * 7 + i * 2) - 0.5) * jitter * 2;
        const jz = (seededRandom(seed * 7 + i * 2 + 1) - 0.5) * jitter * 2;
        points.push(new THREE.Vector3(x + jx, y, z + jz));
    }
    return points;
}

// applyVariation: per-instance scale + position jitter. Mutates and
// returns the spec array.
function applyVariation(specs, scaleRange, jitterAmount = 0, seed = 0) {
    const [smin, smax] = scaleRange;
    for (let i = 0; i < specs.length; i++) {
        const s = specs[i];
        const r = seededRandom(seed * 7919 + i);
        s.scale = smin + (smax - smin) * r;
        if (jitterAmount > 0) {
            const jx = (seededRandom(seed * 7919 + i + 1) - 0.5) * jitterAmount * 2;
            const jz = (seededRandom(seed * 7919 + i + 2) - 0.5) * jitterAmount * 2;
            s.position = s.position.clone();
            s.position.x += jx;
            s.position.z += jz;
        }
    }
    return specs;
}

// applyOrientationRule: stamp rotationY on each spec based on the
// per-shape orientation rule from STRATEGY_TABLE. `random-y` →
// uniform random, `fixed` → 0, `aligned-to-row` → 0 ± ~5°.
function applyOrientationRule(specs, rule, seed = 0) {
    for (let i = 0; i < specs.length; i++) {
        if (rule === 'random-y') {
            specs[i].rotationY = seededRandom(seed * 131 + i) * Math.PI * 2;
        } else if (rule === 'aligned-to-row') {
            // ±5° (~0.0873 rad) jitter — 0.175 rad peak-to-peak
            specs[i].rotationY = (seededRandom(seed * 131 + i) - 0.5) * 0.175;
        } else {
            // 'fixed' or unknown → no rotation
            specs[i].rotationY = 0;
        }
    }
    return specs;
}

// boundsFromSpecs: derive an XZ AABB from an InstanceSpec array.
// Used for camera framing fallback when a template doesn't return
// explicit bounds.
function boundsFromSpecs(specs) {
    if (!specs || specs.length === 0) {
        return { minX: 0, maxX: 0, minZ: 0, maxZ: 0, sizeX: 0, sizeZ: 0 };
    }
    let minX = Infinity, maxX = -Infinity, minZ = Infinity, maxZ = -Infinity;
    for (const s of specs) {
        if (s.position.x < minX) minX = s.position.x;
        if (s.position.x > maxX) maxX = s.position.x;
        if (s.position.z < minZ) minZ = s.position.z;
        if (s.position.z > maxZ) maxZ = s.position.z;
    }
    return { minX, maxX, minZ, maxZ, sizeX: maxX - minX, sizeZ: maxZ - minZ };
}

// SceneTemplate registry. Each entry: { id, name, generate(ctx, count) }.
// ctx = { bbox, shapeCategory, orientationRule, seed }.
// T-006-02: five designer-facing templates. The legacy `benchmark`
// template from T-006-01 lives on as `grid`; `debug-scatter` is
// removed (it was a framework smoke test, not user-facing).
function _ctxSize(ctx) {
    return (ctx.bbox && ctx.bbox.size)
        ? ctx.bbox.size
        : new THREE.Vector3(1, 1, 1);
}

const SCENE_TEMPLATES = {
    grid: {
        id: 'grid',
        name: 'Grid (benchmark)',
        generate(ctx, count) {
            const size = _ctxSize(ctx);
            const spacing = Math.max(size.x, size.z) * 1.3;
            const cols = Math.ceil(Math.sqrt(count));
            const rows = Math.ceil(count / cols);
            const gridW = cols * spacing;
            const gridD = rows * spacing;
            const specs = [];
            for (let i = 0; i < count; i++) {
                const r = Math.floor(i / cols);
                const c = i % cols;
                specs.push(makeInstanceSpec(
                    new THREE.Vector3(
                        c * spacing - gridW / 2 + spacing / 2,
                        0,
                        r * spacing - gridD / 2 + spacing / 2,
                    ),
                ));
            }
            applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
            return specs;
        },
    },
    'hedge-row': {
        id: 'hedge-row',
        name: 'Hedge Row',
        generate(ctx, count) {
            const size = _ctxSize(ctx);
            const spacing = Math.max(size.x, size.z) * 1.1;
            const half = (count - 1) * spacing / 2;
            const start = new THREE.Vector3(-half, 0, 0);
            const end = new THREE.Vector3(half, 0, 0);
            const positions = scatterInRow(start, end, count, 0, ctx.seed);
            const specs = positions.map(p => makeInstanceSpec(p));
            // Hedge rows always face the same way — overrides the
            // shape's orientation rule on purpose.
            applyOrientationRule(specs, 'fixed', ctx.seed);
            return specs;
        },
    },
    'mixed-bed': {
        id: 'mixed-bed',
        name: 'Mixed Bed',
        generate(ctx, count) {
            const size = _ctxSize(ctx);
            const spread = Math.max(size.x, size.z);
            const span = spread * Math.sqrt(count) * 1.4;
            const half = span / 2;
            const positions = scatterRandomly(
                { minX: -half, maxX: half, minZ: -half, maxZ: half },
                count, ctx.seed, spread * 0.9,
            );
            const specs = positions.map(p => makeInstanceSpec(p));
            applyVariation(specs, [0.85, 1.15], 0, ctx.seed);
            applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
            return specs;
        },
    },
    'rock-garden': {
        id: 'rock-garden',
        name: 'Rock Garden',
        generate(ctx, count) {
            const size = _ctxSize(ctx);
            const spread = Math.max(size.x, size.z);
            const span = spread * Math.sqrt(count) * 2.2;
            const half = span / 2;
            const positions = scatterRandomly(
                { minX: -half, maxX: half, minZ: -half, maxZ: half },
                count, ctx.seed, spread * 1.6,
            );
            const specs = positions.map(p => makeInstanceSpec(p));
            applyVariation(specs, [0.7, 1.3], 0, ctx.seed);
            applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
            return specs;
        },
    },
    container: {
        id: 'container',
        name: 'Container',
        generate(ctx, count) {
            // AC clamps the cluster to 5-10 instances. The slider /
            // input value is honored within that range; outside it,
            // we silently clamp.
            const n = Math.max(5, Math.min(10, count));
            const size = _ctxSize(ctx);
            const spread = Math.max(size.x, size.z);
            const half = spread * 1.2; // tight ~2.4× footprint
            const positions = scatterRandomly(
                { minX: -half, maxX: half, minZ: -half, maxZ: half },
                n, ctx.seed, spread * 0.7,
            );
            const specs = positions.map(p => makeInstanceSpec(p));
            applyVariation(specs, [0.9, 1.1], 0, ctx.seed);
            applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
            return specs;
        },
    },
};

let activeSceneTemplate = 'grid';

function setSceneTemplate(id) {
    if (!SCENE_TEMPLATES[id]) {
        console.warn(`setSceneTemplate: unknown template "${id}". Available:`, Object.keys(SCENE_TEMPLATES));
        return false;
    }
    activeSceneTemplate = id;
    console.info(`Scene template set to: ${id}`);
    return true;
}

function getActiveSceneTemplate() {
    return activeSceneTemplate;
}

// Expose hooks so devtools / future picker UI (T-006-02) can drive
// template selection without grepping the source.
window.setSceneTemplate = setSceneTemplate;
window.getActiveSceneTemplate = getActiveSceneTemplate;
window.__SCENE_TEMPLATES = SCENE_TEMPLATES;

// _isSpecArray: detect whether a placement helper received the new
// InstanceSpec[] format or the legacy Vector3[] format. Used to keep
// LOD/production code paths working unchanged on this ticket.
function _isSpecArray(arr) {
    return arr.length > 0 && arr[0] && arr[0].position && 'rotationY' in arr[0];
}

// Third arg accepts either Vector3[] (legacy LOD/production paths) or
// InstanceSpec[] (new T-006-01 template paths). When given specs,
// per-instance rotation+scale come from the spec; the legacy
// `randomRotateY` flag is ignored.
function createInstancedFromModel(model, count, arr, randomRotateY = false) {
    const meshes = [];
    model.traverse((child) => { if (child.isMesh) meshes.push(child); });

    const dummy = new THREE.Object3D();
    const modelInverse = new THREE.Matrix4().copy(model.matrixWorld).invert();
    const created = [];
    const isSpec = _isSpecArray(arr);
    const getPos = i => isSpec ? arr[i].position : arr[i];
    const getRotY = i => isSpec
        ? arr[i].rotationY
        : (randomRotateY ? seededRandom(i) * Math.PI * 2 : 0);
    const getScale = i => isSpec ? arr[i].scale : 1;

    for (const mesh of meshes) {
        const instancedMesh = new THREE.InstancedMesh(mesh.geometry, mesh.material, count);
        instancedMesh.frustumCulled = false;
        const localMatrix = new THREE.Matrix4().multiplyMatrices(modelInverse, mesh.matrixWorld.clone());

        for (let i = 0; i < count; i++) {
            dummy.position.copy(getPos(i));
            dummy.rotation.set(0, getRotY(i), 0);
            const s = getScale(i);
            dummy.scale.set(s, s, s);
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
// T-009-02: tilted-camera billboards. Side-only — the tilted bake has
// no `billboard_top` quad. Camera-facing in yaw, same math as
// `billboardInstances`; the tilt is baked into the texture, not the
// runtime transform.
let tiltedBillboardInstances = []; // { mesh: InstancedMesh, positions: Vector3[] }

// Second arg accepts either Vector3[] (legacy) or InstanceSpec[]
// (T-006-01). Spec form preserves per-instance rotation+scale through
// the variant bucketing.
function createBillboardInstances(model, arr) {
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

    const isSpec = _isSpecArray(arr);
    // Normalize to InstanceSpec[] internally so the per-variant
    // bucketing carries rotation/scale.
    const specs = isSpec
        ? arr
        : arr.map((p, i) => makeInstanceSpec(
            p,
            seededRandom(i + 5555) * Math.PI * 2,
            1,
        ));

    const numVariants = sideQuads.length;
    const variantSpecs = Array.from({ length: numVariants }, () => []);
    for (let i = 0; i < specs.length; i++) {
        const variant = Math.floor(seededRandom(i + 9999) * numVariants);
        variantSpecs[variant].push(specs[i]);
    }

    const created = [];
    const dummy = new THREE.Object3D();

    // Create side billboard instances (camera-facing)
    // geometry.translate() survives GLTF roundtrip — bottom is already at y=0, no offset needed
    for (let v = 0; v < numVariants; v++) {
        const bucket = variantSpecs[v];
        if (bucket.length === 0) continue;

        const quad = sideQuads[v];
        const geom = quad.geometry.clone();
        const mat = quad.material.clone();
        mat.depthWrite = true;
        mat.alphaTest = 0.5;
        mat.transparent = false;
        const instancedMesh = new THREE.InstancedMesh(geom, mat, bucket.length);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < bucket.length; i++) {
            dummy.position.copy(bucket[i].position);
            // Side billboards face the camera at render time — Y
            // rotation is overwritten by updateBillboardFacing(). We
            // still write scale here so size variation is preserved.
            const s = bucket[i].scale;
            dummy.scale.set(s, s, s);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, dummy.matrix);
        }
        instancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(instancedMesh);
        created.push(instancedMesh);
        billboardInstances.push({ mesh: instancedMesh, positions: bucket.map(s => s.position) });
    }

    // Create top-down instances (horizontal, for overhead view)
    if (topQuad) {
        const topGeom = topQuad.geometry.clone();
        const topMat = topQuad.material.clone();
        topMat.depthWrite = true;
        topMat.alphaTest = 0.5;
        topMat.transparent = false;
        const topInstancedMesh = new THREE.InstancedMesh(topGeom, topMat, specs.length);
        topInstancedMesh.frustumCulled = false;

        for (let i = 0; i < specs.length; i++) {
            dummy.position.copy(specs[i].position);
            dummy.rotation.set(0, specs[i].rotationY, 0);
            const s = specs[i].scale;
            dummy.scale.set(s, s, s);
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

// ── Volumetric Instances (Horizontal Layer Stacking) ──
// Each volumetric GLB contains N domed horizontal quads at specific heights.
// Instancing places copies at each position. In hybrid mode, slices fade in
// as the camera tilts toward top-down (>45°).
let volumetricInstances = []; // { mesh: InstancedMesh }[]
let volumetricHybridFade = false; // when true, fade based on camera angle
// T-009-03: when true, the four arrays (billboard side/top, tilted,
// volumetric) are driven by the unified updateHybridVisibility() in
// animate() instead of the legacy 2-state functions. Set only by
// runProductionStressTest; cleared in clearStressInstances.
let productionHybridFade = false;

// Second arg accepts either Vector3[] (legacy) or InstanceSpec[]
// (T-006-01).
function createVolumetricInstances(model, arr, hybridFade = false) {
    // Parse the volumetric GLB: quads named vol_layer_{i}_h{mm}
    const layerQuads = [];

    model.traverse((child) => {
        if (!child.isMesh) return;
        const match = child.name.match(/^vol_layer_(\d+)_h(-?\d+)$/);
        if (match) {
            layerQuads.push({
                layerIndex: parseInt(match[1]),
                baseY: parseInt(match[2]) / 1000,
                mesh: child,
            });
        }
    });

    layerQuads.sort((a, b) => a.layerIndex - b.layerIndex);
    if (layerQuads.length === 0) return [];

    if (hybridFade) volumetricHybridFade = true;

    const isSpec = _isSpecArray(arr);
    const getPos = i => isSpec ? arr[i].position : arr[i];
    const getRotY = i => isSpec
        ? arr[i].rotationY
        : seededRandom(i + 3333) * Math.PI * 2;
    const getScale = i => isSpec ? arr[i].scale : 1;
    const n = arr.length;

    const created = [];
    const dummy = new THREE.Object3D();

    for (const { mesh: quad, baseY } of layerQuads) {
        const geom = quad.geometry.clone();
        // CRITICAL: InstancedMesh ignores the source mesh's transform.
        // Bake the layer's Y offset into the geometry vertices so the
        // stack of slices keeps its vertical spacing under instancing.
        // Use the parsed baseY from the mesh name (survives GLTF roundtrip
        // even if the loaded mesh's position got flattened).
        geom.translate(0, baseY, 0);

        const mat = quad.material.clone();
        mat.depthWrite = true;
        mat.alphaTest = 0.15;
        mat.transparent = true;
        mat.side = THREE.DoubleSide;
        const instancedMesh = new THREE.InstancedMesh(geom, mat, n);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < n; i++) {
            const pos = getPos(i);
            dummy.position.set(pos.x, pos.y, pos.z);
            dummy.rotation.set(0, getRotY(i), 0);
            const s = getScale(i);
            dummy.scale.set(s, s, s);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, dummy.matrix);
        }
        instancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(instancedMesh);
        created.push(instancedMesh);
        volumetricInstances.push({ mesh: instancedMesh });
    }

    if (hybridFade) updateVolumetricVisibility();
    return created;
}

// Crossfade volumetric slices in/out based on camera angle.
// Looking horizontal: invisible. Looking down past 45°: visible.
// Mirror of the billboard side/top crossfade.
function updateVolumetricVisibility() {
    if (volumetricInstances.length === 0 || !volumetricHybridFade) return;

    const camDir = new THREE.Vector3();
    camera.getWorldDirection(camDir);
    const lookDownAmount = Math.abs(camDir.dot(new THREE.Vector3(0, -1, 0)));

    // 45° = 0.707 dot. Fade range centered there: 0.55 to 0.85
    const fadeStart = 0.55;
    const fadeEnd = 0.85;
    const t = THREE.MathUtils.clamp((lookDownAmount - fadeStart) / (fadeEnd - fadeStart), 0, 1);
    const opacity = t * t * (3 - 2 * t); // smoothstep

    for (const { mesh } of volumetricInstances) {
        mesh.visible = opacity > 0.01;
        if (mesh.visible) {
            mesh.material.opacity = opacity;
            mesh.material.transparent = true;
        }
    }
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

// T-009-03: Three-state production crossfade (horizontal → tilted →
// dome). Single visibility pass that reads `lookDownAmount` once and
// applies four opacities (billboard side, billboard top, tilted,
// volumetric dome). Replaces the legacy 2-state functions only when
// `productionHybridFade` is set; standalone preview modes still use
// `updateBillboardVisibility` / `updateVolumetricVisibility`.
//
// Math (see docs/active/work/T-009-03/design.md for the derivation):
//   sLow  = smoothstep((look - lowStart)  / (lowEnd  - lowStart))
//   sHigh = smoothstep((look - highStart) / (1 - highStart))
//   horizontal = 1 - sLow
//   tilted     = sLow * (1 - sHigh)
//   dome       = sHigh
//   side = horizontal * (1 - sLow); top = horizontal * sLow
function updateHybridVisibility() {
    const camDir = new THREE.Vector3();
    camera.getWorldDirection(camDir);
    const lookDown = Math.abs(camDir.dot(new THREE.Vector3(0, -1, 0)));

    const lowStart  = currentSettings ? currentSettings.tilted_fade_low_start  : 0.30;
    const lowEnd    = currentSettings ? currentSettings.tilted_fade_low_end    : 0.55;
    const highStart = currentSettings ? currentSettings.tilted_fade_high_start : 0.75;
    const highEnd   = 1.0;

    const lowDen  = Math.max(lowEnd  - lowStart,  1e-4);
    const highDen = Math.max(highEnd - highStart, 1e-4);
    const lowT  = THREE.MathUtils.clamp((lookDown - lowStart)  / lowDen,  0, 1);
    const highT = THREE.MathUtils.clamp((lookDown - highStart) / highDen, 0, 1);
    const sLow  = lowT  * lowT  * (3 - 2 * lowT);
    const sHigh = highT * highT * (3 - 2 * highT);

    const horizontalOpacity = 1 - sLow;
    const tiltedOpacity     = sLow * (1 - sHigh);
    const domeOpacity       = sHigh;
    const sideOpacity = horizontalOpacity * (1 - sLow);
    const topOpacity  = horizontalOpacity * sLow;

    applyOpacityToMeshes(billboardInstances,    sideOpacity);
    applyOpacityToMeshes(billboardTopInstances, topOpacity);
    applyOpacityToMeshes(tiltedBillboardInstances, tiltedOpacity);
    applyOpacityToMeshes(volumetricInstances,   domeOpacity);
}

function applyOpacityToMeshes(arr, opacity) {
    for (const { mesh } of arr) {
        mesh.visible = opacity > 0.01;
        if (mesh.visible) {
            mesh.material.opacity = opacity;
            mesh.material.transparent = opacity < 1;
        }
    }
}

// T-009-02: Runtime loader for the tilted-camera billboard bake from
// T-009-01. Mirrors `createBillboardInstances` but treats every mesh
// in the model as a side variant — the tilted bake has no
// `billboard_top` quad. Discriminated by file path (caller passes the
// gltf scene loaded from `?version=billboard-tilted`), not by quad
// name, since T-009-01 kept the legacy `billboard_${i}` naming.
function createTiltedBillboardInstances(model, arr) {
    const sideQuads = [];
    model.traverse((child) => { if (child.isMesh) sideQuads.push(child); });
    if (sideQuads.length === 0) return [];

    const isSpec = _isSpecArray(arr);
    const specs = isSpec
        ? arr
        : arr.map((p, i) => makeInstanceSpec(
            p,
            seededRandom(i + 5555) * Math.PI * 2,
            1,
        ));

    const numVariants = sideQuads.length;
    const variantSpecs = Array.from({ length: numVariants }, () => []);
    for (let i = 0; i < specs.length; i++) {
        // Distinct seed offset from the horizontal loader's `+9999` so
        // T-009-03 can run both at the same positions without their
        // variant assignments collapsing into a visible pattern.
        const variant = Math.floor(seededRandom(i + 7777) * numVariants);
        variantSpecs[variant].push(specs[i]);
    }

    const created = [];
    const dummy = new THREE.Object3D();

    for (let v = 0; v < numVariants; v++) {
        const bucket = variantSpecs[v];
        if (bucket.length === 0) continue;

        const quad = sideQuads[v];
        const geom = quad.geometry.clone();
        const mat = quad.material.clone();
        mat.depthWrite = true;
        mat.alphaTest = 0.5;
        mat.transparent = false;
        const instancedMesh = new THREE.InstancedMesh(geom, mat, bucket.length);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < bucket.length; i++) {
            dummy.position.copy(bucket[i].position);
            // Yaw is overwritten every frame by
            // updateTiltedBillboardFacing(); scale is preserved here
            // so size variation survives the first paint.
            const s = bucket[i].scale;
            dummy.scale.set(s, s, s);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, dummy.matrix);
        }
        instancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(instancedMesh);
        created.push(instancedMesh);
        tiltedBillboardInstances.push({
            mesh: instancedMesh,
            positions: bucket.map(s => s.position),
        });
    }

    // Face the camera on the first frame so the very first paint is
    // correct (mirrors the trailing updateBillboardFacing() call in
    // createBillboardInstances).
    updateTiltedBillboardFacing();
    return created;
}

// T-009-02: per-frame yaw update for tilted billboard instances.
// Identical math to updateBillboardFacing — yaw only, tilt is baked
// into the texture. Kept as a separate function (not parameterised
// over the state array) so the two paths can diverge cleanly when
// T-009-03 layers a crossfade on top.
function updateTiltedBillboardFacing() {
    if (tiltedBillboardInstances.length === 0) return;
    const camPos = camera.position;
    const dummy = new THREE.Object3D();

    for (const { mesh, positions } of tiltedBillboardInstances) {
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

function runStressTest(count, useLods, quality = 0.5) {
    if (!currentModel) return;
    clearStressInstances();
    stressActive = true;
    currentModel.visible = false;

    // Use original model bbox for consistent spacing regardless of which version is displayed
    const refBBox = originalModelBBox || modelBBox;
    const fallbackSize = new THREE.Box3().setFromObject(currentModel).getSize(new THREE.Vector3());
    const ctxBBox = refBBox || { size: fallbackSize };

    // T-006-01: build placements via the active SceneTemplate. The
    // legacy 100x grid lives on as the `grid` template (T-006-02).
    const tpl = SCENE_TEMPLATES[activeSceneTemplate] || SCENE_TEMPLATES.grid;
    const cat = currentSettings && currentSettings.shape_category;
    const ctx = {
        bbox: ctxBBox,
        shapeCategory: cat,
        orientationRule: (STRATEGY_TABLE[cat] && STRATEGY_TABLE[cat].instance_orientation_rule) || 'random-y',
        seed: 0,
    };
    const specs = tpl.generate(ctx, count);
    const effectiveCount = specs.length;
    const positions = specs.map(s => s.position);
    const bounds = boundsFromSpecs(specs);
    // Pad bounds by one footprint so the camera doesn't clip the
    // outermost instances; matches the implicit padding the legacy
    // grid math got from `spacing/2` centering.
    const sizeXZ = ctxBBox.size ? Math.max(ctxBBox.size.x, ctxBBox.size.z) : 1;
    const gridWidth = bounds.sizeX + sizeXZ;
    const gridDepth = bounds.sizeZ + sizeXZ;

    if (!useLods) {
        if (previewVersion === 'billboard') {
            // Billboard mode: always face camera
            const instances = createBillboardInstances(currentModel, specs);
            stressInstances.push(...instances);
        } else if (previewVersion === 'volumetric') {
            // Volumetric mode: horizontal dome slices (no fade — always visible)
            const instances = createVolumetricInstances(currentModel, specs, false);
            stressInstances.push(...instances);
        } else if (previewVersion === 'production') {
            // Production hybrid: billboard for low angles + volumetric for top-down,
            // with crossfade at ~45° camera tilt. Loads both GLBs in parallel.
            // T-006-01: still uses Vector3[] (no per-instance variation
            // on this path; revisited in T-006-02).
            runProductionStressTest(positions);
        } else {
            // Regular mode: per-instance rotation+scale come from the
            // template's InstanceSpec; the legacy randomRotateY flag is
            // ignored when the helper detects spec input.
            const instances = createInstancedFromModel(currentModel, effectiveCount, specs, false);
            stressInstances.push(...instances);
        }
    } else {
        // LOD mode: load LOD models and assign by distance from center.
        // T-006-01: still uses Vector3[] (LOD path migration is
        // T-006-02 territory).
        runLodStressTest(effectiveCount, positions, gridWidth, gridDepth, quality);
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
    document.getElementById('instanceCount').textContent = effectiveCount.toLocaleString();
    document.getElementById('totalTris').textContent = (modelTriCount * effectiveCount).toLocaleString();
    document.getElementById('estMemory').textContent = formatBytes(lastModelSize * effectiveCount);
}

// Production hybrid: load billboard + volumetric models in parallel and create
// both instance sets at the same positions. The runtime fade in
// updateBillboardVisibility / updateVolumetricVisibility crossfades them based
// on camera angle (~45° threshold).
async function runProductionStressTest(positions) {
    const file = files.find(f => f.id === selectedFileId);
    // T-009-03: production hybrid now requires all three impostor layers.
    if (!file || !file.has_billboard || !file.has_billboard_tilted || !file.has_volumetric) return;

    try {
        const [billboardGltf, tiltedGltf, volumetricGltf] = await Promise.all([
            new Promise((resolve, reject) => loader.load(
                `/api/preview/${selectedFileId}?version=billboard&t=${Date.now()}`,
                resolve, undefined, reject)),
            new Promise((resolve, reject) => loader.load(
                `/api/preview/${selectedFileId}?version=billboard-tilted&t=${Date.now()}`,
                resolve, undefined, reject)),
            new Promise((resolve, reject) => loader.load(
                `/api/preview/${selectedFileId}?version=volumetric&t=${Date.now()}`,
                resolve, undefined, reject)),
        ]);

        applyEnvironmentToModel(billboardGltf.scene);
        applyEnvironmentToModel(tiltedGltf.scene);
        applyEnvironmentToModel(volumetricGltf.scene);

        // Horizontal billboard instances (side + top quads, fade out as
        // the camera tilts past the low band).
        const bbInstances = createBillboardInstances(billboardGltf.scene, positions);
        stressInstances.push(...bbInstances);

        // Tilted billboard instances (elevated-camera impostor, peak
        // visibility in the ~45° band between the low and high fades).
        const tiltedInstances = createTiltedBillboardInstances(tiltedGltf.scene, positions);
        stressInstances.push(...tiltedInstances);

        // Volumetric instances (horizontal dome slices, fade in across
        // the high band as the camera approaches straight down).
        const volInstances = createVolumetricInstances(volumetricGltf.scene, positions, true);
        stressInstances.push(...volInstances);

        // T-009-03: hand control of all four arrays to the unified
        // updateHybridVisibility() in animate().
        productionHybridFade = true;
    } catch (err) {
        console.error('Production stress test failed:', err);
    }
}

async function runLodStressTest(count, positions, gridWidth, gridDepth, quality = 0.5) {
    const file = files.find(f => f.id === selectedFileId);
    const randomRotate = shouldRandomRotateInstances();
    if (!file || !file.lods || file.lods.length === 0) {
        const instances = createInstancedFromModel(currentModel, count, positions, randomRotate);
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
    const t3 = t2 + 0.1 + (1 - q) * 0.1;  // LOD3 boundary
    const t4 = t3 + 0.05 + (1 - q) * 0.05; // volumetric boundary, rest = billboard

    const maxDist = Math.max(gridWidth, gridDepth) / 2;

    // Use volumetric LODs if available, otherwise mesh LODs
    const hasVlods = file.volumetric_lods && file.volumetric_lods.length > 0 && file.volumetric_lods[0].size > 0;

    let buckets, lodVersions;
    if (hasVlods) {
        buckets = { vlod0: [], vlod1: [], vlod2: [], vlod3: [] };
        for (let i = 0; i < positions.length; i++) {
            const dist = positions[i].length();
            const ratio = dist / (maxDist || 1);
            if (ratio < t0) buckets.vlod0.push(positions[i]);
            else if (ratio < t1) buckets.vlod1.push(positions[i]);
            else if (ratio < t2) buckets.vlod2.push(positions[i]);
            else buckets.vlod3.push(positions[i]);
        }
        lodVersions = ['vlod0', 'vlod1', 'vlod2', 'vlod3'];
    } else {
        buckets = { lod0: [], lod1: [], lod2: [], lod3: [], volumetric: [], billboard: [] };
        for (let i = 0; i < positions.length; i++) {
            const dist = positions[i].length();
            const ratio = dist / (maxDist || 1);
            if (ratio < t0) buckets.lod0.push(positions[i]);
            else if (ratio < t1) buckets.lod1.push(positions[i]);
            else if (ratio < t2) buckets.lod2.push(positions[i]);
            else if (ratio < t3) buckets.lod3.push(positions[i]);
            else if (ratio < t4 && file.has_volumetric) buckets.volumetric.push(positions[i]);
            else if (file.has_billboard) buckets.billboard.push(positions[i]);
            else buckets.lod3.push(positions[i]);
        }
        lodVersions = ['lod0', 'lod1', 'lod2', 'lod3', 'volumetric', 'billboard'];
    }

    let totalTris = 0;
    let totalMem = 0;

    for (const version of lodVersions) {
        const bucket = buckets[version];
        if (bucket.length === 0) continue;

        if (version === 'billboard' && !file.has_billboard) continue;
        if (version === 'volumetric' && !file.has_volumetric) continue;
        if (version.startsWith('vlod')) {
            const lodIdx = parseInt(version[4]);
            if (!file.volumetric_lods?.[lodIdx] || file.volumetric_lods[lodIdx].size === 0) continue;
        } else if (version.startsWith('lod')) {
            const lodIdx = parseInt(version[3]);
            if (!file.lods[lodIdx] || file.lods[lodIdx].error) continue;
        }

        try {
            const url = `/api/preview/${selectedFileId}?version=${version}&t=${Date.now()}`;
            const gltf = await new Promise((resolve, reject) => {
                loader.load(url, resolve, undefined, reject);
            });

            const model = gltf.scene;
            applyEnvironmentToModel(model);
            model.updateMatrixWorld(true);
            const stats = getModelStats(model);

            let instances;
            if (version === 'billboard') {
                // Billboard: camera-facing instances with variant selection
                instances = createBillboardInstances(model, bucket);
            } else if (version === 'volumetric' || version.startsWith('vlod')) {
                // Volumetric: layered billboard impostor (camera-facing + depth layers)
                instances = createVolumetricInstances(model, bucket);
            } else {
                // Mesh LOD: random Y rotation for visual variety,
                // unless the strategy router pinned orientation (T-004-05).
                instances = createInstancedFromModel(model, bucket.length, bucket, randomRotate);
            }
            stressInstances.push(...instances);

            totalTris += stats.triangles * bucket.length;
            if (version === 'billboard' || version === 'volumetric') {
                totalMem += 50000 * bucket.length;
            } else if (version.startsWith('vlod')) {
                const lodIdx = parseInt(version[4]);
                totalMem += (file.volumetric_lods?.[lodIdx]?.size || 0) * bucket.length;
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

// ── First-run hint + inline help (T-008-03) ──
//
// paintHelpText runs once at startup. It walks every
// [data-help-id] inside the right-panel tuning section and appends
// a `<div class="tooltip">` (the existing inline-italic style used
// elsewhere in the form) carrying the matching string from
// help_text.js. Skips rows already painted so calling it twice is
// a no-op.
function paintHelpText() {
    const rows = document.querySelectorAll('[data-help-id]');
    for (const row of rows) {
        if (row.querySelector(':scope > .tooltip[data-help-paint]')) continue;
        const key = row.getAttribute('data-help-id');
        const text = HELP_TEXT[key];
        if (!text) continue;
        const tip = document.createElement('div');
        tip.className = 'tooltip';
        tip.setAttribute('data-help-paint', '1');
        tip.textContent = text;
        row.appendChild(tip);
    }
}

// updatePlaceholderState picks between the first-run "Get started"
// hint and the plain "select or drop a file" fallback. Called from
// renderFileList() (so add/delete update it) and once at startup
// after refreshFiles() resolves.
function updatePlaceholderState() {
    const hintEl = document.getElementById('firstRunHint');
    const fallbackEl = document.getElementById('placeholderFallback');
    if (!hintEl || !fallbackEl) return;
    if (files.length > 0) firstRunHintDismissed = true;
    const showHint = !firstRunHintDismissed && files.length === 0;
    hintEl.style.display = showHint ? 'block' : 'none';
    fallbackEl.style.display = showHint ? 'none' : 'block';
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
    // End the previous analytics session before mutating state, so the
    // session_end goes to the *previous* asset's JSONL with the click's
    // own timestamp, not after the new session_start.
    if (analyticsSessionId && analyticsAssetId !== id) {
        endAnalyticsSession('switched');
    }
    selectedFileId = id;
    previewVersion = 'original';
    renderFileList();
    updatePreviewButtons();
    updateProfileButtons();
    showPreview();
    const file = files.find(f => f.id === id);
    if (file) {
        // Reset to default environment + neutral lights. T-005-03: the
        // calibration decision now happens AFTER loadSettings so it can
        // honor the per-asset color_calibration_mode rather than auto-
        // applying whenever a reference image happens to exist.
        if (referenceEnvironment) { referenceEnvironment.dispose(); referenceEnvironment = null; }
        referencePalette = null;
        scene.environment = defaultEnvironment;
        resetSceneLights();
        loadSettings(id).then(async () => {
            if (
                currentSettings &&
                currentSettings.lighting_preset === 'from-reference-image' &&
                file.has_reference
            ) {
                await loadReferenceEnvironment(id);
            }
            await startAnalyticsSession(id);
            populateTuningUI();
            populateScenePreviewUI();
            populateAcceptedUI(id);
            // T-007-02: apply the asset's saved preset to the live
            // scene and reset the stale-bake flag for the new asset.
            // applyTuningToLiveScene then layers the per-slider
            // intensities on top so the live preview reflects the
            // asset's saved tuning the moment it loads.
            applyPresetToLiveScene();
            applyTuningToLiveScene();
            setBakeStale(false);
            const loadPromise = loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
            // T-004-04: low-confidence assets auto-open the comparison
            // modal once the model has loaded (the modal's thumbnail
            // pipeline needs currentModel to bake against).
            // shape_confidence === 1.0 is the human-confirmed sentinel
            // and skips the prompt; shape_confidence === 0 means
            // never-classified, also skipped (nothing to compare).
            const conf = currentSettings && currentSettings.shape_confidence;
            if (conf > 0 && conf < COMPARISON_AUTO_THRESHOLD) {
                Promise.resolve(loadPromise).then(async () => {
                    if (selectedFileId !== id) return; // user already moved on
                    try {
                        const { settings, candidates } = await fetchClassification(id);
                        if (selectedFileId !== id) return;
                        currentSettings = settings;
                        populateTuningUI();
                        if (candidates && candidates.length > 0) {
                            await openComparisonModal(
                                id,
                                candidates,
                                settings.shape_category,
                                settings.shape_confidence,
                            );
                        }
                    } catch (err) {
                        console.warn('auto-reclassify failed:', err);
                    }
                });
            }
        });
    }
}

// T-005-03 / T-007-03: hide/show the in-panel "Upload reference image"
// row based on whether the active lighting preset is the
// `from-reference-image` calibration preset.
function syncReferenceImageRow() {
    const row = document.getElementById('referenceImageRow');
    if (!row || !currentSettings) return;
    row.style.display =
        currentSettings.lighting_preset === 'from-reference-image'
            ? '' : 'none';
}

// T-005-03: apply or tear down reference-image color calibration to
// match the current settings. Idempotent. Used when the user toggles
// the dropdown after selection (the selection-time path inlines the
// same gate inside selectFile to avoid an extra model reload).
function applyColorCalibration(id) {
    if (!id || !currentSettings) return;
    const file = files.find(f => f.id === id);
    const wantCalibration =
        currentSettings.lighting_preset === 'from-reference-image' &&
        file && file.has_reference;
    if (wantCalibration) {
        loadReferenceEnvironment(id).then(() => {
            if (currentModel) {
                const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
                loadModel(url, lastModelSize);
            }
        });
    } else {
        if (referenceEnvironment) { referenceEnvironment.dispose(); referenceEnvironment = null; }
        referencePalette = null;
        scene.environment = defaultEnvironment;
        // T-007-02: when calibration is torn down, fall back to the
        // active preset rather than raw neutral white.
        applyPresetToLiveScene();
        applyTuningToLiveScene();
        if (currentModel) {
            const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
            loadModel(url, lastModelSize);
        }
    }
}

// Restore the neutral default lighting (called before loading a reference image
// or when switching to a file without one).
function resetSceneLights() {
    const white = new THREE.Color(0xffffff);
    const groundDefault = new THREE.Color(0x303040);
    scene.traverse((obj) => {
        if (obj.isHemisphereLight) {
            obj.color.copy(white);
            obj.groundColor.copy(groundDefault);
            obj.intensity = 0.5;
        } else if (obj.isAmbientLight) {
            obj.color.copy(white);
            obj.intensity = 0.4;
        }
    });
}

function updatePreviewButtons() {
    const file = files.find(f => f.id === selectedFileId);
    btnOriginal.classList.toggle('active', previewVersion === 'original');
    btnOptimized.classList.toggle('active', previewVersion === 'optimized');
    btnOptimized.disabled = !(file && file.status === 'done');

    // LOD buttons
    const hasLods = file && file.lods && file.lods.length > 0;
    const hasVlods = file && file.volumetric_lods && file.volumetric_lods.length > 0;
    lodToggle.style.display = hasLods || hasVlods || (file && (file.has_billboard || file.has_billboard_tilted || file.has_volumetric)) ? 'flex' : 'none';

    lodToggle.querySelectorAll('button').forEach(btn => {
        const lod = btn.dataset.lod;
        btn.classList.toggle('active', previewVersion === lod);
        if (lod === 'billboard') {
            btn.disabled = !(file && file.has_billboard);
        } else if (lod === 'billboard-tilted') {
            btn.disabled = !(file && file.has_billboard_tilted);
        } else if (lod === 'volumetric') {
            btn.disabled = !(file && file.has_volumetric);
        } else if (lod === 'production') {
            btn.disabled = !(file && file.has_billboard && file.has_volumetric);
        } else if (lod.startsWith('vlod')) {
            const idx = parseInt(lod[4]);
            btn.disabled = !(hasVlods && file.volumetric_lods[idx] && file.volumetric_lods[idx].size > 0);
        } else if (lod.startsWith('lod')) {
            const idx = parseInt(lod[3]);
            btn.disabled = !(hasLods && file.lods[idx] && !file.lods[idx].error);
        }
    });

    // Generate buttons: enabled when file exists
    generateLodsBtn.disabled = !file;
    generateBlenderLodsBtn.disabled = !file;
    generateBillboardBtn.disabled = !file || !currentModel;
    generateVolumetricBtn.disabled = !file || !currentModel;
    generateVolumetricLodsBtn.disabled = !file || !currentModel;
    generateProductionBtn.disabled = !file || !currentModel;
    // T-010-03: pack build needs the side intermediate plus at least
    // one of the optional layers (tilted or dome). Mirrors the
    // ticket's acceptance criterion verbatim.
    buildPackBtn.disabled = !(file && file.has_billboard
                              && (file.has_billboard_tilted || file.has_volumetric));
    prepareForSceneBtn.disabled = !file || !currentModel;
    testLightingBtn.disabled = !file || !currentModel;
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
        if (version === 'billboard' || version === 'billboard-tilted' || version === 'volumetric' || version === 'production') {
            fileSize = 50000; // estimate
        } else if (version.startsWith('vlod')) {
            const idx = parseInt(version[4]);
            fileSize = file.volumetric_lods?.[idx]?.size || 0;
        } else {
            const idx = parseInt(version[3]);
            fileSize = file.lods?.[idx]?.size || 0;
        }
        // Production preview: trigger a 1-instance stress test through the
        // hybrid path so the user sees the full three-phase crossfade
        // (side billboards → tilted billboards → volumetric dome) on a
        // single instance, with the original mesh hidden. Without this,
        // clicking "Production" used to load the static volumetric mesh
        // and leave the base model visible — see runStressTest at the
        // top of this file for the path it takes.
        if (version === 'production') {
            runStressTest(1, false);
            return;
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

generateVolumetricBtn.addEventListener('click', () => {
    if (selectedFileId) generateVolumetric(selectedFileId);
});

generateVolumetricLodsBtn.addEventListener('click', () => {
    if (selectedFileId) generateVolumetricLODs(selectedFileId);
});

generateProductionBtn.addEventListener('click', () => {
    if (selectedFileId) generateProductionAsset(selectedFileId);
});

buildPackBtn.addEventListener('click', () => {
    if (selectedFileId) buildAssetPack(selectedFileId);
});

// T-008-01: Prepare-for-scene primary action + post-run "View in scene"
// affordance. The view-in-scene button just delegates to the existing
// stress-test "Run scene" button so it picks up the asset's current
// scene template, instance count, and ground/LOD toggles unchanged.
prepareForSceneBtn.addEventListener('click', () => {
    if (selectedFileId) prepareForScene(selectedFileId);
});

viewInSceneBtn.addEventListener('click', () => {
    const stressBtnEl = document.getElementById('stressBtn');
    if (stressBtnEl) stressBtnEl.click();
});

// T-008-02: toolbar #uploadReferenceBtn removed. The hidden
// #referenceFileInput is now triggered only by the in-tuning-panel
// button (see wireTuningUI → #tuneReferenceImageBtn).
referenceFileInput.addEventListener('change', () => {
    if (referenceFileInput.files.length > 0 && selectedFileId) {
        uploadReferenceImage(selectedFileId, referenceFileInput.files[0]);
        referenceFileInput.value = '';
    }
});

testLightingBtn.addEventListener('click', testLighting);
window.testLighting = testLighting; // also accessible from console

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

// Scene preview controls (T-006-02)
const stressBtn = document.getElementById('stressBtn');
const stressUseLods = document.getElementById('stressUseLods');
const lodQualitySlider = document.getElementById('lodQuality');
const lodQualityValue = document.getElementById('lodQualityValue');
const lodQualityLabel = document.getElementById('lodQualityLabel');

lodQualitySlider.addEventListener('input', () => { lodQualityValue.textContent = lodQualitySlider.value + '%'; });

// Show/hide quality slider when LOD checkbox changes
stressUseLods.addEventListener('change', () => {
    const show = stressUseLods.checked;
    lodQualitySlider.style.display = show ? '' : 'none';
    lodQualityValue.style.display = show ? '' : 'none';
    lodQualityLabel.style.display = show ? '' : 'none';
});

// Picker change: update active template, persist, emit analytics.
sceneTemplateSelect.addEventListener('change', () => {
    const from = getActiveSceneTemplate();
    const to = sceneTemplateSelect.value;
    if (from === to) return;
    setSceneTemplate(to);
    if (currentSettings) {
        currentSettings.scene_template_id = to;
        if (selectedFileId) saveSettings(selectedFileId);
    }
    logEvent('scene_template_selected', {
        from,
        to,
        instance_count: parseInt(sceneInstanceCount.value, 10) || 0,
        ground_plane: sceneGroundToggle.checked,
    }, selectedFileId);
});

// Count input change: clamp, persist. No analytics emission — count
// rides along inside scene_template_selected, and the resting value
// is captured in session_end's final_settings.
sceneInstanceCount.addEventListener('change', () => {
    const n = Math.max(1, Math.min(500, parseInt(sceneInstanceCount.value, 10) || 1));
    sceneInstanceCount.value = n;
    if (currentSettings) {
        currentSettings.scene_instance_count = n;
        if (selectedFileId) saveSettings(selectedFileId);
    }
});

// Ground plane toggle: flip mesh visibility, persist.
sceneGroundToggle.addEventListener('change', () => {
    const on = sceneGroundToggle.checked;
    if (groundPlane) groundPlane.visible = on;
    if (currentSettings) {
        currentSettings.scene_ground_plane = on;
        if (selectedFileId) saveSettings(selectedFileId);
    }
});

stressBtn.addEventListener('click', () => {
    const count = parseInt(sceneInstanceCount.value, 10) || 1;
    if (count <= 1) {
        clearStressInstances();
        if (currentModel) { currentModel.position.set(0, 0, 0); frameCamera(currentModel); }
    } else {
        const quality = parseInt(lodQualitySlider.value, 10) / 100;
        runStressTest(count, stressUseLods.checked, quality);
    }
});

// ── Init ──
console.log('GLB Optimizer frontend loaded');
initThreeJS();
// T-008-03: paint inline help text into tuning rows + render the
// first-run hint state. paintHelpText is idempotent and pure DOM;
// updatePlaceholderState runs again from renderFileList() once
// refreshFiles() resolves with the actual file list.
paintHelpText();
updatePlaceholderState();
refreshFiles();
populateScenePreviewSelect();
applyDefaults();
populateLightingPresetSelect();
wireTuningUI();
populateTuningUI();

// Profiles UI wiring (T-003-03)
profileSelect.addEventListener('change', updateProfileButtons);
profileApplyBtn.addEventListener('click', applySelectedProfile);
profileDeleteBtn.addEventListener('click', deleteSelectedProfile);
profileSaveOpenBtn.addEventListener('click', openSaveProfileForm);
profileSaveCancelBtn.addEventListener('click', closeSaveProfileForm);
profileSaveSubmitBtn.addEventListener('click', submitSaveProfile);
profileNameInput.addEventListener('input', () => {
    const v = profileNameInput.value;
    const ok = v === '' || (PROFILE_NAME_RE.test(v) && v.length <= 64);
    profileNameInput.classList.toggle('invalid', !ok);
    if (ok) profileNameError.textContent = '';
});
loadProfileList();

// Accepted UI wiring (T-003-04)
acceptBtn.addEventListener('click', markAccepted);

// Tab close → flush a session_end via sendBeacon. fetch() is cancelled by
// the browser during unload; sendBeacon is queued and reliable.
window.addEventListener('beforeunload', () => {
    if (analyticsSessionId) endAnalyticsSessionBeacon('closed');
});

// Check server capabilities (Blender availability)
fetch('/api/status').then(r => r.json()).then(status => {
    if (status.blender && status.blender.available) {
        blenderAvailable = true;
        generateBlenderLodsBtn.style.display = '';
        console.log(`Blender ${status.blender.version} available for remesh LODs`);
    }
}).catch(() => {});
