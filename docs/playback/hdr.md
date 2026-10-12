# HDR and Dolby Vision

**In short:**

- **HDR** is a picture with brighter highlights and richer colours than the
  usual picture (called **SDR**). Dolby Vision is one kind of HDR. HDR needs
  a screen that can show it.
- On an HDR screen with the Android or Android TV app (and a device that
  decodes HEVC), you get the original HDR picture, as long as nothing else
  forces a conversion.
- In a web browser, and on any screen that is not HDR, the server converts
  the picture to SDR. That is on purpose: HDR shown on an SDR screen looks
  grey and washed out.
- Converting HDR to SDR is the heaviest work the processor can do. A
  graphics card helps: see
  [Hardware acceleration](../administration/hardware-acceleration.md).

The rest of this page explains each case: the original picture, a re-encode
that keeps the HDR, or an SDR conversion. The general rules of playback are
in [How playback is decided](how-playback-is-decided.md).

## What you will see, by device

| Your screen | What arrives |
| --- | --- |
| An HDR screen on Android TV or an Android phone, with an HEVC decoder | The HDR10 or HLG original, untouched, unless something else (bitrate, resolution, a burned-in subtitle) forces a re-encode. |
| A browser | An SDR conversion, always: the web player does not declare an HDR screen, so the server tone-maps for it. |
| A cast receiver | What the receiver reported about its own screen: see [Casting](casting.md). |
| An SDR screen, anywhere | An SDR conversion. |

Showing HDR on an SDR screen without conversion gives washed-out, grey
colours, so the server never sends it. The Android apps probe the display at
each playback start, not once at install: plugging an HDR television into a
box takes effect at the next title, track switch or quality change.

## When the original is kept

The HDR10 or HLG video is sent as it is when the player reports that its
screen displays HDR and it decodes the codec, and no other reason (bitrate,
resolution, container) requires a re-encode.

## When the video must be re-encoded anyway

A bitrate limit, a resolution limit or a subtitle to burn in can force a
re-encode of an HDR source. The HDR is then kept if all of these hold:

- the player reports an HDR screen and decodes HEVC;
- the server has a graphics card that proved, during its measurement, that it
  encodes HEVC 10-bit with HDR;
- that card is available for this stream.

The result is HEVC 10-bit with the source's HDR signalling (BT.2020 colour,
PQ or HLG transfer) carried over. HDR10+ dynamic metadata is not carried: an
HDR10+ title plays as HDR10 on this path.

When one of the conditions fails, the processor or the card tone-maps the
picture to SDR before encoding it. Stream info then lists **HDR converted to
SDR (tone-mapping)**.

If the original could have been sent but the server cannot serve it as a copy
yet (its seek index is still being built, for example), the video is re-encoded
with the HDR kept on such a card, and Stream info shows **Copy unavailable, HDR
kept through the re-encode**.

## When the screen is SDR

A player that does not report an HDR screen receives a tone-mapped SDR
picture. Tone mapping runs on the graphics card when it has proved it can
(VAAPI with OpenCL, for example), otherwise on the processor: with a
SIMD-optimised filter (AVX2) when the ffmpeg build provides it, with a
portable filter chain otherwise.

!!! note
    On the processor, tone mapping is the most expensive transcode there is.
    The server measures a separate, usually lower, resolution ceiling for HDR,
    so an HDR title may be converted at a lower resolution than an SDR one on
    the same machine. See [Transcoding](transcoding.md#the-server-measures-itself).

## Dolby Vision

A player's report says whether its screen displays HDR, not whether it
supports Dolby Vision, and Evey never infers Dolby Vision support from HDR
support: sending Dolby Vision to a screen that only handles HDR10 gives wrong
colours. The stream is copied only when it carries a base layer that displays
correctly without any Dolby Vision decoding:

| Dolby Vision profile | HDR player | SDR player |
| --- | --- | --- |
| **8.1** (HDR10-compatible base layer) | Treated as HDR10: the rules above apply, the original can be kept. | Tone-mapped to SDR. |
| **7** (UHD Blu-ray) | The HDR10 base layer is re-encoded as HEVC 10-bit with HDR kept. Stream info: **Dolby Vision base layer re-encoded (HDR kept)**. | Tone-mapped to SDR. |
| **5** (no HDR10 base layer) | Tone-mapped to SDR. | Tone-mapped to SDR. |

## Checking what happened

**Stream info** shows the video decision and, among the reasons, the tone
mapping or the HDR kept. Every route that tone-maps reports it there, with one
exception: when the graphics card is full at the moment a stream starts, the
stream moves to the processor after Stream info was computed. **Admin → Now
Playing** shows the backend actually used.

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateVideo (hdr_tonemapping_needed, dovi_base_layer, doViBaseLayerIsHDR10)
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable) ; internal/app/transcoding/resolver_route.go - naturalRoute, passthroughWanted, hwTonemapWanted
hdr signalling carried: internal/adapters/driven/ffmpeg/hls_args_hw.go - hwHDRPassthroughArgs (no HDR10+ metadata)
dovi copy rule: internal/app/transcoding/copy_eligibility.go (dovi_profile, DoViBLCompat)
who declares an HDR screen: frontend/packages/client/src/native/displayHdr.ts - probeDisplayHdr ; frontend/packages/client/src/native/playSession.ts (supportsHdr) ; frontend/apps/web/src/cast-receiver/receiverLogic.ts (hdr=1) ; the web player sends no hdr parameter
tonemapx, software HDR ceiling: internal/adapters/driven/ffmpeg/tonemapx.go ; internal/pkg/hdr/hdr.go ; docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.* ; admin.nowPlaying
plain opening: rules below ; tone mapping cost: internal/app/transcoding/calibration_service.go (separate HDR ceiling)
-->
