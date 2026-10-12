# Hardware acceleration

**In short:**

- A graphics card, or the graphics part built into many processors (Intel
  Quick Sync, for example), lets the server convert more videos at once, and
  at higher resolutions, than the processor alone.
- It only helps when a video is converted (transcoded). Films sent as they
  are, or repackaged, do not use it.
- What you do: add a few lines to your compose file so that Evey can use the
  card, then leave the setting on **Auto (best available)**.
- Evey tests the card with a real conversion before sending any playback to
  it. A card that fails the test is not used.

The steps follow, then how the server proves what the card can do.

## Turn it on

1. Open **Admin → Transcoding → Encode calibration**. **Hardware
   acceleration** is on **Auto (best available)** by default, and the
   **Detected:** line names the accelerators the host advertises.
2. If it says **No GPU accelerator detected**, give the device to the
   container (next section) and recreate it.
3. Click **Recalibrate**, or wait: the server measures itself at its first
   quiet moment. The card then shows the backend in use and the resolutions
   certified for the GPU and for the processor.

There is one image for every case: no driver is baked in for a specific card,
and the same container runs in software only, with an Intel or AMD GPU, or
with an NVIDIA card, depending on the devices you give it.

## Give the device to the container

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # the host's render group: getent group render
    ```

    The group must be the host's `render` group (`getent group render`, or
    `stat -c %g /dev/dri/renderD128`). A wrong value does not break anything:
    the GPU is simply not accessible and the server stays on the processor.

=== "NVIDIA (NVENC)"

    Install the NVIDIA Container Toolkit on the host, then reserve the GPU and
    request the `video` capability, without which the encoder library is not
    injected:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # must include "video"
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    Map `/dev/mpp_service`, `/dev/dri` and, where the kernel exposes it,
    `/dev/rga`. See [ARM hosts](../operations/advanced-deployments.md#arm-hosts).

Both an NVIDIA reservation and `/dev/dri` can be mapped at once.

## What "proved by a real encode" means

With **Auto**, the server works only from measurements, never from the name of
the card:

1. **Detect.** Each render node is probed for VAAPI and, on Intel, Quick Sync;
   each NVIDIA GPU and the Rockchip encoder likewise. An encoder counts only
   if a short real encode through it succeeds.
2. **Certify.** Each accelerator encodes the resolution ladder (720p to 2160p)
   in the same pipeline shape as real playback and keeps the highest rung it
   sustains at 1.2 times real time. A few frames of each probe are decoded
   back and compared with the source, because a faulty driver can exit
   cleanly while producing green or garbled frames. An accelerator that
   cannot sustain 720p is not used.
3. **Measure the routes.** For each capability the accelerator proved (SDR,
   HEVC, AV1, tone mapping on the GPU, HDR kept), the cost of a real session
   is measured.
4. **Choose per stream.** Each playback goes to the cheapest accelerator that
   has proved what that stream needs, or to the processor. See
   [Transcoding and server capacity](../playback/transcoding.md).

The measurement waits for a quiet moment: no active stream and the host's CPU
under 20 % for five samples in a row, four seconds apart, so that a busy boot
or a library scan does not pin the ceilings too low. It runs again when the
hardware changes. **Recalibrate** runs it immediately: use it after adding a
card or moving the container to another machine.

!!! note "A cold boot is not a verdict"
    A GPU that fails to initialise in the first seconds after the container
    starts is probed again on a short schedule before anything is recorded.
    Only a failure that stays, or one of a deterministic kind (missing
    encoder, unsupported format), marks the accelerator as unusable.

## Forcing a backend or turning it off

**Off (software only)** keeps every transcode on the processor, which is the
escape hatch for a faulty driver. Choosing a **Backend** by hand (NVENC,
VAAPI, QSV, RKMPP) forces that one; if it turns out unusable, the server falls
back to the processor and the card shows the backend actually in use.

On a machine with several accelerators, Auto has no fixed preference: each is
certified and measured, and every stream goes to the backend that proved the
route it needs, at the best proven quality and the lowest measured cost. Two
streams on the same server can therefore use different backends, for example
HDR tone mapping on one card and an HEVC encode on another.

## Checking that it is used

**Admin → Transcoding → Encode calibration** shows the accelerators detected,
the backend in use, the resolutions certified for the GPU and for the
processor, and under **Hardware fallbacks** the recent encodes that fell back
to the processor with their cause. During playback, **Admin → Now Playing**
names the backend each stream is encoded with.

Under WSL2, NVENC works; VAAPI and Quick Sync do not (the WSL graphics driver
fails on these encodes), and the panel marks them **not supported on WSL**.

<!-- sources
auto, certify, routes, recalibrate: docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
detection by real encode: internal/adapters/driven/ffmpeg/hwdetect.go - probeHWEncode
ladder, 1.2x realtime margin, 720p floor: internal/adapters/driven/ffmpeg/benchmark.go - hwBenchLadder, Margin
visual check: internal/adapters/driven/ffmpeg/hw_output_validation.go
cold-boot init retry: internal/adapters/driven/ffmpeg/hw_readiness.go - runHWReadinessProbe, hwReadinessRetryDelays
quiet moment: internal/app/transcoding/calibration_service.go - idleProbes, idleProbeEvery, idleCPUPercent
render group: docker-compose.gpu.yml ; nvidia: docker-compose.gpu-nvidia.yml
panel: frontend/apps/web/src/features/admin/EncodeCalibrationCard.tsx (hw_mode, detected_backends, is_wsl)
labels: i18n admin.calibration.cardTitle, hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, detected, detectedNone, recalibrate, hwFallbacks, wslUnsupported ; admin.nav.transcodingConfig, nowPlaying
plain opening: rules below ; i18n admin.calibration.hwModeAuto
-->
