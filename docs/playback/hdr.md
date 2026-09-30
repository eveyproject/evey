# HDR and Dolby Vision

An HDR picture can reach the screen in three ways: the original, untouched; a
re-encode that keeps the HDR; or a re-encode tone-mapped to SDR. Which one
applies depends on the player's screen, on the format of the HDR, and on what
the server's hardware has proved it can do.

## When the original is kept

The HDR10 or HLG video is sent as it is when the player reports that its
screen displays HDR and it decodes the codec, and no other reason (bitrate,
resolution, container) requires a re-encode. The general rules are in
[How playback is decided](how-playback-is-decided.md).

## When the video must be re-encoded anyway

A bitrate limit, a resolution limit or a subtitle to burn in can force a
re-encode of an HDR source. The HDR is then kept if all of these hold:

- the player reports an HDR screen and decodes HEVC;
- the server has a graphics card that proved, during its measurement, that it
  encodes HEVC 10-bit with HDR;
- that card is available for this stream.

The result is HEVC 10-bit with the static HDR10 information (colour space,
transfer, mastering display) carried over. HDR10+ dynamic metadata is not
carried: an HDR10+ title plays as HDR10 on this path.

When one of the conditions fails, the processor or the card tone-maps the
picture to SDR before encoding it. Stream info then lists **HDR converted to
SDR (tone-mapping)**.

If the original could have been sent but the server cannot serve it as a copy
yet (its seek index is still being built, for example), the video is re-encoded
with the HDR kept on such a card, and Stream info shows **Copy unavailable, HDR
kept through the re-encode**.

## When the screen is SDR

A player that does not report an HDR screen receives a tone-mapped SDR
picture. Showing HDR on an SDR screen without conversion gives washed-out,
grey colours, so the server never sends it.

Tone mapping runs on the graphics card when it has proved it can (VAAPI with
OpenCL, for example), otherwise on the processor with a SIMD-optimised filter
(AVX2 on x86-64, NEON on ARM). On the processor it is the most expensive
transcode there is: the server measures a separate, usually lower, resolution
ceiling for HDR, so an HDR title may be converted at a lower resolution than an
SDR one on the same machine.

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
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable), Reported ; internal/app/transcoding/resolver_route.go - naturalRoute
hdr10 static metadata, hdr10+, profile 5: internal/app/transcoding/resolver_route.go - hdrPassthroughWanted
tonemapx, software HDR ceiling: docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.*
-->
