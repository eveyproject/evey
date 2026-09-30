# Audio

The audio track is judged separately from the video. In most transcodes it is
the only thing re-encoded: the video is copied and the sound converted to a
format the player reads.

## Which track plays

A track chosen by hand, on the title page or in the player, always wins.
Otherwise the players follow the profile's **Default Audio Language**, then the
track the file marks as default, then the first one. A track flagged as
commentary is skipped in favour of a regular one when the file has both.

Only the track being played is judged: a DTS track you are not listening to
never causes a transcode.

## Copied or re-encoded

The track is copied when the player decodes its codec and has enough channels
for it, within the profile's **Max Audio Channels** (Stereo, 5.1 or 7.1).
Otherwise it is re-encoded to AAC:

- with as many channels as the source, the player and the profile allow, up
  to 5.1 (a 7.1 source is folded to 5.1, which every player decodes; stereo
  sources stay stereo);
- regardless of the sample rate, which is never a reason to re-encode on its
  own.

A copied track inside a transcoded or remuxed stream must also be a format the
player accepts inside the stream's segments. AAC, AC-3 and E-AC-3 always are;
other codecs only when the player says so. When it is not the case, the track
is re-encoded and Stream info says so: a player may decode MP3 in a file and
still refuse it inside a stream.

## Dolby Digital in a browser

Most browsers cannot decode AC-3 or E-AC-3. Instead of re-encoding the track
on the server, the web player decodes it itself, with a decoder that runs in
the page: the track is sent unchanged and Stream info shows it as
*client-decoded*. Browsers that decode Dolby Digital natively, such as
Safari, receive it as an ordinary track; the choice is made by testing the
browser, not by recognising it.

A reverse proxy that sets its own Content-Security-Policy must allow
`'wasm-unsafe-eval'` in `script-src`, otherwise the decoder cannot start and
the film plays without sound. See
[Reverse proxy configurations](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Sound out of sync

**Audio sync** in the player shifts the sound for the current title. A copied
track cannot be shifted, so the server re-encodes it; Stream info shows
**Audio resync requested**.

When the offset is in the file itself (an audio track that starts after the
picture, or tracks that disagree with each other), **Admin → Media Health**
reports it but never rewrites it on its own: an offset that starts late is
usually a deliberate dub alignment. **Check sync** on the file's row lets you
judge it by eye and apply a shift, which its history can undo. See
[Media health](../runbooks/media-health.md).

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: docs/playback-serving.md (Audio is decided separately) ; frontend/packages/client/src/web/codec.ts ; internal/app/transcoding/decision.go - wasmAudioApplies
sample rate: internal/app/transcoding/decision.go - evaluateAudio (no sample-rate check)
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; i18n player.audioSync
labels: i18n auth.settings.playback.maxAudioChannels, audioLanguage
-->
