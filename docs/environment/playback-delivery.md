# Playback delivery and downloads

The switches behind how streams are packaged and what the server prepares
for downloads. The defaults are the recommended behaviour: these exist to
turn a path off while investigating a problem, or to opt into one that is
off. How a stream is chosen in the first place:
[How playback is decided](../playback/how-playback-is-decided.md).

A variable with a panel key can also be set as a row with that key in
**Admin → Server Config → Config entries**; the row wins over the
environment. The panel suggests the download keys; the others are typed in
as they appear here. All of them are read at startup, so a change applies at
the next restart whichever way it is made.

## Packaging

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | on | Serves a file whose tracks can be copied by repackaging it on the fly, without ffmpeg and without writing to the cache. `off`: such files go through the ffmpeg copy path instead. | Config entries `transcoding.hls_ondemand`, restart |
| `EVEY_REMUX_COPY` | on | The ffmpeg copy path: the video is copied without re-encoding when the file allows it. `off`: those files are transcoded. | Config entries `transcoding.remux_copy`, restart |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | Offers the audio tracks as separate renditions, so a player changes language without fetching the video again. Also lets a copied video play with an audio track re-encoded on the side. | none |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | Adds one low transcoded quality next to a copied stream, so a player whose connection collapses can step down instead of freezing. Costs nothing until a player uses it. | none |
| `EVEY_HLS_ONDEMAND_CONCAT` | on | Gapless playback across episodes, when the web player asks for it. An episode that cannot be copied falls back to the normal stream. | Config entries `transcoding.hls_ondemand_concat`, restart |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | off | Puts only the selected audio track in each stream instead of all of them. | Config entries `transcoding.hls_ondemand_track_trim`, restart |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | off | Encrypts repackaged streams with AES-128 (HLS only: no DASH manifest is offered then). | Config entries `transcoding.hls_ondemand_encrypt`, restart |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | With encryption on, changes the key every N segments; `0` = one key per title. | Config entries `transcoding.hls_key_rotation_segments`, restart |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | Lets a Chromecast decode Dolby Digital itself instead of receiving AAC. Off because it overloaded low-end receivers without saving anything. Browsers are not affected. | none |

## Downloads

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | on | Prepares a download whose audio a phone can play (AAC), video untouched. Only the audio is re-encoded, which takes minutes. | Config entries `transcoding.compat_download`, restart |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | off | Offers a smaller 720p copy for downloads. It re-encodes the video, at about real time on a weak machine, which is why it is off until you turn it on. | Config entries `downloads.quality_choice`, restart |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296` (4 GiB) | The size above which the app offers the smaller copy; the phone also offers it when the film does not fit. | Config entries `downloads.reduce_above_bytes`, restart |

## Player

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Seconds skipped by the forward button. | none |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Seconds skipped by the back button. | none |

<!-- sources
packaging flags: internal/adapters/driving/http/transcode_vod_wire.go (serverFlagEnabled, resolveServerFlag: hls_ondemand, remux_copy, hls_ondemand_concat, hls_ondemand_track_trim, hls_ondemand_encrypt, hls_key_rotation_segments)
renditions, fallback rung, cast wasm: internal/config/config.go - AudioRenditionEnabled, CopyFallbackRungEnabled, CastWasmAudioEnabled
downloads: cmd/evey/http.go (compat_download, quality_choice, reduce_above_bytes via resolveFlag)
config entries catalogue: frontend/apps/web/src/features/admin/operatorFlags.ts ; i18n admin.serverConfig.entriesTitle, appliedOnRestart
skip: internal/config/config.go - EVEY_PLAYBACK_SKIP_*
-->
