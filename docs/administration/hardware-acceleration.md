# Hardware acceleration

A graphics card or an integrated GPU lets the server transcode more streams at
once, and higher resolutions, than the processor alone. There is one image for
every case: no driver is baked in for a specific card, and the same container
runs in software only, with an Intel or AMD GPU, or with an NVIDIA card,
depending on the devices you give it.

Two steps: expose the device to the container, then leave **Hardware
acceleration** on **Auto** in **Admin → Transcoding → Encode calibration**.

## Expose the device

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

## What Auto does

With **Auto**, the server works only from measurements, never from the name of
the card:

1. **Detect.** Each render node is probed for VAAPI and, on Intel, Quick Sync;
   each NVIDIA GPU and the Rockchip encoder likewise. An encoder counts only if
   a real encode succeeds.
2. **Certify.** Each accelerator encodes the resolution ladder (720p to 2160p)
   in the same pipeline shape as real playback, keeps the highest rung it
   sustains at 1.2 times real time, and passes a visual check. An accelerator
   that cannot sustain 720p is not used.
3. **Measure the routes.** For each capability the accelerator proved (SDR,
   HEVC, AV1, tone mapping on the GPU, HDR kept), the cost of a real session
   is measured.
4. **Choose per stream.** Each playback goes to the cheapest accelerator that
   has proved what that stream needs, or to the processor. See
   [Transcoding and server capacity](../playback/transcoding.md).

The measurement runs at the server's first quiet moment and again after the
hardware changes. **Recalibrate** runs it immediately: use it after adding a
card or moving the container to another machine.

**Off** keeps every transcode on the processor, which is the escape hatch for a
faulty driver. Choosing a **Backend** by hand (NVENC, VAAPI, QSV, RKMPP)
forces that one; if it turns out unusable, the server falls back to the
processor and the card shows the backend actually in use.

On a machine with several accelerators, Auto has no fixed preference: each is
certified and measured, and every stream goes to the backend that proved the
route it needs, at the best proven quality and the lowest measured cost. Two
streams on the same server can therefore use different backends, for example
HDR tone mapping on one card and an HEVC encode on another.

## Checking that it is used

**Admin → Transcoding → Encode calibration** shows the accelerators detected,
the backend in use, the resolutions certified for the GPU and for the
processor, and any fallback to the processor. During playback, **Admin → Now
Playing** names the backend each stream is encoded with.

Under WSL2, NVENC works; VAAPI and Quick Sync do not (the WSL graphics driver
fails on these encodes), and the admin panel greys them out.

<!-- sources
auto, certify, routes, recalibrate:  docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
render group: docker-compose.gpu.yml
nvidia: docker-compose.gpu-nvidia.yml
labels: i18n admin.calibration.hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, recalibrate ; admin.sessions.track.backend.*
-->
