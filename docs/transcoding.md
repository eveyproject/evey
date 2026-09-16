# Transcoding

Evey transcodes media on-the-fly when the client doesn't support the source format.

## How It Works

Evey picks the best playback method automatically:
1. **Direct Play** - format supported, no processing
2. **Remux** - repackage container (MKV → MP4), no re-encoding
3. **Transcode** - re-encode to HLS with adaptive bitrate

## Configuration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv | rkmpp - see Hardware Acceleration below
EVEY_TRANSCODING_HW_MODE=off
```

## Hardware Acceleration

Evey ships **one image** whose ffmpeg has NVENC, VAAPI, and QSV all compiled in.
No GPU driver is baked into the image - you inject the driver at runtime by
mapping the device - no privileged mode needed. The same image is software-only,
NVENC, or VAAPI/QSV depending purely on what you map. See
[ADR-0015](adr/0015-hardware-acceleration.md) for the rationale.

### NVIDIA (NVENC)

Install the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html), then reserve the GPU **and** request the `video` driver capability (without it the toolkit injects `nvidia-smi`/CUDA but not `libnvidia-encode.so`, and NVENC fails to load):

```yaml
services:
  evey:
    environment:
      NVIDIA_DRIVER_CAPABILITIES: all   # must include `video` for NVENC
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

### Intel / AMD (VAAPI, QSV)

Expose the DRM render node; libva auto-selects the right driver (iHD / i965 /
radeonsi) from the GPU's PCI id:

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

You can map both at once (NVIDIA reservation + `/dev/dri`) - the one image
serves whichever backend you then select.

### Enabling it

Hardware acceleration is controlled from **Admin → Server configuration → Encode
calibration**:

- A master **Hardware acceleration** toggle. Off = software only (the escape
  hatch for a flaky GPU/driver), on = use the GPU.
- When on, a **backend** picker: `Auto` (calibration picks the best detected),
  or force `NVENC` / `VAAPI` / `QSV` / `RKMPP`. If a forced backend turns out
  unusable it falls back to software and the card shows the active backend.

**What `Auto` does.** You expose the hardware to the container (`/dev/dri`, the
NVIDIA reservation, the Rockchip MPP node); Evey does the rest, from measurement
only - never from a CPU or GPU name:

1. **Detect**: every DRM render node is probed for VAAPI and, on an Intel node,
   for QSV; every NVIDIA GPU by CUDA index; the Rockchip MPP node. Each encoder
   must pass a real encode before it counts.
2. **Certify**: each detected accelerator walks the encode ladder (720p → 2160p)
   in the runtime's own pipeline shape and keeps the tallest rung it sustains at
   1.2× realtime, then passes a visual-correctness gate. Under realtime at 720p =
   rejected, software only. Every certified backend is kept with its **own**
   ceiling, decode matrix, features and proven routes - a 1080p-only accelerator
   next to a 2160p one serves 1080p ladders, never the other's.
3. **Measure the routes**: for every route each backend proved (SDR, HEVC, AV1,
   GPU tone-map, CPU bridge, HDR passthrough, decoupled decode) and for the
   software routes, a warm production session is measured at the reference
   rungs and at the route's ceiling - so a backend certified only at 720p, or
   the 720p software floor, has its cost too.
4. **Choose per workload**: the resolver picks, per request, the cheapest
   certified backend that proves the route and supports the codec (HDR→SDR can
   land on VAAPI's OpenCL tone-map while HEVC takes a faster QSV on the same box).
   Admission prices the session on that route's measurement against a CPU pool
   and an accelerator pool; a full pool spills a GPU stream to the CPU or sheds.

The software ladder floors at **720p SDR / 480p HDR**: a CPU that clears no rung
at the margin still offers those rungs, admitted one session at a time. Direct
play and copy/remux never take an encode slot and are unaffected by any of this.

The choice persists and re-benches live (no restart). It can also be seeded from
the environment for headless deploys:

```env
# Either the legacy on/off flag…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …or the unified mode (auto|off|nvenc|vaapi|qsv|rkmpp); the admin override wins.
EVEY_TRANSCODING_HW_MODE=auto
# Fast-fail a wedged GPU to software after this delay (default 5s).
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# Spawn one ffmpeg per ABR rung on the HW path (one-encoder-per-rung) instead of
# one multi-output process - isolates per-rung failures and avoids the
# multi-encode graph that crashes fragile VA drivers. Off by default; software
# always uses the single-process graph. See docs/adr/0015.
EVEY_TRANSCODING_HW_PER_VARIANT=false
# Encode only the one ABR rung the client is actually requesting (on-demand,
# one-encoder-per-rung) instead of one ffmpeg producing the whole ladder.
# ON by default (=1). Multi-variant shares one decode across the rungs but
# also encodes every rung the viewer never watches - measured on prod, three
# wasted encodes starved the fourth. The calibration benches the SAME shape the
# runtime runs (one rung per probe in single-variant, the cumulative parallel
# ladder in multi-variant), for the software AND the hardware ladders, so the
# ceilings match what a session actually costs. Set =0 only on a host with no
# hardware decode AND heavy sources, to get the shared decode back.
# Cache-compatible either way.
EVEY_TRANSCODING_SINGLE_VARIANT=1
```

> **Five settings now live in the admin panel** (Admin > Transcoding configuration >
> Advanced limits): `HW_PER_VARIANT`, `SINGLE_VARIANT`, `MAX_SESSIONS`,
> `SOFTWARE_MAX_HEIGHT` and `SOFTWARE_HDR_MAX_HEIGHT`. A value saved there is
> persisted and wins over the environment, so you no longer have to edit a
> compose file and recreate the container to change a playback ceiling. All five
> are read at boot, and the panel says so: saving stores your choice, restarting
> applies it.

### Encode calibration sample

At boot (and on `POST /api/v1/admin/transcode/calibration/recalibrate`) the bench
measures the host's realtime encode ceiling against a 4K clip, separately for
SDR and HDR (`software_max_height` / `software_hdr_max_height`). The clip's
decode + tonemap + encode cost is what the cap is derived from, so a
**real-content clip calibrates far more honestly than a synthetic one** - a
synthetic over-states cost and pins the cap low (the classic symptom: a 4K HDR
title that the box can actually play at 1080p gets capped to 480p/360p).

The bench resolves its input in priority order:

1. **Operator file** - a real clip you drop in. Used verbatim.
2. **Pinned download** - a license-clean reference clip fetched once (SHA256-verified), cached.
3. **Synthetic** - generated on the fly (`testsrc2` + light grain at ~5 Mbit/s). Always available; the airgapped fallback.

```bash
# Point the bench at a real file (yours, or a downloaded reference clip):
EVEY_BENCH_HDR_SAMPLE=/data/bench/my-4k-hdr.hevc
EVEY_BENCH_SDR_SAMPLE=/data/bench/my-4k-sdr.hevc
# …or a pinned download (URL must be https; SHA256 is mandatory when URL is set):
EVEY_BENCH_HDR_SAMPLE_URL=https://example.org/evey-bench-hdr10.hevc
EVEY_BENCH_HDR_SAMPLE_SHA256=<sha256>
EVEY_BENCH_SDR_SAMPLE_URL=https://example.org/evey-bench-sdr.hevc
EVEY_BENCH_SDR_SAMPLE_SHA256=<sha256>
```

Build the reference clips from a permissively-licensed source (Blender Open
Movies, CC-BY - **not** CC-BY-NC-ND content like Netflix "Sol Levante") with
`scripts/make-bench-sample.sh <source-4k>`; it prints the SHA256 to pin.

### WSL2 caveat

On WSL2 the GPU is reached through Mesa's experimental **d3d12** driver. **NVENC
works** (real CUDA passthrough); **VAAPI/QSV do not** - the d3d12 VA driver
crashes on Evey's multi-variant encode. Evey detects WSL and greys VAAPI/QSV in
the admin. Validate VAAPI/QSV on a native Linux host.

### HDR passthrough and HDR10+

On a GPU probed for Main10 encoding (`hw_hdr10_encode`), an HDR source played by
an HDR-capable client that decodes HEVC is re-encoded as HEVC Main10 with the
HDR kept: the static HDR10 signalling (bt2020 / PQ or HLG, mastering display
SEI) is carried through. HDR10+ dynamic metadata is **not** re-emitted, so an
HDR10+ title plays as plain HDR10 on that path. Dolby Vision profile 7 falls
back to its HDR10 base layer the same way; profile 5 is tone-mapped to SDR.

Audio sample rate is never a reason to re-encode: a 96 kHz track the client's
codec list accepts is copied as-is. A client whose decoder stops at 48 kHz has
to say so through its codec declaration, the server does not assume it.

## Browser AC-3 / E-AC-3 audio (WebAssembly decoder)

Most browsers cannot decode AC-3/E-AC-3 natively (licensing), which classically
forces an audio re-encode on the server. Evey instead ships a WebAssembly
decoder to the browser: the master playlist goes video-only, the AC-3 track is
served as an out-of-band rendition (copied, not re-encoded), and the client
decodes it to PCM clock-locked to the video. Zero server CPU spent on the
audio when the source packages verbatim.

- Browsers that DO decode AC-3 natively (Safari-class) are excluded by
  capability detection, not by name - they get regular passthrough.
- **Reverse-proxy requirement**: the CSP reaching the browser must include
  `'wasm-unsafe-eval'` in `script-src`, or the decoder cannot compile and the
  film plays muted. See [reverse-proxy.md](reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Verify

Open **Admin → Server configuration → Encode calibration**: it shows the
detected accelerators, the active backend, the measured software/HW ceilings,
and any automatic HW→software fallbacks. A "Recalibrate" button re-runs the
benchmark after a hardware or driver change.
