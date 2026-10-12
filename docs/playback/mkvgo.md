# MKVGo in Evey

**In short:**

- MKVGo is a part of Evey that reads and repackages video files (MKV, WebM,
  MP4, MOV) without converting the picture or the sound.
- Where it can, it is what serves **Remux** playbacks, extracts subtitles,
  and reads your files during a scan. Anything that needs real conversion goes to
  FFmpeg, a separate video tool.
- You have nothing to set. **Admin → Now Playing** shows, track by track,
  whether MKVGo or FFmpeg served a playback.

The rest of this page is for the reader who wants to know what the server
does without starting FFmpeg, and where FFmpeg still takes over. MKVGo runs
inside the Evey server process.

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

Start and seek do not wait for a process or for a first segment on disk: the
segment is built on request from the file's index. Whether a file takes this
path depends on what the player decodes:
[How playback is decided](how-playback-is-decided.md).

## What MKVGo does

| Work | Where it shows | When MKVGo cannot |
| --- | --- | --- |
| **Reading files** | During a library scan, codecs, resolution, HDR, bitrate and duration are read from the head of the file. | ffprobe reads it. It also takes over on any doubt: a parse error, or colour information MKVGo could not read on an HDR-capable codec. |
| **Keyframe index** | The positions playback can start from, read from the file's index; they let Remux and video copy cut segments on keyframes. | ffprobe. |
| **Streaming without re-encoding** | Remux: HLS segments in fragmented MP4 (CMAF), built on request from a plan kept per file. A segment is written to the connection straight from the source file as it goes out: nothing is assembled in memory first, nothing is written to the transcoding cache. A DASH manifest is produced over the same segments; the apps use HLS. | FFmpeg copies the tracks, still without re-encoding, into segments written to the cache. |
| **MP4 for the browser** | A Matroska file a browser cannot open is rewritten once as a fast-start MP4, kept in a cache. | - |
| **Subtitles** | Subtitle tracks embedded in an MKV are extracted on request, text tracks to WebVTT and bitmap tracks (PGS) to images. Where the file's own index (Cues) covers a track completely, the server reads only the blocks it points to instead of the whole file, and keeps that block index under its data directory (`subtitle-index/`) for the next request. A track that still needs a full read is answered "being prepared" with a retry delay, and the players poll until it is ready. | ffmpeg extracts the track, reading the file through. |
| **Preview images** | The keyframe nearest the requested position is found through the index, and FFmpeg decodes only that frame. Used when that keyframe is within about two seconds of the request. | FFmpeg seeks and decodes up to the exact position. |
| **Episode chains** | The server can package several episodes as one continuous stream (`?gapless=1` on the stream address). No app offers the switch yet. | Each episode plays on its own. |
| **Media Health** | The diagnosis (seek index, audio start, tracks stored in blocks), the rebuilt index, the resync past a damaged stretch, and moving an audio track by an offset. See [Media health](../runbooks/media-health.md). | Re-muxing tracks stored in blocks is done by FFmpeg. |

## What stays with FFmpeg

- Every re-encode, including HDR tone mapping.
- Decoding pictures: the seek bar thumbnails, the frame of a preview image.
- Analysis that needs decoded data: interlace detection, audio fingerprints
  for intro detection.
- Containers other than MKV/WebM and MP4/MOV, such as AVI or MPEG-TS.

## Load, replaced files and updates

**How much it reads at once.** Building a segment reads one window of the
source, video and audio together: 6 to 10 MiB for a 1080p file, about 15 MiB for
a UHD remux. The server reads as many windows at once as it has processors,
and fewer on a machine with little memory: all the windows in flight must fit
in a quarter of the memory the server allows itself (half of the container's
limit, or of the host's memory). Requests beyond that wait for a free slot,
so the memory used does not grow with the audience; on a small host, a crowd
starting at the same moment sees its first segments arrive one after the
other rather than the server running out of memory.

**A file replaced during playback.** Every segment address names the file it
was planned from. When a file is replaced on disk (a better copy, a repair),
a segment requested at that moment is rebuilt once from the new file, and the
addresses of the old file's playlist answer `410 Gone` instead of mixing the
two files; opening the title again plays the new one.

**After an update.** Segment addresses carry the MKVGo version, so a browser
or a proxy cache never serves a segment built by an older version, and no
segment of this path is kept on disk to clear. The MP4 copies made for
browsers are named after the version of their format: when it changes, each
one is rebuilt the next time it is asked for.

**Watching it.** On `GET /metrics` (see
[Monitoring and logs](../operations/monitoring.md#prometheus-metrics)),
`evey_mkvgo_walks_in_flight` is the number of windows being read right now,
and `evey_mkvgo_plan_stats` sums the packager's counters by `stat`. A
`walks_in_flight` sitting at its ceiling while `stat="slot_waits"` climbs means
viewers are queuing for the processor; `stream_fallbacks` counts segments that
had to be assembled in memory instead of streamed.

## Seeing which engine served a stream

**Admin → Dashboard** and **Admin → Now Playing** name the engine of each
track: `mkvgo` or `ffmpeg`. A Remux can show both, for example the video
packaged by MKVGo and the audio copied by FFmpeg.

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
streamed segments: internal/app/transcoding/ondemand.go - OnDemandResource (Bytes nil, WriteRange) ; internal/adapters/driving/http/transcode_ondemand_stream.go - servePlanResourceStreamed
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driving/http/subtitle_handler.go - extractSubtitleVTT ; subtitle_pgs.go ; subtitle_index_cache.go - fromCues, resolveAndStore, newSubtitleIndexCache ; subtitle_preparing.go (202 + Retry-After) ; subtitle_prewarm.go - ensureIndex
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
reads at once: internal/adapters/driven/mkvgo/budget.go - BudgetFor (one walk per CPU, walks within memLimit/4 at 15 MiB each) ; cmd/evey/wire_infra.go (applied after the memory limit) ; cmd/evey/memlimit.go - applyMemoryLimit (half the cgroup or host budget)
replaced file: internal/adapters/driving/http/transcode_ondemand.go - planSourceParam, servePlanSourceGone (410), replay once on a rebuilt plan ; internal/adapters/driving/http/transcode_vod_manifest_rebake.go - stampPlanSourceURIs
after an update: packager version in segment URLs (?v=) ; internal/adapters/driving/http/remux_handler.go - remuxCacheFormat, remuxCacheName
metrics: internal/pkg/metrics/metrics.go - MkvgoWalksInFlight, MkvgoPlanStats ; cmd/evey/wire.go - startMkvgoMetrics
plain opening: rules below ; i18n admin.sessions.track.engine
-->
