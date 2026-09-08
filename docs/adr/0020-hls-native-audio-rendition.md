# ADR-0020: HLS-native audio rendition (EXT-X-MEDIA) for multi-language switching

- Status: Accepted
- Date: 2026-06-14

## Context

A user watching a film wants to switch audio track (original → dubbed, or to a commentary track) without re-fetching the video. In the muxed model - audio baked into every video variant - switching language means a fresh transcode session of the *whole* stream, re-downloading video the client already has. Mainstream servers either run a dedicated audio-extraction service or re-mux the whole stream.

We want per-language audio switching that:

- doesn't re-fetch or re-transcode the video,
- works through standard HLS (so Safari/AVPlayer/cast handle it natively),
- and doesn't regress the existing muxed pipeline for the common single-audio case.

## Decision

Audio is served as an HLS-native **rendition group** using `EXT-X-MEDIA`, with audio segments produced by a dedicated audio-only pipeline, separate from the video variants.

**EXT-X-MEDIA over a separate audio-only ffmpeg-per-switch.** The master manifest declares the audio tracks as an `EXT-X-MEDIA` rendition group keyed off the source audio streams. The player switches `AUDIO` group member without touching the video playlist - the standard HLS mechanism every native client already implements. This is chosen over spawning a fresh audio-only ffmpeg session on each switch (the EAE-style approach): the rendition group is declarative, the client drives the switch, and there is no per-switch server work.

**A dedicated audio segment path.** `internal/adapters/driving/http/transcode_audio_handler.go` + `internal/app/transcoding/audio_segment_orchestrator.go` produce the audio segments independently of the video orchestrator, and the manifest generator grows an audio-rendition branch. The audio segment URL is keyed so each track addresses its own segments.

**Flag-off default preserves the muxed pipeline.** The rendition path is behind a flag that defaults **off**: an install with no need for multi-language audio keeps the proven muxed pipeline exactly as before. The split path is opt-in until it has the same field mileage.

**Composes with the audio-copy / passthrough work.** This sits above the audio *codec* decisions (audio-copy of a source AAC track, multichannel AAC 5.1, AC-3/E-AC-3 passthrough): the rendition group describes *which* audio tracks exist and how to address them; each rendition's bytes are produced by the normal audio decision (copy / transcode / passthrough).

## Consequences

- **Positive:** language switching with zero video re-fetch, through standard HLS that native clients drive themselves - a differentiator over dedicated audio-extraction services. No per-switch server cost.
- **Cost:** a second segment-production path to maintain alongside the muxed one; the manifest generator carries both shapes. Contributors adding audio-codec support (passthrough, TrueHD/Atmos → E-AC-3 JOC) need to understand the rendition-group model, hence this ADR. The flag-off default means the path only exercises in installs that enable it - keep the muxed path as the safe fallback.
- Web clients without native HLS audio-group switching fall back to the muxed behaviour; the feature targets Safari/AVPlayer/tvOS/cast first.

## Amendment - 2026-07-23: the flag now defaults ON

The decision above was written when the rendition path was new and states it is
"behind a flag that defaults **off**". That is no longer true, and had not been
for some time: `internal/config/config.go` reads
`envBool("EVEY_HLS_AUDIO_RENDITION_ENABLED", true)`, so the split path is the
default for every install and the muxed pipeline is the opt-out
(`EVEY_HLS_AUDIO_RENDITION_ENABLED=false`).

This is recorded rather than edited in place: the original reasoning stands, only
its blast radius changed. Two Consequences above are read differently now - "the
flag-off default means the path only exercises in installs that enable it" is
inverted, so the split path is what everyone runs and the muxed path is the one
at risk of bit-rotting from disuse. The safe-fallback argument now cuts the other
way, and the muxed path needs deliberate exercise to stay trustworthy.
