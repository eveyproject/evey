# ac3go in Evey

**In short:**

- Chrome, Firefox and Edge cannot play Dolby Digital (AC-3) or Dolby
  Digital Plus (E-AC-3) sound. Instead of having the server convert it, Evey's
  web player decodes it inside the page, with ac3go, a decoder that runs in
  the browser as WebAssembly.
- The sound reaches you as it is in the file, and the server does no audio
  conversion for it.
- You have nothing to set. **Stream info** in the player shows the audio as
  *client-decoded*, and **Admin → Now Playing** shows *ac3go in the player*
  for that track.
- If the decoder cannot run, the player switches to sound converted by the
  server (AAC) on its own.

The rest of this page explains when this path is taken, how the sound stays
in sync with the picture, and what to check when a film plays without sound.
Which audio track plays, and when it is copied or converted in general, is on
[Audio](audio.md).

ac3go is developed as a separate library, written in Go with no dependency:
[github.com/gravity-zero/ac3go](https://github.com/gravity-zero/ac3go).

## When the browser decodes the sound itself

All of these must hold:

| Condition | Otherwise |
| --- | --- |
| The browser cannot play AC-3 or E-AC-3 itself, and it runs WebAssembly. The player tests the browser for both rather than recognising it by name. | A browser that decodes Dolby Digital natively, such as Safari, receives the track as an ordinary one. |
| The audio track being played is AC-3 or E-AC-3. Other tracks of the film do not count. | Another codec follows the rules on [Audio](audio.md). |
| The track has at most six channels (5.1), and no more than the profile's **Max Audio Channels**. This path hands over the track as it is in the file, so it cannot fold the channels down. | The server converts the track to AAC, which can fold the channels for a stereo profile. A 7.1 E-AC-3 track is always converted. |
| The film plays through the server's stream (Remux or Transcode), not as a file read directly. | The track follows the decision for that playback. |

## How it plays

The picture plays through the browser's usual player, muted. The sound comes
from a separate audio stream that the server names in the playlist: the
track as it is in the file, packaged by [MKVGo](mkvgo.md) when it can, by
FFmpeg copying it otherwise. Neither decodes it.

In the page, a background worker loads ac3go and decodes each audio segment
to plain sound, away from the code that draws the picture. The player then
plays that sound through the browser's audio output, kept in step with the
picture: the picture is the clock, and the sound is scheduled against it. A
pause stops both together. A seek, or a gap after the stream stalled,
re-aligns the sound to the new position.

A 5.1 track is handed to the browser in the channel order it expects, so the
browser can fold it for stereo speakers or headphones.

At the start of a film, the decoder takes a moment to load. When the server
also offers the track converted to AAC, that sound plays during the moment,
and ac3go takes over once it is ready. The
decoder is downloaded once by the browser and checked again at each use, so a
server update never mixes an old and a new copy.

## If ac3go cannot run

When the decoder fails to load, fails on a segment, or the server named no
audio stream for it, the player stops using it for the rest of the title and
asks the server for the normal stream, with the sound converted to AAC. The
film keeps its sound; **Stream info** then shows the conversion instead of
*client-decoded*.

!!! warning
    A reverse proxy that sets its own Content-Security-Policy must allow
    `'wasm-unsafe-eval'` in `script-src`, otherwise the decoder cannot start.
    It must also serve the decoder files under `/wasm/` like the rest of the
    web app. See
    [Reverse proxy configurations](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## On a cast receiver

Evey's Chromecast receiver does not use ac3go. On TV boxes with little
memory, receivers decoding the sound this way stopped after a few minutes of
playback. The TV gets Dolby Digital as it is when its receiver reports that
it passes it through, and the server's AAC otherwise: see
[Casting](casting.md).

## The library and its licence

ac3go is published under the GNU AGPLv3, like Evey. It is built from source
when the Evey image is built (compiled to WebAssembly with the Go toolchain),
and its licence is served next to the decoder, at `/wasm/ac3go.LICENSE.md`.

<!-- sources
conditions: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK, WasmAudioMaxChannels ; internal/app/transcoding/decision.go - wasmAudioApplies
browser test: frontend/packages/client/src/web/codec.ts - canWasmDecodeAC3, isWasmDecodableAudioCodec
stream topology: internal/adapters/driving/http/transcode_ondemand.go - serveWasmAudioMaster, wasmAudioEffective
player: frontend/apps/web/src/features/playback/HLSPlayer.tsx - wasmAudioActive, wasmAudioDisabled, wasmAudioLeg ; frontend/apps/web/src/features/playback/wasmaudio/WasmAudioEngine.ts ; frontend/apps/web/src/features/playback/wasmaudio/relayHandover.ts - planRelayHandover
worker: frontend/apps/web/public/wasm/ac3Worker.js
cache: internal/adapters/driving/http/spa.go - setStaticCachePolicy
cast: frontend/apps/web/src/cast-receiver/main.ts - CAST_WASM_ENABLED ; internal/config/config.go - CastWasmAudioEnabled
build and licence: Dockerfile - wasmbuild ; frontend/apps/web/public/wasm/ac3go.LICENSE.md
csp: internal/adapters/driving/http/helpers.go - securityHeadersMiddlewareFn
labels: i18n player.streamInfo.decisionWasm ; admin.sessions.track.engine.client
-->
