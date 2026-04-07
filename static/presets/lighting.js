// Lighting preset registry (T-007-01).
//
// A preset is a named, opinionated bundle of light intensities, light
// colors, env-gradient stops, and tone exposure. Picking a preset
// rewrites the dependent AssetSettings fields (intensities + bake
// exposure) and supplies the colors used by the bake pipeline.
//
// Schema (per preset):
//   {
//     id: string,
//     name: string,           // display name
//     description: string,    // one-line summary
//     bake_config: {
//       ambient: number,                          // → ambient_intensity
//       hemisphere_intensity: number,             // → hemisphere_intensity
//       hemisphere_sky:    [r,g,b],               // 0..1 linear-ish
//       hemisphere_ground: [r,g,b],
//       key_intensity: number,                    // → key_light_intensity
//       key_color:     [r,g,b],
//       fill_intensity: number,                   // → bottom_fill_intensity
//       fill_color:     [r,g,b],
//       env_gradient:   [[r,g,b], [r,g,b], [r,g,b]], // top, mid, bottom
//       env_intensity: number,                    // → env_map_intensity
//       tone_exposure: number,                    // → bake_exposure
//     },
//     preview_config: { /* identical shape; T-007-02 wires this */ }
//   }
//
// `bake_config` and `preview_config` have the same shape so T-007-02
// can wire one or both into the live preview without further schema
// changes. They are seeded equal here.

// makePreset deep-clones bake_config into preview_config so the two
// are independently mutable but start out identical.
function makePreset({ id, name, description, bake_config }) {
    const clone = (v) => Array.isArray(v) ? v.map(clone)
                       : v && typeof v === 'object' ? Object.fromEntries(Object.entries(v).map(([k,x]) => [k, clone(x)]))
                       : v;
    return Object.freeze({
        id,
        name,
        description,
        bake_config: Object.freeze(clone(bake_config)),
        preview_config: Object.freeze(clone(bake_config)),
    });
}

// Insertion order matters — listLightingPresets() preserves it for the
// dropdown UI.
export const LIGHTING_PRESETS = Object.freeze({
    'default': makePreset({
        id: 'default',
        name: 'Default',
        description: 'Neutral baseline (matches built-in defaults)',
        bake_config: {
            ambient: 0.50,
            hemisphere_intensity: 1.00,
            hemisphere_sky:    [1.00, 1.00, 1.00],
            hemisphere_ground: [0.27, 0.27, 0.27],
            key_intensity: 1.40,
            key_color:     [1.00, 1.00, 1.00],
            fill_intensity: 0.40,
            fill_color:     [1.00, 1.00, 1.00],
            env_gradient:   [[1.00,1.00,1.00], [0.85,0.85,0.85], [0.45,0.45,0.45]],
            env_intensity: 1.20,
            tone_exposure: 1.00,
        },
    }),
    'midday-sun': makePreset({
        id: 'midday-sun',
        name: 'Midday Sun',
        description: 'Bright neutral white sun, strong key, mild exposure',
        bake_config: {
            ambient: 0.45,
            hemisphere_intensity: 1.10,
            hemisphere_sky:    [0.95, 0.97, 1.00],
            hemisphere_ground: [0.30, 0.27, 0.22],
            key_intensity: 1.80,
            key_color:     [1.00, 0.99, 0.95],
            fill_intensity: 0.35,
            fill_color:     [0.85, 0.88, 1.00],
            env_gradient:   [[1.00,1.00,1.00], [0.92,0.94,1.00], [0.55,0.55,0.60]],
            env_intensity: 1.20,
            tone_exposure: 1.00,
        },
    }),
    'overcast': makePreset({
        id: 'overcast',
        name: 'Overcast',
        description: 'Soft, low-contrast, slightly cool',
        bake_config: {
            ambient: 1.10,
            hemisphere_intensity: 1.40,
            hemisphere_sky:    [0.90, 0.93, 0.98],
            hemisphere_ground: [0.55, 0.58, 0.62],
            key_intensity: 0.60,
            key_color:     [0.92, 0.94, 0.98],
            fill_intensity: 0.60,
            fill_color:     [0.85, 0.88, 0.92],
            env_gradient:   [[0.90,0.93,0.98], [0.78,0.81,0.86], [0.55,0.58,0.62]],
            env_intensity: 1.10,
            tone_exposure: 1.00,
        },
    }),
    'golden-hour': makePreset({
        id: 'golden-hour',
        name: 'Golden Hour',
        description: 'Warm key, low ambient, dramatic',
        bake_config: {
            ambient: 0.30,
            hemisphere_intensity: 0.80,
            hemisphere_sky:    [1.00, 0.85, 0.55],
            hemisphere_ground: [0.20, 0.13, 0.08],
            key_intensity: 1.60,
            key_color:     [1.00, 0.78, 0.45],
            fill_intensity: 0.30,
            fill_color:     [0.65, 0.55, 0.40],
            env_gradient:   [[1.00,0.85,0.55], [0.80,0.55,0.30], [0.30,0.18,0.10]],
            env_intensity: 1.30,
            tone_exposure: 1.10,
        },
    }),
    'dusk': makePreset({
        id: 'dusk',
        name: 'Dusk',
        description: 'Cool blue ambient, low key intensity',
        bake_config: {
            ambient: 0.55,
            hemisphere_intensity: 0.90,
            hemisphere_sky:    [0.55, 0.65, 1.00],
            hemisphere_ground: [0.10, 0.12, 0.20],
            key_intensity: 0.40,
            key_color:     [0.65, 0.75, 1.00],
            fill_intensity: 0.30,
            fill_color:     [0.45, 0.55, 0.85],
            env_gradient:   [[0.55,0.65,1.00], [0.30,0.38,0.65], [0.08,0.10,0.18]],
            env_intensity: 1.00,
            tone_exposure: 0.85,
        },
    }),
    'indoor': makePreset({
        id: 'indoor',
        name: 'Indoor',
        description: 'Soft fill, no strong key',
        bake_config: {
            ambient: 0.85,
            hemisphere_intensity: 1.20,
            hemisphere_sky:    [0.95, 0.93, 0.88],
            hemisphere_ground: [0.45, 0.42, 0.38],
            key_intensity: 0.50,
            key_color:     [1.00, 0.96, 0.88],
            fill_intensity: 0.80,
            fill_color:     [0.95, 0.93, 0.90],
            env_gradient:   [[0.95,0.93,0.88], [0.78,0.75,0.70], [0.45,0.42,0.38]],
            env_intensity: 0.90,
            tone_exposure: 0.95,
        },
    }),
});

// O(1) lookup. Returns undefined for unknown ids — callers MUST handle
// undefined (corrupt settings file, hand-edit) by falling back to
// LIGHTING_PRESETS.default.
export function getLightingPreset(id) {
    return LIGHTING_PRESETS[id];
}

// Ordered [{id, name, description}] list for dropdown population.
// Insertion order is preserved.
export function listLightingPresets() {
    return Object.values(LIGHTING_PRESETS).map(p => ({
        id: p.id,
        name: p.name,
        description: p.description,
    }));
}

// Mapping from preset bake_config keys to AssetSettings field names.
// Used by the cascade application in app.js to know which sliders to
// rewrite when a preset is picked.
export const PRESET_FIELD_MAP = Object.freeze({
    ambient:              'ambient_intensity',
    hemisphere_intensity: 'hemisphere_intensity',
    key_intensity:        'key_light_intensity',
    fill_intensity:       'bottom_fill_intensity',
    env_intensity:        'env_map_intensity',
    tone_exposure:        'bake_exposure',
});

// Dev-time sanity check: the 'default' preset's mapped intensities
// must equal the hardcoded defaults in app.js (makeDefaults()), so
// that picking 'default' on a fresh asset is a no-op. We can't import
// makeDefaults from here without creating a cycle, so the matching
// constants are duplicated and asserted at module load. If the real
// defaults drift, this fires a console.warn — does not throw, to
// avoid breaking the page on a typo.
(function assertDefaultMatchesAppDefaults() {
    const expected = {
        ambient_intensity:     0.50,
        hemisphere_intensity:  1.00,
        key_light_intensity:   1.40,
        bottom_fill_intensity: 0.40,
        env_map_intensity:     1.20,
        bake_exposure:         1.00,
    };
    const def = LIGHTING_PRESETS['default'].bake_config;
    const actual = {
        ambient_intensity:     def.ambient,
        hemisphere_intensity:  def.hemisphere_intensity,
        key_light_intensity:   def.key_intensity,
        bottom_fill_intensity: def.fill_intensity,
        env_map_intensity:     def.env_intensity,
        bake_exposure:         def.tone_exposure,
    };
    for (const k of Object.keys(expected)) {
        if (expected[k] !== actual[k]) {
            console.warn(
                `[lighting presets] 'default' preset drifted from app defaults: ` +
                `${k} expected ${expected[k]} got ${actual[k]}`
            );
        }
    }
})();
