# ADR-0015: One image for every GPU, drivers injected at runtime

- Status: Accepted
- Date: 2026-06-01

## Context

Hardware encoding needs a different driver per vendor. Shipping one image per
backend means building, publishing and keeping several images in sync, and it
makes the operator pick the right one before they know what their host can do.

## Decision

**One image, no driver baked in.** The runtime stage carries an ffmpeg with
NVENC, VAAPI and QSV compiled in, and nothing vendor-specific. Drivers arrive at
run time: the NVIDIA Container Toolkit injects them when a GPU is reserved, and
VAAPI or QSV work off a mapped `/dev/dri`. The same image is therefore
software-only, NVENC or VAAPI/QSV depending purely on what the host maps into it.

**The host is measured, not declared.** A benchmark at boot records what the
machine can actually encode, and `transcoding.hw_mode` in the admin panel
overrides the result: `auto`, `off`, or a forced backend. Changing it re-benches
live, without a restart.

**Hardware that fails at play time falls back to software.** A hardware session
that produces no segment is demoted and the failure is remembered; one that has
not written its init segment within five seconds is killed and demoted. Five
seconds because a working GPU writes it in one to three, and a dead one never
does. The segment cache is encoder-agnostic, so the client's retry lands on the
software tier without any manifest change.

## Consequences

- An operator maps a GPU or does not. There is no image to choose, and no
  configuration to get wrong.
- What a host can do is discovered rather than trusted, so a driver that breaks
  later degrades to software instead of breaking playback.
- `NVIDIA_DRIVER_CAPABILITIES` must include `video`. Without it the toolkit
  injects CUDA but not the encoder library, and NVENC fails while `nvidia-smi`
  works - the one non-obvious knob.
- VAAPI and QSV do not work under WSL2. The admin panel detects it and greys
  them rather than pretending; the documented ffmpeg workarounds were tried and
  still crash. WSL means NVENC or software.
