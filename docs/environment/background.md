# Background work

The variables behind the passes the server runs on its own and how they
share the processor. What each pass does and how playback is protected from
it: [Background work and server load](../operations/background-work.md).

## CPU share

The share of the processor background tasks may use is set in **Admin →
Server Config → Processing power**, live, without a restart; it has no
environment variable. The two variables below size a pool by hand and bypass
that share for it.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | from the share | Files analysed in parallel during a scan. | none |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | from the share | Titles analysed at once for intros and credits. | none |

## Passes

Each variable has a key, also accepted as a row in **Admin → Server Config →
Config entries**, which suggests these keys; the row wins over the
environment. The **Background work** card of the same screen sets the main
ones. All are read at startup, so a change applies at the next restart
whichever way it is made.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | on | Pauses background work while a stream plays. `off` keeps it running. | Config entries `transcoding.pause_bg_on_playback`, restart |
| `EVEY_KEYFRAME_BACKFILL` | on | Indexes the keyframes of scanned files and classifies their frame rate. Direct Play and copy depend on it. `off` disables it. | Config entries `transcoding.keyframe_backfill`, restart |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | server default | Files per pass (> 0). | Config entries `transcoding.keyframe_backfill_batch`, restart |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | server default | Milliseconds between files (>= 0). | Config entries `transcoding.keyframe_backfill_pause_ms`, restart |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | server default | Seconds between passes when idle (> 0). | Config entries `transcoding.keyframe_backfill_idle_sec`, restart |
| `EVEY_LEVEL_BACKFILL` | on | Reads the H.264/HEVC level of files scanned before it was collected. `off` disables it. | Config entries `transcoding.level_backfill`, restart |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | server default | Same pacing, for this pass. | Config entries `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec`, restart |
| `EVEY_SNAPSHOT_FASTPATH` | on | Extracts thumbnails with the fast in-process decoder. `off` uses the exact ffmpeg extractor only, to rule it out when a thumbnail shows the wrong frame. | Config entries `transcoding.snapshot_fastpath`, restart |
| `EVEY_TITLE_PROBE` | on | Probes titles in the background at low priority, paused during playback. `off` disables it. | Config entries `transcoding.title_probe`, restart |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
