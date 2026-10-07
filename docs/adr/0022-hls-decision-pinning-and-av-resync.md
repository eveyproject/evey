# ADR 0022 - HLS decision pinning and A/V resync

Status: Accepted (amended: fp seal / 410 refusal, decision moved to the app-layer PlanResolver)

## Context

The HLS VOD pipeline chooses, per playback, how to serve each track: the video
can be REMUXED (`-c:v copy`, the "copy rung") or re-encoded ("transcode"), and
the audio likewise. That choice - the *decision* - depends on the media, the
client's declared codec/HDR capabilities, and the active profile's preferences.
It is computed once per request by the app-layer resolver
(`PlanResolver.Resolve`, `internal/app/transcoding/playback_plan_resolver.go`;
the http adapter only marshals the request) and its cache identity is the
`DecisionFingerprint` (`internal/app/transcoding/segment_cache.go`).

Three production failure classes shaped this subsystem, and the invariants
below exist to prevent each from recurring:

1. **Green frames on a cast receiver.** Background keyframe enrichment landed
   mid-viewing, the same live playlist URL flipped from transcode to the copy
   rung between two polls, and source segments were served under an init made
   for the re-encode.
2. **Constant A/V desync on some muxes.** An audio track whose first timestamp
   sits well after the video (a container defect) plays with a fixed lag when
   source PTS are preserved.
3. **User-driven resync.** Users need to nudge audio or subtitles live, without
   re-opening the file.

## Decision

**1. Every decision input is baked into the child URLs.** A playback's first playlist computes the decision and stamps it into every
descendant URL via `querySuffix` (`manifest_generator.go`): `vd=copy|transcode`
for video, `ad=copy|transcode` for audio. Follow-up polls read the pin back
(`pinnedVideoDecision`/`pinnedAudioDecision`) instead of re-deriving, so
background enrichment can never flip a live session's plan mid-stream.

- Pinning `transcode` always holds: a re-encode is always serveable.
- Pinning `copy` cannot be *forced*: if the fresh decision no longer supports
  copy (keyframes wiped, HDR truth changed), the plan is structurally gone, so
  the session flips to transcode WITH a logged warning (carrying the request
  path) rather than 500-ing. The player may need a restart; a warning beats a
  broken stream.

RULE for anyone adding a decision input: it MUST (a) enter `DecisionFingerprint.Hash()`
so different bytes get a different cache key, and (b) ride `querySuffix` so the
whole session stays coherent across polls. Miss (a) and two byte-streams alias
in the cache; miss (b) and the decision flips under the player.

**1b. The decision hash is SEALED into every URI, and a stale seal is refused (amendment).** Pins protect the *decision*; they do not protect the *ladder shape*. `stream_N`
is a ladder INDEX: when the shaped ladder changed mid-play (capability flap,
admission pin gone cold), the same URL silently mapped to a *different*
rendition and the player decoded a stranger's bytes - on-screen, a full-frame
rainbow. Two mechanisms close this:

- The manifest seals the current `DecisionFingerprint.Hash()` into every URI it
  emits (`fp=`). A later request whose freshly-derived hash no longer matches
  is refused with **HTTP 410** and `X-Evey-Error: stale_rendition` - the player
  refetches the master and rebuilds coherently. A URL with no `fp=` (cached
  link, pre-feature client) is served as before: degrade open. The WARN log
  that accompanies each refusal is the direct detector of "the ladder moved
  under a live player", which previously left no trace (see the incident
  runbook).
- The shaped ladder itself is **frozen on the stream's admission pin**
  (`VideoAdmissionShaper.FreezeLadder`): shaped once, read verbatim by every
  later manifest- and segment-time call, so within a live stream the ladder
  cannot drift in the first place. The 410 seal remains the backstop for
  cross-stream staleness (a pin that expired and re-derived differently).

**2. Source audio start-delay is cancelled with a second, shifted input.** When the probed source delay exceeds the noise floor (~0.1 s) AND the audio is
being transcoded, the builder feeds audio from a SECOND ffmpeg input shifted by
`-itsoffset`, leaving the video input (and its seek bookkeeping) untouched.

Only TRANSCODED audio is eligible. Copied audio packets cannot be trimmed at the
seek edge, and a second input there was measured to hijack the `make_zero`
zero-point of copy-on-seek sessions and land the video off plan, which the
copy-timeline tripwire caught.

**3. User resync composes into the same net input shift.** - **Subtitles** shift entirely client-side: the player re-anchors each loaded
  cue to `sourceTime + offset` (`useSubtitleManager.applySubtitleOffset`,
  keyed by a `WeakMap` of source times so a zero clamp is exactly reversible).
  Instant, no server round-trip, works for embedded and external tracks.
- **Audio** needs a re-encode from a shifted input, so `audioOffsetMs` is a
  decision input: it forces audio to transcode, forks the fingerprint, and the
  builder combines it with the source-delay cancel into ONE net shift:
  `-itsoffset -(AudioStartDelaySec - AudioUserOffsetSec)`. The value is
  quantized to the 100 ms UI grid so a crafted sweep of adjacent-millisecond
  offsets cannot fork an unbounded fleet of ffmpeg sessions.

## Consequences

- `BuildFFmpegArgs` stays PURE (probes happen in `Start`); the second-input
  logic lives in `audioDelayCancelActive`/`audioNetShiftSec` and is shared by
  all builders, so `-map 1:a` never appears without its input.
- The copy-timeline tripwire (`verifySegmentTimeline`, ±500 ms) is the
  runtime backstop: it logs ERROR + metric rather than corrupting playback if a
  copy session ever drifts.
- **`make transcode-itest` is mandatory** on any change here: it decodes the
  served AUDIO CONTENT against an adversarial fixture and correlates it, in
  both offset signs. Unit suites cannot catch a zero-point hijack.
