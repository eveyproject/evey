# ADR-0017: Software HDR→SDR tone-map at delivery resolution, not source

- Status: Accepted

## Context

On a weak NAS (an old-iGPU box: no HEVC hardware decoder, no 10-bit VAAPI
VPP - `scale_vaapi` aborts `-22` on a `p010` surface, so hardware acceleration
is off for this content), playing a 4K HEVC 10-bit HDR title collapsed the ABR
ladder to 360p. A reference open-source server, configured **with hardware acceleration disabled**
(full software) on the *same* box, sustained ~1080p at ~8 Mbit with correct
colours and only minor buffering. So the box is capable; our software pipeline
was simply far less efficient.

Two costs dominated, measured with a micro-benchmark on a synthetic 4K HEVC
Main 10 HDR10 sample (the production container's `ffmpeg`):

1. **Tone-map ran at the source resolution.** The HDR→SDR chain
   (`zscale` linearize → `gbrpf32le` float → `tonemap` → bt709) ran on the full
   3840×2160 frame *before* the downscale - 8.3 M px/frame of float math and
   memory bandwidth on the dominant stage. Folding the resize into the first
   (linearizing) `zscale` so the float pipeline runs at 1080p measured **36 →
   75-83 fps (≈2.1-2.3×)** - and matches the reference scale-before-tonemap order.
2. **Multi-variant fan-out.** One ffmpeg process tone-mapped once at 4K then
   `split` into N parallel `libx264` encodes. Single 1080p vs the 4-rung burst
   measured **67 vs 26 fps (≈2.58×)**.

The calibration bench had also been measuring the slow geometry (tone-map at 4K,
`ultrafast`), so `software_hdr_max_height` did not reflect the chain the runtime
would use after the fix.

This is the **software** counterpart to ADR-0015 (hardware acceleration): when
HW is absent or unusable for a title, the software path must itself be efficient
rather than relied on as a rare fallback.

## Decision

Tone-map at the **delivery resolution**, in linear light, on every software
path:

- `hdr.BuildToneMapFilter` gains `ToneMapOptions.DstWidth`. When > 0 the
  downscale is folded into the first `zscale` (`w=W:h=-2:t=linear`), so the
  float tone-map operates on the delivery-resolution frame. `DstWidth ≤ 0` keeps
  the legacy full-resolution chain for the subtitle burn-in path (the overlay
  must happen at a fixed size before scaling).
- `buildSingleTranscodeArgs` folds the rung width and drops the trailing
  `scale`. `buildMultiVariantArgs` tone-maps the shared feed at the **largest**
  rung width; the per-rung scalers only downscale from there (top rung = no-op).
- The calibration bench (`benchSoftwareHDR`) tone-maps at the ladder-top width
  (same scale-first geometry) and encodes with `-preset veryfast` (was
  `ultrafast`, the runtime preset), so `software_hdr_max_height` re-calibrates
  against the chain the runtime actually runs. **Re-running calibration on the
  target box is required** for the cap to rise from the old (slow-geometry)
  value.
- Encoding a single client-requested rung instead of the parallel ladder removes
  the fan-out multiplier. This is **now the default** (`SingleVariant` defaults to
  true in `TranscodingConfig`); set `EVEY_TRANSCODING_SINGLE_VARIANT=0` to restore
  the legacy multi-variant ladder on a host powerful enough to pre-encode every
  rung. The segment cache is keyed `(hash, variantIdx, segmentIdx)` in both modes,
  so the switch is cache-compatible.

Resizing in linear light (`t=linear`) is colour-correct and arguably higher
quality than scaling in the encoded domain.

## Consequences

- **~2× throughput on the dominant stage**, multiplicative with single-variant -
  enough to lift such a box off the 360p floor toward reference parity (~1080p
  with occasional dips). The hard floor that remains is the **4K HEVC 10-bit
  software *decode*** itself, which touches every pixel once regardless of
  filter order - the reference server has the same limit, hence its minor buffering too.
- **The cap is data-driven, not hand-tuned.** After the bench change, calibration
  must be re-run on the deployment host so `software_hdr_max_height` reflects the
  faster chain; until then the ladder stays capped at the stale value. The cap is
  applied in `profilesForDecision` only when software-tone-mapping
  (`!hwUsable && hdr_tonemapping_needed`), gated on `CapLadderToHeight`.
- **Burn-in is unchanged** (`DstWidth = 0`), so subtitle overlay geometry is
  untouched - the fold is HDR10/HLG-software-path only.
- **Rollback** is the `DstWidth ≤ 0` branch (the original chain) plus leaving the
  single-variant flag off.
- Behaviour is locked by tests at every layer: `BuildToneMapFilter` fold,
  single- and multi-variant arg shape, the `profilesForDecision` HDR cap, and
  `CapLadderToHeight` / `HWCanDecode` (the gates the path turns on).

## Alternatives considered

- **Software-decode + `hwupload` + `h264_vaapi` (offload only the encode).**
  Considered, because the box *does* expose an h264 VAAPI encoder. Rejected for
  this box/release: HW is off and the calibration found no usable HW encode
  ceiling, and the missing decoupled pipeline is a larger change in the vital
  player path; the all-software efficiency fix alone closes the gap to the reference.
  Kept as future work (and the natural complement to ADR-0015).
- **Just raise the resolution cap.** Rejected: dishonest - it would advertise
  rungs the slow pipeline could not sustain, starving the player. The fix is to
  make the pipeline fast *and* re-measure the cap against it.
- **Per-title content-class ladders.** Orthogonal and deferred (a separate
  bandwidth optimisation), not a fix for the tone-map cost.

## Update

The `SingleVariant` default was **reversed** after field experience. Contrary to
the Consequences note above, `SingleVariant` now defaults to **false**
(`internal/config/config.go`: `envBool("EVEY_TRANSCODING_SINGLE_VARIANT",
false)`): multi-variant - which shares ONE decode across every rung - is the
stable default for the weak-NAS target, and single-variant is opt-in via
`EVEY_TRANSCODING_SINGLE_VARIANT=1` on hosts powerful enough to re-decode per
rung. On a decode-bound box, single-variant re-decodes the 4K source for each
rung and thrashes down to the 360p floor, so it is exactly the wrong default
there. The tone-map-at-delivery-resolution decision itself (the core of this
ADR) is unchanged; only the ladder-mode default flipped.
