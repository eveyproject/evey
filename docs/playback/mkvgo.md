# MKVGo in Evey

MKVGo is the media library Evey runs inside its own process to read and
repackage MKV/WebM and MP4/MOV files. This page is for the reader who wants to
know what the server does without starting FFmpeg, and where FFmpeg still
takes over.

MKVGo works on the structure of a file: its headers, its index, its tracks and
their packets. It never decodes or encodes a picture or a sound; everything
that needs server-side decoding goes to FFmpeg. It is developed as a separate
Go library: [github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo).

## Three levels of work

| Level | What happens | What it costs the server |
| --- | --- | --- |
| **Direct Play** | The file is served as it is. | A file read. |
| **MKVGo** | Structural work: reading a file, repackaging its tracks for streaming, rebuilding its index, moving a track on the timeline. | Code inside the server process: no FFmpeg or ffprobe started per stream, and no segments written to disk for on-demand packaging. |
| **FFmpeg** | Real transformations: re-encoding, tone mapping, decoding a picture. Also the fallback whenever MKVGo cannot handle a file. | One process per job. |

An FFmpeg copy (`-c copy`) re-encodes nothing either and stays cheap; what
MKVGo saves on top is the process started for each session and the segments
written to the transcoding cache. A re-encode is the expensive case, and a
video re-encode is the only work that takes a transcoding slot: see
[Transcoding and server capacity](transcoding.md).

On capable clients, around 80% of video playback paths can avoid FFmpeg
entirely: what the player decodes decides how often the file can be served as
it is or only repackaged. How that is decided: [How playback is decided](how-playback-is-decided.md).

## What MKVGo does

| Work | Where it shows | When MKVGo cannot |
| --- | --- | --- |
| **Reading files** | During a library scan, codecs, resolution, HDR, bitrate and duration are read from the head of the file. | ffprobe reads it. It also takes over on any doubt: a parse error, or colour information MKVGo could not read on an HDR-capable codec. |
| **Keyframe index** | The positions playback can start from, read from the file's index; they let Remux and video copy cut segments on keyframes. | ffprobe. |
| **Streaming without re-encoding** | Remux: HLS segments in fragmented MP4 (CMAF), built on request from a plan kept per file. A DASH manifest is produced over the same segments; the apps use HLS. | FFmpeg copies the tracks, still without re-encoding. |
| **MP4 for the browser** | A Matroska file a browser cannot open is rewritten once as a fast-start MP4, kept in a cache. | - |
| **Subtitles** | Subtitle tracks embedded in an MKV are extracted to SRT or ASS. | - |
| **Preview images** | The keyframe nearest the requested position is found through the index, and FFmpeg decodes only that frame. Used when that keyframe is within about two seconds of the request. | FFmpeg seeks and decodes up to the exact position. |
| **Episode chains** | The server can package several episodes as one continuous stream (`?gapless=1` on the stream address). No app offers the switch yet. | Each episode plays on its own. |
| **Media Health** | The diagnosis (seek index, audio start, tracks stored in blocks), the rebuilt index, the resync past a damaged stretch, and moving an audio track by an offset. See [Media health](../runbooks/media-health.md). | Re-muxing tracks stored in blocks is done by FFmpeg. |

## What stays with FFmpeg

- Every re-encode, including HDR tone mapping.
- Decoding pictures: the seek bar thumbnails, the frame of a preview image.
- Analysis that needs decoded data: interlace detection, audio fingerprints
  for intro detection.
- Containers other than MKV/WebM and MP4/MOV, such as AVI or MPEG-TS.

## Seeing which engine served a stream

**Admin → Dashboard** and **Admin → Now Playing** name the engine of each
track: `mkvgo` or `ffmpeg`. A Remux can show both, for example the video
packaged by MKVGo and the audio copied by FFmpeg.

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driven/mkvgo/subtitle_extractor.go - ExtractSubtitle
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
-->
