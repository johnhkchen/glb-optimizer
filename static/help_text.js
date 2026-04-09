// One-line help strings for the asset-tuning panel (T-008-03).
//
// Each key is the DOM id of the control the row contains. app.js
// reads this map at startup, walks every [data-help-id] in
// index.html, and inserts a <div class="tooltip"> below the row's
// label. Edit the prose here — the markup follows automatically.
//
// Style guide:
//   - One sentence per entry. Keep it short.
//   - Describe what the control changes in user-visible terms
//     ("makes the dome look taller"), not pipeline jargon
//     ("scales the Y axis of the slice generator").
//   - No recommended values; the defaults already encode those.
//   - Plain text only — no HTML, no markdown.

export const HELP_TEXT = {
    tuneVolumetricLayers:
        "How many horizontal slices fake the 3D volume. More layers look fuller but cost more to bake.",
    tuneVolumetricResolution:
        "Pixel size of each baked slice texture. Higher is sharper but slower to bake and heavier in memory.",
    tuneDomeHeightFactor:
        "How tall the dome of slices reaches above the asset. Lower it for flat shrubs, raise it for tall trees.",
    tuneBakeExposure:
        "Overall brightness of the baked textures. Nudge up if the result looks muddy, down if it looks washed out.",
    tuneAmbientIntensity:
        "Flat fill light from every direction. Lifts shadows without adding any sense of direction.",
    tuneHemisphereIntensity:
        "Soft sky-vs-ground gradient light. Adds outdoor ambience and hints at where the sky is.",
    tuneKeyLightIntensity:
        "Strength of the main directional sun. Higher gives sharper highlights and harder shadows.",
    tuneBottomFillIntensity:
        "Bounce light coming from below. Use it to keep the underside of foliage from going pitch black.",
    tuneEnvMapIntensity:
        "How much the environment is reflected on shiny surfaces. Lower it if leaves look glassy.",
    tuneAlphaTest:
        "Cutoff for treating semi-transparent pixels as fully transparent. Raise it to clean up halo edges around leaves.",
    tuneTiltedFadeLowStart:
        "Camera tilt where the tilted impostor begins to fade in over the horizontal billboard. Tune by eye if the handoff looks abrupt.",
    tuneTiltedFadeLowEnd:
        "Camera tilt where the horizontal billboard is fully gone and the tilted impostor takes over. Must be greater than the fade-in start.",
    tuneTiltedFadeHighStart:
        "Camera tilt where the tilted impostor begins to fade out into the top-down dome slices. Slides toward 1.0 to delay the dome takeover.",
    tuneLightingPreset:
        "Picks an opinionated bundle of light colors and intensities. Switching presets resets the related sliders.",
    tuneSliceDistributionMode:
        "How the slice heights are spaced through the asset. Visual-density bunches them where there's the most stuff to see.",
    tuneGroundAlign:
        "Forces the lowest slice to sit exactly on the ground plane so the asset doesn't float.",
    tuneReferenceImageBtn:
        "Upload a photo to color-match the bake against. Only used when the lighting preset is set to from-reference-image.",
    tuneReclassifyBtn:
        "Re-run shape classification and pick from a few candidate strategies side-by-side.",
    tuneResetBtn:
        "Restore every tuning value above to its default for this asset.",
};
