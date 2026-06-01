# Transcoding

Evey transcodes media on-the-fly when the client doesn't support the source format.

## How It Works

Evey picks the best playback method automatically:
1. **Direct Play** — format supported, no processing
2. **Remux** — repackage container (MKV → MP4), no re-encoding
3. **Transcode** — re-encode to HLS with adaptive bitrate

## Configuration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

## Hardware Acceleration

Evey ships **one image** whose ffmpeg has NVENC, VAAPI, and QSV all compiled in.
No GPU driver is baked into the image — you inject the driver at runtime by
mapping the device, exactly like Jellyfin. The same image is software-only,
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

You can map both at once (NVIDIA reservation + `/dev/dri`) — the one image
serves whichever backend you then select.

### Enabling it

Hardware acceleration is controlled from **Admin → Server configuration → Encode
calibration**:

- A master **Hardware acceleration** toggle. Off = software only (the escape
  hatch for a flaky GPU/driver), on = use the GPU.
- When on, a **backend** picker: `Auto` (calibration picks the best detected),
  or force `NVENC` / `VAAPI` / `QSV`. If a forced backend turns out unusable it
  falls back to software and the card shows the active backend.

The choice persists and re-benches live (no restart). It can also be seeded from
the environment for headless deploys:

```env
# Either the legacy on/off flag…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …or the unified mode (auto|off|nvenc|vaapi|qsv); the admin override wins.
EVEY_TRANSCODING_HW_MODE=auto
# Fast-fail a wedged GPU to software after this delay (default 5s).
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# Spawn one ffmpeg per ABR rung on the HW path (Jellyfin/Plex model) instead of
# one multi-output process — isolates per-rung failures and avoids the
# multi-encode graph that crashes fragile VA drivers. Off by default; software
# always uses the single-process graph. See docs/adr/0015.
EVEY_TRANSCODING_HW_PER_VARIANT=false
```

### WSL2 caveat

On WSL2 the GPU is reached through Mesa's experimental **d3d12** driver. **NVENC
works** (real CUDA passthrough); **VAAPI/QSV do not** — the d3d12 VA driver
crashes on Evey's multi-variant encode. Evey detects WSL and greys VAAPI/QSV in
the admin. Validate VAAPI/QSV on a native Linux host.

## Verify

Open **Admin → Server configuration → Encode calibration**: it shows the
detected accelerators, the active backend, the measured software/HW ceilings,
and any automatic HW→software fallbacks. A "Recalibrate" button re-runs the
benchmark after a hardware or driver change.
