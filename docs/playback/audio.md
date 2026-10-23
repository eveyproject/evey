# Audio

**In short:**

- You hear your profile's language when the film has it: set it in
  **Settings → Playback → Default Audio Language**. A track you pick by hand
  in the player always wins.
- The sound is sent as it is when your device can play it. Otherwise the
  server converts it to a common format (AAC). Often the sound is the only
  thing converted, and the picture stays untouched.
- On a phone's speaker, headphones or Bluetooth, the Android app plays
  stereo. With the Android or Android TV app on HDMI, you get as many
  channels as the TV or amplifier accepts (5.1, for example).
- Sound out of sync with the picture? See
  [Sound out of sync](#sound-out-of-sync).

The rest of this page explains which track plays, when it is copied or
converted, and with how many channels. The profile settings named here are
in **Settings → Playback**.

## What you will hear, by device

| Your device | What arrives |
| --- | --- |
| A browser | The track as it is when the browser decodes it; AC-3 and E-AC-3 decoded in the page (below); otherwise AAC. A surround AAC track is folded to your output by the browser. |
| An Android phone on its speaker, headphones or Bluetooth | Stereo. The app declares a stereo output, so a surround track is re-encoded to stereo by the server, with a normalised fold. Dolby Digital the app decodes itself is sent as it is and folded by the app. |
| An Android phone or Android TV on HDMI | As many channels as the HDMI sink reports it takes: a 5.1 track stays 5.1 on a 5.1 amplifier. |
| A cast receiver | What the receiver reported about its output: see [Casting](casting.md). |

The Android apps probe the output at each playback start: plugging headphones
in takes effect at the next title, track switch or quality change, not in the
middle of a scene.

## Which track plays

A track chosen by hand, on the title page or in the player, always wins.
Otherwise the players follow the profile's **Default Audio Language**, then the
track the file marks as default, then the first one. A track flagged as
commentary is skipped in favour of a regular one when the file has both.

Only the track being played is judged: a DTS track you are not listening to
never causes a transcode.

## Copied or re-encoded

The track is copied when the player decodes its codec and its output has
enough channels for it, within the profile's **Max Audio Channels** (**Stereo
(2.0)**, **Surround (5.1)** or **Surround (7.1)**). Otherwise it is re-encoded
to AAC:

- with as many channels as the source, the device and the profile allow, up
  to 5.1 (a 7.1 source is folded to 5.1, which every player decodes; stereo
  sources stay stereo);
- at 48 kHz at most: a track above 48 kHz is re-encoded for that reason alone
  (`audio_samplerate_exceeded`); at or below 48 kHz the sample rate is not a
  reason.

A copied track inside a transcoded or remuxed stream must also be a format the
player accepts inside the stream's segments. AAC, AC-3 and E-AC-3 always are;
other codecs only when the player says so. When it is not the case, the track
is re-encoded and Stream info says **Audio track cannot be passed through
unchanged in the stream**: a player may decode MP3 in a file and still refuse
it inside a stream.

## Dolby Digital in a browser

Most browsers cannot decode AC-3 or E-AC-3. Instead of re-encoding the track
on the server, the web player decodes it itself, with a decoder that runs in
the page: the track is sent unchanged and Stream info shows it as
*client-decoded*. Browsers that decode Dolby Digital natively, such as
Safari, receive it as an ordinary track; the choice is made by testing the
browser, not by recognising it. A surround track is decoded in the page only
when the profile's **Max Audio Channels** allows it: a profile set to stereo
gets the AAC re-encode instead, which can fold the channels.

How the page decodes it, and what happens when it cannot:
[ac3go in Evey](ac3go.md).

!!! warning
    A reverse proxy that sets its own Content-Security-Policy must allow
    `'wasm-unsafe-eval'` in `script-src`, otherwise the decoder cannot start
    and the film plays without sound. See
    [Reverse proxy configurations](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Sound out of sync

**Audio sync** in the player shifts the sound for the current title
(positive values play it later). It is offered when the stream comes from the
server as a remux or a transcode; in Direct Play the control reads
**Available during transcoded playback only**. Applying a shift restarts the
stream at the current position. A copied track cannot be shifted, so the
server re-encodes it and Stream info shows **Audio resync requested**.

When the offset is in the file itself (an audio track that starts after the
picture, or tracks that disagree with each other), **Admin → Media Health**
reports it but never rewrites it on its own: an offset that starts late is
usually a deliberate dub alignment. **Check sync** on the file's row lets you
judge it by eye and apply a shift, which its history can undo. See
[Media health](../runbooks/media-health.md#checking-the-sync-by-eye).

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
device channel ceiling: frontend/packages/client/src/native/outputChannels.ts - resolveMaxOutputChannels (STEREO_ONLY_TYPES, hdmi.maxChannelCount) ; frontend/packages/client/src/native/playSession.ts - playerDecodesCopiedAudio ; internal/adapters/driving/http/client_capabilities.go - maxAudioChannelsQuery
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/app/transcoding/source_facts.go - AudioCopyCodecAllowedFor ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK ; internal/app/transcoding/decision.go - wasmAudioApplies ; frontend/packages/client/src/web/codec.ts
sample rate: internal/app/transcoding/decision.go - evaluateAudio (audio_samplerate_exceeded, maxCopiedAudioSampleRate) ; internal/app/transcoding/decision_samplerate_test.go
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; frontend/apps/web/src/features/playback/HLSPlayer.tsx (audioOffsetAvailable=transcodeMode) ; frontend/apps/mobile/app/(auth)/player/[id].tsx (audioOffsetAvailable)
media health: i18n admin.syncPreview.btn ; docs/runbooks/media-health.md
labels: i18n auth.settings.playback.maxAudioChannels, channelsStereo, channels51, channels71, audioLanguage ; player.settings.audioSync, audioSyncHint, audioSyncUnavailable ; player.streamInfo.decisionWasm, reason.audio_passthrough_unsupported, reason.audio_resync_requested
plain opening: frontend/packages/client/src/hooks/usePlayerTrackSelection.ts ; i18n auth.settings.playback.audioLanguage
-->
