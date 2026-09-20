# weather-v2 - animated weather widget (Evey plugin)

Rewrite of `examples/plugins/weather` fixing its three field problems:

| v1 problem | v2 fix |
|---|---|
| OOM / memory growth (1.3 MB three.js+R3F bundle, 4 THREE scenes + FBO, per-frame allocations, particle geometry re-created without dispose, 3000-particle CPU->GPU upload every frame) | Zero-dependency raw WebGL: 1 context, 1 program, 1 VBO, 1 texture, no framebuffers, allocation-free render loop, full teardown on pagehide + WEBGL_lose_context. Bundle: 52 KB. |
| Not responsive (fixed 280px height, fixed 44px font, dpr=1) | Height negotiated with the host per width (216/248/280), clamp() typography on container-query units (vw fallback), DPR-aware backing store with quality-tier cap, ResizeObserver. |
| Sun effect glitches (cloud occlusion via hard-coded -0.32 screen offset, hour-of-day sun position without latitude/season + visualAlt hacks) | Sun/moon driven by REAL sunrise/sunset from Open-Meteo (`timeformat=unixtime`, yesterday..tomorrow); per-pixel cloud occlusion computed in the same shader pass, so it can never desync. |

## Architecture (DDD, dependencies point inward)

```
src/
  domain/            pure TS, zero deps, fully unit-tested
    model/           weather.ts (snapshot, sun times), scene.ts (params, palette)
    services/        conditionToScene (WMO table), dayCycle (real solar events),
                     paletteMixer (time-of-day x mood), moonPhase
    ports/           WeatherProvider, HostBridge, SceneRenderer
  application/       WeatherApp: orchestration, refresh (15 min) + day-cycle
                     tick (5 s, live sun/moon motion without any page reload),
                     bounded retries, lifecycle/dispose
  adapters/          openmeteo/ (HTTP, 10 s timeout), evey/ (window.evey bridge,
                     standalone fallback), render/ (raw WebGL renderer + shader)
  ui/                vanilla-DOM overlay (no React)
  i18n/              labels en/fr/es/de
```

## Assets

Both celestial bodies are real NASA photos (public domain), bundled as small
JPEGs (disc masked in-shader, no alpha needed):
- moon: NASA/GSFC FullMoon2010 (via Wikimedia Commons), 320px, 17 KB
- sun: 2 real NASA/SDO full-disc images (credit: NASA/SDO and the AIA science
  team), one per daily rotation skin, cropped tight so the disc fills the
  frame (like the moon), 384px each: AIA 171 (gold) and AIA 193 (bronze). A
  different one shows each UTC day (see `domain/services/sunModel.ts`); the
  shader keeps each skin's own tones by day and warms everything toward orange
  at the horizon. Disc radius is set by `SR` in the shader (~2x the moon).

  These images are snapshots bundled at build time (offline-safe, no network,
  no CORS). Fetching the *live* "latest" SDO image per day is not possible from
  the sandboxed iframe: NASA serves no CORS header (WebGL texture upload would
  taint), and the plugin CSP restricts `img-src` to the host origin. Live
  fetch would require a host-side proxy endpoint in Evey core - see the note
  at the bottom.

## Rendering

Single-pass fragment shader compositing (straight alpha over transparent
canvas, host page shows through): golden-hour wash -> moon (photo texture,
phase terminator) -> 2 FBM cloud layers (rim-lit toward the light source)
-> thunder flash -> fog -> GPU rain/snow -> sun (disk + corona + rays,
occluded per pixel by the clouds above it). ~30 fps cap, pauses when hidden
or scrolled out of view, honors prefers-reduced-motion, survives WebGL
context loss.

## Build and packaging

`./build-zip.sh` does everything: installs deps, typechecks, runs the 27
domain tests, builds dist/index.js (~52 KB, moon.jpg inlined), prunes
node_modules to the backend runtime deps (@grpc/*) and produces
`../weather-v2.zip`.

The zip is tailored to pass the server's static analyzer: it ships only
plugin.yaml, main.js, dist/, vendored node_modules and a metadata-only
package.json (this README, sources, tests and dev manifests are excluded -
see the comments in build-zip.sh for the exact analyzer rules involved).
Verified against the real analyzer: safe=true, no critical finding, only the
expected informational warning about outbound fetch() (covered by the
`network` permission + `external_fetch` whitelist).

## Version history

Moved out of the manifest description in 2.15.1: that field is what someone
reads when deciding whether to install the plugin, not a changelog.

- **2.15.0** - on a television the widget asks for a banner instead of a card, because a TV row is as wide as a desktop page while the panel has a fraction of the vertical room and the card height hid the library below the fold.
- **2.14.0** - live SDO frames are re-cropped to the bundled skin's tight geometry (limb at 0.93) so the shader's disc mask lines up and the edge stops looking nibbled.
- **2.13.0** - dark SDO frames are now rejected server-side (jpeg-js decode) before being persisted; the frontend pixel gate is blinded by Firefox resistFingerprinting.
- **2.12.0** - dark SDO frames (eclipse/calibration) are rejected on decoded brightness, so the sun never renders black.
- **2.11.0** - the sun-image fetch now goes through the host Fetch broker (no direct plugin network access).
- **2.10.0** - disambiguated city picker (region + country, stores coordinates) so homonyms like Brest FR/BY resolve correctly.
