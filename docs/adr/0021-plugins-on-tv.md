# ADR 0021 - Plugins on TV: WebView portage now, native SDUI renderer later

Date: 2026-07-09
Status: Accepted
Scope: phase 1 implemented, phase 2 planned

## Context

Evey plugins ship a sandboxed FRONTEND (iframe on web, WebView on mobile)
talking to the host through a tiny postMessage bridge (`window.evey`:
resize, navigate, toast, prefs per profile, media APIs and asset reads
proxied by the PARENT with a scoped plugin token the plugin never sees).
The mobile app already carries a hardened 1:1 portage of the web protocol
(`apps/mobile/src/features/plugins/PluginSlot.tsx`: disk-cached bundles,
three-stage lazy arming, hardware layer for WebGL, navigation blocking,
path-traversal guards) - "plugins built for web run unchanged on mobile".

The TV app needs plugin surfaces too. Requirements from the product owner:
simple, easily portable, good performance on weak Android TV hardware.

Options considered:

1. **WebView portage** (copy the mobile component). Maximal simplicity and
   portability - existing plugins run unchanged, the protocol is already
   proven on two platforms. Perf mitigations already exist (lazy arming,
   bundle cache, hardware layer); the flagship plugins were rewritten
   zero-dependency (~50-90 KB WebGL bundles) precisely for weak clients.
   Limits: no WebKit on tvOS (Apple TV builds get no plugins this way),
   and a WebView does not participate in D-pad focus - slots are
   display-only on TV.
2. **Native SDUI renderer**. The sanitized server-driven UI schema
   (`apps/web/src/features/sdui/schema.ts`) was explicitly designed to be
   consumed by React Native renderers: pure data, allow-listed node types,
   anti-DoS budgets. Perfect D-pad focus, perfect perf, works on tvOS. But
   no RN renderer exists yet, no plugin emits SDUI documents yet (no
   manifest surface / endpoint), and it breaks "web plugins run unchanged".
3. **Hybrid** - WebView for today's visual widgets, SDUI as the growth path
   for interactive/tvOS surfaces.

## Decision

Hybrid, phased:

- **Phase 1 (done)**: port the mobile `PluginSlot` + `pluginBridge` to
  `apps/tv/src/features/plugins/`, mounted on `home.section` under the home
  hero, WebView `focusable={false}` (display-only, never traps the D-pad),
  with the same lazy-arming/caching pattern and the same security guards
  (`pluginIdShape`, asset path traversal checks).
- **Phase 2 (later)**: move the SDUI schema into a shared `packages/`
  module, write an RN renderer with native D-pad focus, and add a plugin
  manifest surface for SDUI documents - the path for interactive TV
  widgets and for tvOS, where WebView does not exist. Plugins may then
  declare a perf hint (e.g. `heavy: webgl`) so weak TV profiles skip
  arming 3D plugins entirely.

## Consequences

- Every existing plugin (weather-v2, cosmos, marquee) appears on TV with
  zero plugin-side changes.
- Apple TV builds show no plugin slots until phase 2.
- TV plugin slots are ambient/display-only by design for now; interaction
  stays on web/mobile until the SDUI renderer lands.
