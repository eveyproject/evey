# ADR-0015: Hardware-accelerated transcoding — one full-HW image, runtime-injected drivers, admin-selectable mode

- Status: Accepted
- Date: 2026-06-01

## Context

Evey transcodes media to an HLS ABR ladder. The software (libx264) path is the
portable floor; on capable hosts a GPU encoder (NVENC, VAAPI, QSV) cuts CPU cost
dramatically. We need this to work across wildly different deployments — a weak
NAS with an Intel iGPU (QSV/VAAPI), a workstation with an NVIDIA card (NVENC),
and a software-only Raspberry Pi — without shipping a different artifact per
host, and without an operator having to hand-pick ffmpeg flags.

Earlier we had **per-backend images**: a CUDA-based `runtime-nvidia` and an
Ubuntu `runtime-vaapi`, run as two sibling dev containers. That was wrong on
three counts:

1. **Image sprawl.** Two HW images to build, ship, and keep in sync, plus the
   software base — three artifacts for one product.
2. **Shared-state bleed.** The two containers shared one DB and stepped on each
   other's persisted encode capability (the calibration cache), causing 30 s
   segment-timeout loops until a per-container backend pin was bolted on.
3. **It's not how the field does it.** Jellyfin/Plex/Emby ship ONE image and
   inject the GPU driver at runtime.

We also learned (the hard way, on the WSL2 dev box) that VAAPI/QSV through
Mesa's experimental **d3d12** translation driver **cannot sustain Evey's
multi-variant encode** — a single `ffmpeg` process running `split=3 → 3×
scale_vaapi → 3× h264_vaapi` segfaults in `d3d12_drv_video.so`, and even three
separate single-output processes crash. NVENC (a real CUDA passthrough) is
unaffected. This is a WSL/d3d12 limitation, not an Evey bug, and it is
officially unsupported by Jellyfin/Plex/Emby alike.

## Decision

### 1. One full-HW image; drivers injected at runtime

A single glibc runtime image (`Dockerfile` target `runtime-hw`, Ubuntu) whose
**distro ffmpeg already has `nvenc + vaapi + qsv` compiled in**. The image bakes
**no GPU driver**. Drivers come at run time, exactly like Jellyfin:

- **NVENC** — the NVIDIA Container Toolkit injects the driver when a GPU is
  reserved (`--gpus all` / a compose `devices: [{driver: nvidia}]`). The image
  sets `NVIDIA_DRIVER_CAPABILITIES=all`; without `video` in that capability set
  the toolkit injects `nvidia-smi`/CUDA but **not** `libnvidia-encode.so`, and
  NVENC fails with "Cannot load libnvidia-encode.so.1" even though `nvidia-smi`
  works. This was the one non-obvious knob.
- **VAAPI / QSV** — a mapped `/dev/dri` (the host's Mesa/iHD driver). `vainfo`,
  `mesa-va-drivers`, and `libva` are installed for it. `LIBVA_DRIVER_NAME` is
  **not** baked: a native host auto-selects radeonsi/iHD/i965 from the GPU PCI
  id; only the WSL2 dev box forces `d3d12` from its compose override.

So the same image is software-only, NVENC, or VAAPI/QSV depending purely on what
the runtime maps. The CGO-free `evey` binary is unchanged across all of it; only
`ffmpeg` and the Landlock shim are glibc.

### 2. Capability is auto-calibrated, then admin-selectable

At boot a benchmark probes the host and persists an `EncodeCapability` (software
ceiling + the detected HW backend + per-codec support). The operator overrides
the policy from the admin panel via a single setting, `transcoding.hw_mode`:

- `auto` (default) — calibration picks the best detected backend.
- `off` — software only, **even if a usable GPU is present** (the escape hatch
  for a flaky/TDR-prone driver, or a deliberate CPU-only choice).
- `nvenc` / `vaapi` / `qsv` — force that backend; if it turns out unusable here,
  calibration strips it back to software and the panel shows the active backend.

Changing the mode re-benches live (no restart). `resolveHW` and the segment
cache are untouched — the calibrated capability stays the single source of
truth, so the cache stays encoder-agnostic and a software fallback slots into
the same manifest. The admin UI is a master "Hardware acceleration" toggle
(off = software) plus, when on, a backend picker.

### 3. Runtime safety nets keep playback alive when HW misbehaves

HW can fail at play time even after a clean bench (driver regression, TDR,
thermal). Two nets, both demoting to software transparently (the cache is
encoder-agnostic, so the client's automatic retry re-resolves the lower tier):

- **Failure classifier + zero-output demotion** — a HW session that exits
  (cleanly or via crash) without producing a segment is treated as a dead
  backend and demoted (`hw_init`, persisted).
- **Fast-fail watchdog** — a HW session that writes no fMP4 init segment within
  `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` (default 5 s) is killed + demoted,
  so a wedged GPU falls back to software in ~5 s instead of the ~25 s segment
  timeout. The init (not a full segment) is the liveness signal: a working-but-
  slow GPU writes it within ~1–3 s, a dead one never does, so 5 s does not
  false-fail a slow weak-NAS encoder.

### 4. WSL2 VAAPI/QSV is detected and greyed, not "fixed"

`sysinfo.IsWSL()` (from `/proc/version`) flags the environment. The admin GET
surfaces `is_wsl`, and the UI greys VAAPI/QSV with a "not supported on WSL" note.
We do not try to work around the d3d12 driver: we tested the documented ffmpeg
mitigations (enlarged surface pool + `async_depth`, a system-memory
`hwdownload→split→hwupload` rewrite, and per-variant processes) — all still
segfault. WSL HW = NVENC only; validate VAAPI/QSV on a native Linux host.

## Consequences

- **One artifact** for every backend; no per-GPU rebuild, no sibling-container
  cache bleed (the backend pin from the interim design is subsumed by the mode).
- Operators get a Jellyfin-grade "pick your acceleration" control with a real
  software escape hatch, and playback survives a broken GPU by design.
- The dev compose runs ONE `evey-hw` container that maps both the NVIDIA GPU and
  `/dev/dri`, defaulting to NVENC; the Alpine Air `evey` container stays
  software-only for code hot-reload + the musl plugin sandbox.

## Per-variant HW path (implemented, opt-in)

By default Evey transcodes the **whole ABR ladder in one ffmpeg process**
(`split` → N encoders) — efficient (decode once, encode N) and rock-solid on
software, but HW-fragile: a multi-encode graph stresses even native VAAPI
drivers under load (intel/media-driver #207, #1342) and hard-crashes the d3d12
shim. Jellyfin/Plex/Emby instead run **one ffmpeg per variant** (single output).

`EVEY_TRANSCODING_HW_PER_VARIANT=true` switches the **HW** path to that model:
one single-output ffmpeg per rung behind a composite handle, while software
keeps the single-process graph. This avoids the multi-encode-in-one-process
graph that crashes fragile VA drivers and isolates per-rung failures. It is
encapsulated in the ffmpeg adapter — the orchestrator (sessions, throttle,
teardown, demotion, watchdog, concurrency slots) is unchanged. **Off by default**
(the multi-output path works on NVENC and the change is player-vital); flip the
flag to enable. Validated on NVENC (3 rungs, each its own ffmpeg, no `split`);
VAAPI/QSV to be validated on real hardware. It does **not** rescue WSL2 d3d12 (a
dead end). Cost: N× decode (cheap with HW decode) and N concurrent encode
sessions (mind the NVENC session cap on consumer cards).

Remaining future work: route per-variant on demand at the **orchestrator** level
(spawn only the rung the client actually requests) for the full "only encode
what's watched" win — the current composite still spawns all N rungs up front.
