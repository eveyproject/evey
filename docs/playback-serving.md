# Serving a video - the HLS playback path

This is the **read path**: what happens when a player asks for a video, from the
first `master.m3u8` request down to the bytes of a segment. It is the counterpart
to the scan pipeline in [ARCHITECTURE.md](ARCHITECTURE.md) (the write path that
primes the read model), and the deep-dive behind
[CODEBASE_MAP.md](CODEBASE_MAP.md) "Golden path 4".

The playback path is **VITAL** and spread across ~8 files in the `http` adapter.
There is no single function that runs the whole flow - this document is the map.

## The mental model: 3 questions, 2 engines, 1 key

Serving a video reduces to answering three questions, then packaging the answer,
with everything addressed by one hash:

- **Q1 - Can I read the file cheaply?** Probe dispatch, by **file extension**:
  `.mkv/.webm` and `.mp4/.mov` go to **mkvgo** (head-only, no process); everything
  else, and any fast-path failure, goes to **ffprobe** (the authority).
- **Q2 - Can the client play it as-is?** The playback **decision**, taken *per
  track*: video `copy` vs `transcode`, audio `copy` vs `transcode` vs the wasm
  AC-3 path. Video and audio are decided **independently** - the common case is
  "copy the video, re-encode the audio to AAC".
- **Q3 - How much can this host encode?** The calibrated **capability** (from the
  measurement taken at the first quiet moment) plus **admission** (which ladder
  rung fits, is there a slot).

Two engines run through all of it:

- **mkvgo** - reads and repackages (remux / on-demand) without spawning a process. **Free.**
- **ffmpeg / ffprobe** - decodes, re-encodes, probes by spawning a process. **Costly.**

One key: everything (cache directory, ffmpeg session, segment URLs) is addressed by
the **decision fingerprint** - a hash of the decision. Same bytes ⇒ same hash ⇒ one
shared encode. The whole system's correctness rests on that hash being **stable**
between the manifest a player holds and every segment it later fetches.

## Reading order - follow one request through the code

Open these in sequence to trace a single playback request end to end:

1. **Entry** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` authenticates, then dispatches to master / variant / segment / audio.
2. **The plan** · `http/transcode_vod_plan.go` marshals the request into a
   `PlaybackRequest`; `app/transcoding/playback_plan_resolver.go` -
   `PlanResolver.Resolve` returns one immutable `PlaybackPlan`: the decision
   (Q2), the ladder class, the shaped ladder, the hardware route, the packager
   and the fingerprint. Every manifest and segment of the session is served from
   it ([ADR-0030](adr/0030-one-plan-one-decision.md)). `makeDecision` and
   `buildFingerprint` in `http/transcode_vod_fingerprint.go` are thin wrappers
   for the call sites without a plan (prewarm, on-demand master).
3. **Q2 · Decision** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (pure): the per-track copy/transcode brain.
   The resolver layers on the request-time pins (`vd=` / `ad=`), the
   audio-resync override, etc.
4. **Q3 · Capability** · `internal/app/transcoding/calibration_service.go` -
   `Current()` returns `domain/transcoding.Derive(report, policy)`
   (`derive.go`): the host's ceilings, derived from the last calibration.
5. **Ladder and admission** · the resolver shapes the ladder (complexity class
   → producer profiles → source/capability caps → CMCD/AV1 caps → admission
   route and ceiling), then **freezes it** on the admission pin
   (`admission_shaper.go`, `FreezeLadder`), read verbatim on every later call -
   the cache-hash invariant made structural. `video_session_limiter.go` holds
   the capacity slot; the route spills from hardware to software when full.
6. **Fingerprint** · a `DecisionFingerprint` (type + `Hash()` in
   `app/transcoding/segment_cache.go`), assembled by the resolver. The hash is
   the cache key.
7. **Producing the bytes** · copy → **mkvgo** `app/transcoding/ondemand.go` (port) /
   `adapters/driven/mkvgo/ondemand.go`; transcode → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` is pure; segments
   land in `/cache/transcode/segments/<hash>/`).
8. **Packaging** · `app/transcoding/manifest_generator.go` - `master.m3u8` /
   `stream_N.m3u8`; `manifest_generator_dash.go` - `manifest.mpd`.

## The two engines, by cost

The **probe dispatcher** (`adapters/driven/prober/dispatcher.go`) is a
chain-of-responsibility keyed on extension: it tries mkvgo, and falls back to
ffprobe on any doubt (parse error, panic, unmappable result, a dropped MP4
media track, DoVi profile 5, unreadable colour). Both fast-paths are always on
(nil gates in `wire.go`); ffprobe is the safety net, never bypassed. A second,
identical dispatcher (`keyframe_dispatcher.go`) handles the seek index.

| Operation | mkvgo (free) | ffprobe / ffmpeg (costly) |
|---|---|---|
| Metadata probe (codec, dims, HDR, bitrate, duration) | `.mkv/.webm/.mp4/.mov` head-only | everything else, or fallback |
| Frame-rate mode (cfr/vfr) | `AnalyzeFrameRateMode` | - (migrated) |
| Remux MKV→MP4 (no re-encode) | `RemuxToMP4` | - |
| On-demand HLS/DASH (copy rung) | `OnDemandPackager` | - |
| Video re-encode / HDR tonemap | - | `hls_transcoder.go` |
| Interlace detect (idet), seek-landing, chromaprint, DoVi P5 | - (needs decode / side-data) | ffmpeg / ffprobe |

Three ffprobe forks still run on MKV/MP4 sources whose data mkvgo already exposes
(duration in `trickplay.go`, bitrate+dims in `complexity_classifier.go`, playback
audio-delay in `audio_delay.go`) - candidates to migrate; small CPU win, mostly wiring.

## Audio is decided separately from video

`makeDecision` produces `VideoDecision` and `AudioDecision` independently. The
"copy rung" (`isCopyRequest`) is gated on the **video** only; even a copied video
usually carries a re-encoded AAC track alongside. The audio decision has three
outcomes: copy, transcode to AAC (ffmpeg, light), or - for **AC-3 / E-AC-3** on a
browser that can't decode it natively - the **wasm path**: the master is made
video-only with an `#EVEY-AUDIO-URI` tag, the AC-3 is **copied** (mkvgo `od_audio`
if the plan builds, else ffmpeg `-c:a copy`), and `ac3go.wasm` decodes it to PCM in
the browser (`WasmAudioEngine.ts`), clock-locked to the video. Safari is excluded
by capability, not by name: `canWasmDecodeAC3()` returns true only when the browser
reports it *cannot* play AC-3 natively - Safari reports it can.

## Packaging: CMAF, HLS, DASH

The container is always **CMAF** (fMP4): a per-variant `init.mp4` plus `.m4s`
segments. (The MPEG-TS branch in `hls_transcoder.go` is dead - `needsFMP4Segments`
always returns true.) Over the *same* segments:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → `EXT-X-MAP` init + `.m4s`) - both engines.
- **DASH** (`manifest.mpd`) - on the mkvgo copy path the packager's own manifest
  (`OnDemandPlan.HasDASH`, byte-identical, zero storage, disabled under AES-128);
  on the ffmpeg path a manifest **declared from the plan** over the same fragments
  the HLS playlists address (`ManifestGenerator.MPD`), written before any fragment
  exists. What makes it honest is the **segment grid** (`domain/transcoding.SegmentGrid`):
  every transcoded rung is cut on the same whole number of frames (72 at 23.976 fps,
  75 at 25, 90 at 29.97), so a segment is exact in the video track's ticks and
  identical across the switch set; the copy fallback declares its keyframe plan. A
  source whose frame rate is unknown or variable stays on the legacy 3 s clock, keeps
  HLS, and answers 404 on `manifest.mpd`. No client is switched to DASH: HLS stays the
  default everywhere. mkvgo's `DASHFromCMAF` over the produced fragments is the
  parity oracle (`internal/pkg/cmafparity`, `TestDASHGridIntegration` at 23.976/25/29.97
  fps: cold, seek, two-rung switch set, separate audio within one AAC frame).

Per-session URLs are re-baked (`transcode_vod_manifest_rebake.go`) to carry the
stream token, for HLS and DASH alike, including cookie-less Chromecast.

## Design patterns in play

- **Ports & adapters (hexagonal)** - the app layer talks to interfaces
  (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`, `StreamProducer`); ffmpeg
  and mkvgo implement them. A new engine is a new adapter.
- **Strategy** - `StreamProducer` (software / hardware / copy), picked by a
  `ProducerSelector`.
- **Chain of responsibility with fallback** - the probe/keyframe dispatchers
  (mkvgo → ffprobe authority).
- **Content-addressed cache** - the `DecisionFingerprint` hash keys the cache and
  the ffmpeg session.
- **Pure core / imperative shell** - `Derive`, `MakePlaybackDecision`, the cost
  model are pure and unit-tested without ffmpeg or a DB.
- **Immutable value object** - `StreamPlan`: the admission decision, emitted once,
  never mutated, so the URLs↔bytes invariant holds by construction.
- **Graceful degradation** - a safe fallback at every step: 720p default capability,
  hw→software spill, copy→transcode, mkvgo→ffprobe.

## The decision brain: the app-layer PlanResolver

The request-handling brain - decision pins, ladder shaping, hardware routing,
fingerprint assembly, copy-rung routing, producer pick - lives in the app
layer: `PlanResolver` (`app/transcoding/playback_plan_resolver.go`), the
concrete `PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Each
request is resolved **once**: the http adapter marshals the `*http.Request`
into a transport-free `PlaybackRequest` (`http/transcode_vod_plan.go`) and
serves off the returned immutable `PlaybackPlan`. Every consumer - manifest
input, shed probe, stale-URL seal, producer pick, segment key - reads that
single resolution, so *within a request* the cache-hash invariant holds by
construction (and the segment hot path went from ~5 profile prefs fetches
to 1). The source-fact helpers (codec, HDR, SAR, ten-bit…) live alongside in
`app/transcoding/source_facts.go`; the adapter keeps thin aliases. Across
requests the invariant still rests on URL baking: **every input to the
decision must be baked into the child URLs**, or a mid-play poll can flip the
plan under the player (`querySuffix` in `manifest_generator.go`).
