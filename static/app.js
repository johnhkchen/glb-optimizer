import * as THREE from 'three';
import { GLTFLoader } from 'three/addons/loaders/GLTFLoader.js';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import { KTX2Loader } from 'three/addons/loaders/KTX2Loader.js';
import { MeshoptDecoder } from 'three/addons/libs/meshopt_decoder.module.js';
import { GLTFExporter } from 'three/addons/exporters/GLTFExporter.js';
import { RoomEnvironment } from 'three/addons/environments/RoomEnvironment.js';

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
let currentSettings = null; // per-asset bake/tuning settings, populated by selectFile()
let _saveSettingsTimer = null; // debounce handle for saveSettings()

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
const generateVolumetricBtn = document.getElementById('generateVolumetricBtn');
const generateVolumetricLodsBtn = document.getElementById('generateVolumetricLodsBtn');
const generateProductionBtn = document.getElementById('generateProductionBtn');
const uploadReferenceBtn = document.getElementById('uploadReferenceBtn');
const referenceFileInput = document.getElementById('referenceFileInput');
const testLightingBtn = document.getElementById('testLightingBtn');
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
    } catch (err) {
        console.warn(`loadSettings(${id}) failed, using defaults:`, err);
        applyDefaults();
    }
    return currentSettings;
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
        lighting_preset: 'default',
        slice_distribution_mode: 'visual-density',
        ground_align: true,
        color_calibration_mode: 'none',
        reference_image_path: '',
    };
}

function applyDefaults() {
    currentSettings = makeDefaults();
    return currentSettings;
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
    { field: 'lighting_preset',       id: 'tuneLightingPreset',       parse: v => v,               fmt: v => v },
    // T-005-01: enrolled for setting_changed analytics. DOM ids are
    // reserved here so the auto-instrumentation in wireTuningUI picks
    // them up the moment T-005-02 lands the controls; populate/wire
    // both short-circuit harmlessly when the elements are absent.
    { field: 'slice_distribution_mode', id: 'tuneSliceDistributionMode', parse: v => v,             fmt: v => v },
    { field: 'ground_align',            id: 'tuneGroundAlign',           parse: v => v === true || v === 'true',
                                                                          fmt: v => String(v) },
    // T-005-03: color calibration source enum (none / from-reference-image).
    // The full preset enum lands in S-007.
    { field: 'color_calibration_mode',  id: 'tuneColorCalibrationMode',  parse: v => v,             fmt: v => v },
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
            currentSettings[spec.field] = v;
            const valEl = document.getElementById(spec.id + 'Value');
            if (valEl) valEl.textContent = spec.fmt(v);
            updateTuningDirty();
            saveSettings(selectedFileId);
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
            // T-005-03: when the calibration mode flips, the in-panel
            // upload row visibility and the live scene need to follow.
            if (spec.field === 'color_calibration_mode') {
                syncReferenceImageRow();
                applyColorCalibration(selectedFileId);
            }
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
            applyDefaults();
            populateTuningUI();
            saveSettings(selectedFileId);
            // T-005-03: defaults set mode back to "none", so any active
            // calibration on the live scene needs to be torn down.
            applyColorCalibration(selectedFileId);
        });
    }
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
    } catch (err) { console.error('Billboard generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'billboard', success }, id);
    }

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
    offRenderer.toneMappingExposure = currentSettings.bake_exposure;

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

// Add bake lights to an offscreen scene. Tinted by the reference palette if
// one is loaded. The offscreen scene also gets its own PMREM env (see
// createBakeEnvironment) so PBR materials have IBL — these direct lights
// supply the highlights and shadows on top.
// Omnidirectional bake lighting — rotationally symmetric around the Y axis so
// every billboard angle gets the same illumination. No side-biased directional
// lights. Palette colors are used as TINTS (near-white normalized colors), so
// light intensities can stay high without darkening.
function setupBakeLights(offScene) {
    const sky = referencePalette
        ? new THREE.Color(referencePalette.bright.r, referencePalette.bright.g, referencePalette.bright.b)
        : new THREE.Color(0xffffff);
    const fill = referencePalette
        ? new THREE.Color(referencePalette.mid.r, referencePalette.mid.g, referencePalette.mid.b)
        : new THREE.Color(0xffffff);
    const ground = referencePalette
        ? new THREE.Color(referencePalette.dark.r, referencePalette.dark.g, referencePalette.dark.b)
        : new THREE.Color(0x444444);

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

// Build a PMREM environment map ON THE GIVEN RENDERER. This is the key fix
// for the bake — we can't reuse the main renderer's PMREM texture in an
// offscreen context, so each offscreen render generates its own env using
// its own PMREMGenerator. Tinted by the reference palette if loaded.
function createBakeEnvironment(renderer) {
    const pmrem = new THREE.PMREMGenerator(renderer);
    pmrem.compileEquirectangularShader();

    let env;
    if (referencePalette) {
        // Same gradient trick as the live preview env: bright→mid→dark
        const w = 256, h = 128;
        const cv = document.createElement('canvas');
        cv.width = w; cv.height = h;
        const ctx = cv.getContext('2d');
        const grad = ctx.createLinearGradient(0, 0, 0, h);
        const c = (col) => `rgb(${Math.round(col.r * 255)},${Math.round(col.g * 255)},${Math.round(col.b * 255)})`;
        grad.addColorStop(0, c(referencePalette.bright));
        grad.addColorStop(0.5, c(referencePalette.mid));
        grad.addColorStop(1, c(referencePalette.dark));
        ctx.fillStyle = grad;
        ctx.fillRect(0, 0, w, h);

        const tex = new THREE.CanvasTexture(cv);
        tex.mapping = THREE.EquirectangularReflectionMapping;
        tex.colorSpace = THREE.SRGBColorSpace;
        tex.needsUpdate = true;
        env = pmrem.fromEquirectangular(tex).texture;
        tex.dispose();
    } else {
        // Neutral default: RoomEnvironment, generated on this renderer
        env = pmrem.fromScene(new RoomEnvironment(), 0.04).texture;
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

// ── Volumetric Distillation (Horizontal Layer Peeling) ──
// Slices the model into horizontal bands and renders each from above with
// everything above that band clipped away. The result is stacked horizontal
// quads at their respective heights — a "top-down MRI" of the plant.
// Note: layer count and resolution are now read from currentSettings
// (volumetric_layers / volumetric_resolution). T-002-01 made these
// per-asset and T-002-02 wired them in.

async function generateVolumetric(id) {
    if (!currentModel || !threeReady) return;

    generateVolumetricBtn.textContent = 'Rendering...';
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
    } catch (err) { console.error('Volumetric generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'volumetric', success }, id);
    }

    generateVolumetricBtn.textContent = 'Volumetric';
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

    // Tinted by reference palette if loaded. Palette colors are normalized
    // tints near 1.0, so we can use bright intensities without darkening.
    const sky = referencePalette
        ? new THREE.Color(referencePalette.bright.r, referencePalette.bright.g, referencePalette.bright.b)
        : new THREE.Color(0xffffff);
    const fill = referencePalette
        ? new THREE.Color(referencePalette.mid.r, referencePalette.mid.g, referencePalette.mid.b)
        : new THREE.Color(0xffffff);
    const ground = referencePalette
        ? new THREE.Color(referencePalette.dark.r, referencePalette.dark.g, referencePalette.dark.b)
        : new THREE.Color(0x444444);
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

function renderHorizontalLayerGLB(model, numLayers, resolution) {
    return new Promise((resolve, reject) => {
        const exportScene = new THREE.Scene();

        // Adaptive: pick layer count by aspect ratio, then dispatch on
        // the per-asset slice_distribution_mode (T-005-01). The default
        // case falls through to the legacy vertex-quantile picker so a
        // missing/stale setting degrades to the prior behavior, never
        // a crash.
        const actualLayers = pickAdaptiveLayerCount(model, numLayers);
        const mode = currentSettings.slice_distribution_mode;
        let boundaries;
        switch (mode) {
            case 'equal-height':
                boundaries = computeEqualHeightBoundaries(model, actualLayers);
                break;
            case 'visual-density':
                boundaries = computeVisualDensityBoundaries(model, actualLayers);
                break;
            case 'vertex-quantile':
            default:
                boundaries = computeAdaptiveSliceBoundaries(model, actualLayers);
                break;
        }

        for (let i = 0; i < actualLayers; i++) {
            const floorY = boundaries[i];
            const ceilingY = boundaries[i + 1];
            const layerThickness = Math.max(ceilingY - floorY, 0.001);

            const { canvas, quadSize } = renderLayerTopDown(model, resolution, floorY, ceilingY);

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

    generateVolumetricLodsBtn.textContent = 'Generating...';
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
    } catch (err) { console.error('Volumetric LOD generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'volumetric_lods', success }, id);
    }

    generateVolumetricLodsBtn.textContent = 'Vol LODs';
    generateVolumetricLodsBtn.classList.remove('generating');
    generateVolumetricLodsBtn.disabled = false;
}

// ── Production Asset (Hybrid) ──
// One-click generation: billboard for ~horizontal views, volumetric (horizontal
// dome slices) for top-down views, with crossfade at ~45° in the runtime preview.
async function generateProductionAsset(id) {
    if (!currentModel || !threeReady) return;

    generateProductionBtn.textContent = 'Rendering...';
    generateProductionBtn.classList.add('generating');
    generateProductionBtn.disabled = true;

    let success = false;
    try {
        // Billboard pass (multi-angle camera-facing impostor)
        const billboardGlb = await renderMultiAngleBillboardGLB(currentModel, BILLBOARD_ANGLES);
        await fetch(`/api/upload-billboard/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: billboardGlb,
        });
        store_update(id, f => f.has_billboard = true);

        // Volumetric pass (horizontal dome slices, top-down)
        const volumetricGlb = await renderHorizontalLayerGLB(currentModel, currentSettings.volumetric_layers, currentSettings.volumetric_resolution);
        await fetch(`/api/upload-volumetric/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/octet-stream' },
            body: volumetricGlb,
        });
        store_update(id, f => f.has_volumetric = true);

        await refreshFiles();
        updatePreviewButtons();
        success = true;
    } catch (err) { console.error('Production asset generation failed:', err); }
    finally {
        logEvent('regenerate', { trigger: 'production', success }, id);
    }

    generateProductionBtn.textContent = 'Production Asset';
    generateProductionBtn.classList.remove('generating');
    generateProductionBtn.disabled = false;
}

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
        if (currentSettings && currentSettings.color_calibration_mode === 'from-reference-image') {
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

// Tint the scene's ambient + hemisphere lights with the reference palette so
// direct lighting also picks up the calibrated colors.
function applyReferenceTint(palette) {
    const sky = new THREE.Color(palette.bright.r, palette.bright.g, palette.bright.b);
    const ground = new THREE.Color(palette.mid.r, palette.mid.g, palette.mid.b);
    scene.traverse((obj) => {
        if (obj.isHemisphereLight) {
            obj.color.copy(sky);
            obj.groundColor.copy(ground);
            obj.intensity = 1.0;
        } else if (obj.isAmbientLight) {
            obj.color.copy(sky);
            obj.intensity = 0.6;
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
    // Volumetric slices: in hybrid mode, fade based on camera tilt angle
    if (stressActive && volumetricInstances.length > 0 && volumetricHybridFade) {
        updateVolumetricVisibility();
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
    volumetricInstances = [];
    volumetricHybridFade = false;
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
        // Boost env map intensity on all PBR materials so the scene environment
        // contributes strongly to indirect lighting. Many GLTF exports have this
        // set conservatively (~1.0); cranking it up brings out leaf greens.
        applyEnvironmentToModel(currentModel);
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

// ── Volumetric Instances (Horizontal Layer Stacking) ──
// Each volumetric GLB contains N domed horizontal quads at specific heights.
// Instancing places copies at each position. In hybrid mode, slices fade in
// as the camera tilts toward top-down (>45°).
let volumetricInstances = []; // { mesh: InstancedMesh }[]
let volumetricHybridFade = false; // when true, fade based on camera angle

function createVolumetricInstances(model, positions, hybridFade = false) {
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
        const instancedMesh = new THREE.InstancedMesh(geom, mat, positions.length);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < positions.length; i++) {
            const pos = positions[i];
            dummy.position.set(pos.x, pos.y, pos.z);
            dummy.rotation.set(0, seededRandom(i + 3333) * Math.PI * 2, 0);
            dummy.scale.set(1, 1, 1);
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
        } else if (previewVersion === 'volumetric') {
            // Volumetric mode: horizontal dome slices (no fade — always visible)
            const instances = createVolumetricInstances(currentModel, positions, false);
            stressInstances.push(...instances);
        } else if (previewVersion === 'production') {
            // Production hybrid: billboard for low angles + volumetric for top-down,
            // with crossfade at ~45° camera tilt. Loads both GLBs in parallel.
            runProductionStressTest(positions);
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

// Production hybrid: load billboard + volumetric models in parallel and create
// both instance sets at the same positions. The runtime fade in
// updateBillboardVisibility / updateVolumetricVisibility crossfades them based
// on camera angle (~45° threshold).
async function runProductionStressTest(positions) {
    const file = files.find(f => f.id === selectedFileId);
    if (!file || !file.has_billboard || !file.has_volumetric) return;

    try {
        const [billboardGltf, volumetricGltf] = await Promise.all([
            new Promise((resolve, reject) => loader.load(
                `/api/preview/${selectedFileId}?version=billboard&t=${Date.now()}`,
                resolve, undefined, reject)),
            new Promise((resolve, reject) => loader.load(
                `/api/preview/${selectedFileId}?version=volumetric&t=${Date.now()}`,
                resolve, undefined, reject)),
        ]);

        applyEnvironmentToModel(billboardGltf.scene);
        applyEnvironmentToModel(volumetricGltf.scene);

        // Billboard instances (camera-facing, fade out when looking down)
        const bbInstances = createBillboardInstances(billboardGltf.scene, positions);
        stressInstances.push(...bbInstances);

        // Volumetric instances (horizontal dome slices, fade in when looking down)
        const volInstances = createVolumetricInstances(volumetricGltf.scene, positions, true);
        stressInstances.push(...volInstances);
    } catch (err) {
        console.error('Production stress test failed:', err);
    }
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
                // Mesh LOD: random Y rotation for visual variety
                instances = createInstancedFromModel(model, bucket.length, bucket, true);
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
                currentSettings.color_calibration_mode === 'from-reference-image' &&
                file.has_reference
            ) {
                await loadReferenceEnvironment(id);
            }
            await startAnalyticsSession(id);
            populateTuningUI();
            populateAcceptedUI(id);
            loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
        });
    }
}

// T-005-03: hide/show the in-panel "Upload reference image" row based
// on the current color_calibration_mode.
function syncReferenceImageRow() {
    const row = document.getElementById('referenceImageRow');
    if (!row || !currentSettings) return;
    row.style.display =
        currentSettings.color_calibration_mode === 'from-reference-image'
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
        currentSettings.color_calibration_mode === 'from-reference-image' &&
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
        resetSceneLights();
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
    lodToggle.style.display = hasLods || hasVlods || (file && (file.has_billboard || file.has_volumetric)) ? 'flex' : 'none';

    lodToggle.querySelectorAll('button').forEach(btn => {
        const lod = btn.dataset.lod;
        btn.classList.toggle('active', previewVersion === lod);
        if (lod === 'billboard') {
            btn.disabled = !(file && file.has_billboard);
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
    uploadReferenceBtn.disabled = !file;
    testLightingBtn.disabled = !file || !currentModel;
    if (file && file.has_reference) {
        uploadReferenceBtn.textContent = 'Reference ✓';
        uploadReferenceBtn.title = 'Reference image loaded — click to replace';
    } else {
        uploadReferenceBtn.textContent = 'Reference Image';
        uploadReferenceBtn.title = 'Upload a reference image to calibrate scene lighting';
    }
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
        if (version === 'billboard' || version === 'volumetric' || version === 'production') {
            fileSize = 50000; // estimate
        } else if (version.startsWith('vlod')) {
            const idx = parseInt(version[4]);
            fileSize = file.volumetric_lods?.[idx]?.size || 0;
        } else {
            const idx = parseInt(version[3]);
            fileSize = file.lods?.[idx]?.size || 0;
        }
        // Production preview shows the volumetric model (the static slices) since
        // the hybrid behavior only kicks in during stress test instancing
        const previewVer = version === 'production' ? 'volumetric' : version;
        loadModel(`/api/preview/${selectedFileId}?version=${previewVer}&t=${Date.now()}`, fileSize);
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

uploadReferenceBtn.addEventListener('click', () => {
    if (selectedFileId) referenceFileInput.click();
});

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
applyDefaults();
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
