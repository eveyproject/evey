# Background work

The passes the server runs on its own, and how they share the processor. How
they behave: [Background work and server load](../operations/background-work.md).

## CPU share

The share of the processor background tasks may use is set in **Admin →
Server Config → Processing power**, live, without a restart. It has no
environment variable. The two below size a pool by hand and bypass that share
for it:

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | from the share | Files analysed in parallel during a scan. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | from the share | Titles analysed at once for intros and credits. |

## Passes

Each variable has a key, also accepted as a row in **Admin → Server Config →
Config entries**, which suggests these keys; the row wins. All are read at
startup, and **Background work** in the same screen sets the main ones.

| Variable | Panel key | Default | Effect |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | `transcoding.pause_bg_on_playback` | on | Pauses background work while a stream plays. `off` keeps it running. |
| `EVEY_KEYFRAME_BACKFILL` | `transcoding.keyframe_backfill` | on | Indexes the keyframes of scanned files and classifies their frame rate. Direct Play and copy depend on it. `off` disables it. |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | `transcoding.keyframe_backfill_batch` | server default | Files per pass (> 0). |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | `transcoding.keyframe_backfill_pause_ms` | server default | Milliseconds between files (>= 0). |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | `transcoding.keyframe_backfill_idle_sec` | server default | Seconds between passes when idle (> 0). |
| `EVEY_LEVEL_BACKFILL` | `transcoding.level_backfill` | on | Reads the H.264/HEVC level of files scanned before it was collected. `off` disables it. |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec` | server default | Same pacing, for this pass. |
| `EVEY_SNAPSHOT_FASTPATH` | `transcoding.snapshot_fastpath` | on | Extracts thumbnails with the fast in-process decoder. `off` uses the exact ffmpeg extractor only, to rule it out when a thumbnail shows the wrong frame. |
| `EVEY_TITLE_PROBE` | `transcoding.title_probe` | on | Probes titles in the background at low priority, paused during playback. `off` disables it. |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
