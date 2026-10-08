# Transcoding and hardware

The variables behind capacity, the graphics card, quality limits and the
transcoding cache. Most installations set none of them: the server measures
the machine and picks the hardware itself, as explained in
[Transcoding and server capacity](../playback/transcoding.md). Where a
setting exists in **Admin → Transcoding**, the value saved there wins.

## Hardware acceleration

Exposing the GPU to the container:
[Hardware acceleration](../administration/hardware-acceleration.md).

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | empty | `auto`, `off`, `nvenc`, `vaapi`, `qsv` or `rkmpp`. Empty: derived from the two variables below. | Transcoding → **Hardware acceleration**, live (re-measures) |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | The older on/off switch. | Transcoding → **Hardware acceleration**, live |
| `EVEY_TRANSCODING_HW_BACKEND` | empty (detect) | Pins one backend (`nvenc`, `vaapi`, `qsv`): only that one is measured. Lets several Evey containers on one host each use a different GPU path. | none |
| `EVEY_TRANSCODING_HW_DEVICE` | empty (all) | Pins one device: a render node (`/dev/dri/renderD129`) or a CUDA index (`1`). With `EVEY_TRANSCODING_HW_BACKEND`, binds a container to one GPU on a multi-GPU host. | none |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | A hardware session that has not produced its first segment in this delay is treated as a stuck GPU and moved to the processor. | none |
| `EVEY_TRANSCODING_HW_TONEMAP` | on | `0` disables HDR tone mapping on the GPU and uses the processor for it, for a driver known to misbehave. | none |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | One ffmpeg per quality on the GPU path instead of one process for all: isolates failures, and avoids the multi-output graph that crashes fragile VA drivers. | Transcoding → Advanced limits → **One hardware encoder process per rung**, restart |

The low-power Intel encoder and AV1 output (switch and ceiling) have no
variable: they are set in **Admin → Transcoding** only.

## Capacity and limits

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (number of CPUs) | Total concurrent transcoding sessions. On a machine that can barely sustain one 4K HDR software transcode, set `1` or `2`. | Transcoding → Advanced limits → **Max concurrent HLS sessions**, restart |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Transcode jobs outside live playback. | Transcode Cache → **Max Concurrent**, live |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Encodes only the quality a viewer is watching. `false` shares one decode between all qualities but encodes those nobody watches; only for a host with no hardware decoding and heavy sources. | Transcoding → Advanced limits → **Encode one ladder rung at a time**, restart |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (measured) | Forces the processor's resolution ceiling instead of the measured one. | Transcoding → Advanced limits → **Software transcode height ceiling**, restart |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (measured) | Same, for HDR sources tone-mapped on the processor. | Transcoding → Advanced limits → **Software HDR height ceiling**, restart |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | no cap | Threads per software encoder (1 to 64; anything else means no cap). Hardware encoders ignore it. | none |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (follows the source) | A ceiling on the bitrate of transcoded video. | Transcoding → **Video bitrate ceiling**, live |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (none) | The same ceiling for clients outside the home network. | Transcoding → **Video bitrate ceiling, remote viewers**, live |

## Cache

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | The size of the whole transcoding cache (segments, remuxes, prepared downloads), least recently used removed first. | Server Config → Server Settings → **Transcode cache budget**, restart |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Where it lives. Set by the image. | none |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Where the seek bar thumbnails live. Set by the image: they are expensive to regenerate, so they stay in `/data`. | none |

## Calibration clip

The measurement encodes a 4K clip. In order of preference: your file, a clip
downloaded once and checked by its hash, or a synthetic clip. A real clip
measures more honestly than the synthetic one. None of these has a panel
equivalent.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | empty | A clip of yours, HDR and SDR. | none |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | empty | A clip to download once, over `https`. | none |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | empty | The SHA256 of that download; required with a URL. | none |

<!-- sources
internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SingleVariant, SoftwareMaxHeightOverride, SoftwareAV1MaxHeightOverride (panel-only), AV1Enabled, AV1MaxHeight, HWLowPowerEnabled
panel overrides: cmd/evey/main.go - loadOverride (transcoding.*) ; cmd/evey/wire.go (transcoding.hw_mode read directly) ; internal/adapters/driving/http/settings_handler.go
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: frontend/apps/web/src/routes/_auth/admin/transcode-cache.lazy.tsx (transcoding.max_concurrent) ; i18n admin.transcodeCache.maxConcurrent
labels: i18n admin.transcoding.advancedTitle, maxSessions, softwareMaxHeight, softwareHDRMaxHeight, singleVariant, hwPerVariant, hwModeLabel, maxBitrateLabel, maxBitrateRemoteLabel ; admin.serverSettings.transcodeCacheMaxGB
-->
