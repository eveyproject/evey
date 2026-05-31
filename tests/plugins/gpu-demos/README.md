# GPU plugin demos — legit + red-team

Six installable example plugins exercising the `hardware.gpu` permission and its
hardening (see `docs/plugins.md` and the security audit in memory
`gpu-plugin-security`). Pre-zipped in `_zips/` for installing through the admin
plugin UI; the behaviour is also locked by `TestRedTeamSandboxEscape`
(`cryptominer-gpu` + `cryptominer-gpu-granted`).

`hardware.gpu` is **admin-only, deny-by-default**, and grants the **DRM render
node alone** (`/dev/dri/renderD128`) — never the card/control node, `/dev/dxg`,
`/dev/nvidia*` or `/dev/kfd`. It is for **first-party / trusted plugins only**:
render access is a per-GPU, not per-tenant, boundary (residual risk: cross-process
GPU-memory leak on AMD/Apple/Qualcomm, local GPU DoS, kernel driver ioctl surface).

To meaningfully test the GPU paths, run on the hardware-transcode dev box where a
render node is mapped in (the dev `evey` service maps `/dev/dri`; needs
`sudo modprobe vgem` on WSL2). Watch the plugin logs and `docker compose logs`.

## Legitimate (well-behaved)
| Plugin | Declares | Does | Expected |
|---|---|---|---|
| `gpu-scene-tagger` | `hardware.gpu` | opens the render node, runs a short **bursty** inference pass (~1.5s), releases it | render node opens; never flagged by the monitor (bursty, not sustained); degrades to CPU if no GPU |
| `gpu-frame-analyzer` | `hardware.gpu` | opens the render node read-only to report GPU availability, exits | render node opens; minimal, immediate release |

## Red-team
| Plugin | Declares | Tries | Expected |
|---|---|---|---|
| `redteam-miner-overt` | (none) | open GPU nodes + CPU mine | **all GPU BLOCKED** (no permission); CPU peg flagged by the monitor |
| `redteam-fake-subtitle` | (none, posing as a "subtitle enhancer") | covertly open GPU + spawn a helper + CPU mine | **disguise defeated**: GPU BLOCKED, spawn BLOCKED (no subprocess), CPU mine flagged |
| `redteam-fake-upscaler` | `hardware.gpu` (posing as an "AI upscaler") | use the granted render node, then **escalate** to card0/dxg/nvidia*/kfd and mine | render node opens (cover); **every escalation BLOCKED**; sustained pegging is the miner signature the monitor kills (a real GPU peg via `drm-engine` fdinfo) |
| `redteam-gpu-escalator` | `hardware.gpu` | widen the single-node grant to card/control/compute nodes, list `/dev`, spawn ffmpeg | render node opens (control); **all widening BLOCKED**; `/dev` listing BLOCKED; ffmpeg spawn BLOCKED (LD_PRELOAD) |

The two **disguised miners** (`redteam-fake-subtitle`, `redteam-fake-upscaler`)
are the supply-chain case: a friendly-looking plugin that turns malicious. The
sandbox defeats the disguise regardless of the manifest text — without
`hardware.gpu` the GPU is fully denied, and even *with* it the grant is the
render node alone and mining is detected/killed.

## Rebuild the zips
```
cd tests/plugins/gpu-demos && for d in */; do [ "$d" = "_zips/" ] && continue; (cd "$d" && zip -q "../_zips/${d%/}.zip" plugin.yaml main.js); done
```
