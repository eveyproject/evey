# Transcoding and hardware

Capacity, graphics card, quality limits and the transcoding cache. Most
installations set none of these: the server measures the machine and picks the
hardware itself (see [Transcoding and server capacity](../playback/transcoding.md)).
Where a setting exists in **Admin → Transcoding**, the value saved there wins;
the table says so.

## Hardware acceleration

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | empty | `auto`, `off`, `nvenc`, `vaapi`, `qsv` or `rkmpp`. Empty: derived from the two variables below. Overridden by **Hardware acceleration** in the panel, which re-measures live. |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | The older on/off switch. Overridden by the panel. |
| `EVEY_TRANSCODING_HW_BACKEND` | empty (detect) | Pins one backend (`nvenc`, `vaapi`, `qsv`): only that one is measured. Lets several Evey containers on one host each use a different GPU path. |
| `EVEY_TRANSCODING_HW_DEVICE` | empty (all) | Pins one device: a render node (`/dev/dri/renderD129`) or a CUDA index (`1`). With `EVEY_TRANSCODING_HW_BACKEND`, binds a container to one GPU on a multi-GPU host. |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | A hardware session that has not produced its first segment in this delay is treated as a stuck GPU and moved to the processor. |
| `EVEY_TRANSCODING_HW_TONEMAP` | on | `0` disables HDR tone mapping on the GPU and uses the processor for it, for a driver known to misbehave. |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | One ffmpeg per quality on the GPU path instead of one process for all: isolates failures, and avoids the multi-output graph that crashes fragile VA drivers. Overridden by **Advanced limits**; read at startup. |

Exposing the GPU to the container: [Hardware acceleration](../administration/hardware-acceleration.md).

## Capacity and limits

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (number of CPUs) | Total concurrent transcoding sessions. On a machine that can barely sustain one 4K HDR software transcode, set `1` or `2`. Overridden by **Advanced limits**; read at startup. |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | Transcode jobs outside live playback. Overridden by **Max Concurrent** in **Admin → Transcode Cache**. |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | Encodes only the quality a viewer is watching. `false` shares one decode between all qualities but encodes those nobody watches; only for a host with no hardware decoding and heavy sources. Overridden by **Advanced limits**; read at startup. |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (measured) | Forces the processor's resolution ceiling instead of the measured one. Overridden by **Advanced limits**; read at startup. |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (measured) | Same, for HDR sources tone-mapped on the processor. |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | no cap | Threads per software encoder (1 to 64). Hardware encoders ignore it. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (follows the source) | A ceiling on the bitrate of transcoded video. Overridden by **Video bitrate ceiling**. |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (none) | The same ceiling for clients outside the home network. Overridden by **Video bitrate ceiling, remote viewers**. |

## Cache

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | The size of the whole transcoding cache (segments, remuxes, prepared downloads), least recently used removed first. 2 GiB are always kept free on the volume. Overridden by **Transcode cache budget** in **Server Settings**; read at startup. |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | Where it lives. Set by the image. |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | Where the seek bar thumbnails live. Set by the image: they are expensive to regenerate, so they stay in `/data`. |

## Calibration clip

The measurement encodes a 4K clip. In order of preference: your file, a clip
downloaded once and checked by its hash, or a synthetic clip. A real clip
measures more honestly than the synthetic one.

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | empty | A clip of yours, HDR and SDR. |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | empty | A clip to download once, over `https`. |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | empty | The SHA256 of that download; required with a URL. |

<!-- sources
internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SoftwareMaxHeightOverride
panel overrides: cmd/evey/main.go - loadOverride
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: cmd/evey/wire.go (apptranscoding.NewService) ; i18n admin.transcodeCache.maxConcurrent
-->
