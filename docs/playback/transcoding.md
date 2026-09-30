# Transcoding and server capacity

Transcoding is the expensive outcome of [the playback decision](how-playback-is-decided.md):
Direct Play and Remux cost the server almost nothing and never take a
transcoding slot. This page explains how the server knows what it can encode,
how it shares that capacity between viewers, and what happens when it is full.

## The server measures itself

The capacity is not a setting you estimate. The server measures it:

- **When.** Not at startup: the server starts on what it knew before (or on a
  conservative 720p software default the first time), and runs the measurement
  at its first quiet moment, when nobody is watching and the processor is
  mostly idle. It takes a few minutes. A measurement taken under load is marked
  provisional and redone when the machine is idle.
- **What.** Real encodes of a 4K clip, in SDR and in HDR, on the processor and
  on each accelerator the server detected, in the same shape as a real
  playback. The result is the highest resolution each one sustains in real
  time, and the cost of each kind of stream.
- **Again.** After the hardware changes, or on **Recalibrate** in **Admin →
  Transcoding → Encode calibration**.

While the measurement runs, a playback that needs a transcode shows **The
server is calibrating itself** and is asked to try again shortly. Direct Play
is not affected.

On a processor that sustains none of the resolutions, the server still offers
720p (SDR) and 480p (HDR), one stream at a time.

The measurement uses, in this order, a clip you provide, a reference clip
downloaded once and checked by its SHA256, or a synthetic clip generated on the
spot. A real clip measures more honestly than the synthetic one, which
overstates the cost and can cap a machine lower than it deserves:

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/my-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/my-4k-sdr.hevc
```

## Sharing the capacity

**Live playback capacity** (**Admin → Transcoding**) is the number of video
streams that may be encoded at the same time. It is derived from the
measurement and adjusts on its own when the hardware changes. **Limit
simultaneous video streams** can lower it, never raise it.

- A transcode that only re-encodes the audio (video copied) uses no slot.
- A viewer changing quality keeps a single slot.
- Only the quality a viewer actually watches is encoded, not the whole ladder
  of qualities.

Each stream goes to the cheapest route that can serve it: an accelerator that
has proved what the stream needs (for example HDR tone mapping, or HEVC
output), or the processor. The processor and the accelerators are two separate
pools. When the accelerators are full, a new stream can move to the processor
if it has room; when neither has room, the player shows **The server is busy
transcoding other streams** and can retry.

While someone watches, the server's own background work (thumbnails, intro
detection, media analysis) pauses, so that playback keeps the machine: see
[Background work and server load](../operations/background-work.md).

When one kind of source keeps making viewers wait despite all this, the server
lowers its maximum quality for a while, then lifts the limit on its own. See
[Monitoring and logs](../operations/monitoring.md#how-playback-actually-went).

## What is encoded

| | Default |
| --- | --- |
| Video codec | H.264. HEVC when the player decodes it and the graphics card encodes it; AV1 when enabled (**AV1 output**, hardware only) and the player decodes it. |
| Resolution | The source's, unless a limit applies: the player's maximum, the bitrate limit, or what the server can encode in real time. |
| Bitrate | Follows the source, scaled to the resolution. **Video bitrate ceiling** caps it. |
| Audio | AAC. See [Audio](audio.md). |
| Delivery | HLS, in fragmented MP4 segments. |

<figure class="evey-shot" markdown="span">
  ![The two video bitrate ceilings in Admin → Transcoding](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>The two ceilings of <strong>Admin → Transcoding</strong>: for every viewer, and for viewers outside the local networks.</figcaption>
</figure>

## The transcoding cache

Encoded segments are kept on disk in the `evey-cache` volume and reused when
another viewer, or the same one later, plays the same title with the same
decision. **Admin → Transcode Cache** shows its size and clears it; the
**Transcode cache budget** (**Admin → Server Config → Server Settings**) caps
it. Clear it
after an update that changes how videos are encoded, so that nobody is served
segments from the previous version.

## Checking what happens

- **Admin → Now Playing**: every stream, its method track by track, and the
  backend encoding it.
- **Admin → Transcoding → Encode calibration**: the measured ceilings and the
  accelerators in use.
- **Admin → Streaming Quality**: how playback went for real viewers.
- **Load test** (**Admin → Transcoding**): simulates viewers to measure what
  the server sustains. Run it only when nobody is watching.

<!-- sources
calibration: internal/app/transcoding/calibration_service.go ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md ; i18n player.calibrating
capacity: i18n admin.transcoding.maxVideoSessionsDesc, videoSessionsAutoAdjusts ; internal/app/transcoding/admission_shaper.go ; internal/app/transcoding/video_session_limiter.go
busy: i18n errors max_concurrent_reached
software floors: internal/domain/transcoding/capability.go
output codec: internal/app/transcoding/resolver_route.go - outputCodecFor ; i18n admin.calibration.av1Label, av1Hint
single variant: internal/config/config.go (EVEY_TRANSCODING_SINGLE_VARIANT)
cache budget, automatic limits: i18n admin.serverSettings.transcodeCacheMaxGB, admin.qoe.overrides.*
-->
