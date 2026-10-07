# ADR-0016: In-process mkvgo fast-path for the MKV scan prober (ffprobe stays the authority)

- Status: Accepted

> **Update.** The fast-path is now **always on** for both EBML
> (`.mkv`/`.webm`) and ISO-BMFF (`.mp4`/`.m4v`/`.m4a`) containers - including the
> music scan prober (`.m4a` AAC; `.flac`/`.mp3`/ALAC fall through to ffprobe); the
> `EVEY_MKV_FASTPATH` env flag is removed (field-equivalence validated on a real
> library, ffprobe stays the fallback). The flag/default references below are
> historical.

## Context

Every file the library scanner touches is probed for its stream layout (codecs,
languages, default/forced flags, dimensions, channels, colour/HDR). The probe is
done by forking `ffprobe` once per file. `ffprobe` is correct and is our
reference, but for a Matroska container it does far more work than the scan
needs: it opens the file, reads ~1 MB, and for HDR detection parses into the
bitstream. On a re-scan, a repair pass, or a weak NAS over a network mount, that
per-file fork + read dominates the prober phase.

`mkvgo` (`github.com/gravity-zero/mkvgo`) is an in-process Matroska reader. Its
`ReadMeta`/`OpenMeta` (v0.5.0) read **only** the file head - `Info` + `Tracks`,
early-stopping via `SeekHead` before the `Cues`/`Clusters` - at roughly **2 ms
and ~900 bytes per file**, versus ~50 ms and ~1.2 MB for `ffprobe` (measured on
a set of real 4K files). v0.6.0 added
colour/HDR derivation from the codec SPS VUI (`hvcC`/`avcC`/`av1C`) when the
container has no `Colour` element, which is what unblocked HEVC/AV1 HDR
coverage - without it those files had to fall back to `ffprobe`.

The question was how to adopt this without making `ffprobe` correctness optional.

## Decision

Introduce `internal/adapters/driven/prober` - a `library.StreamProber`
**dispatcher** that wraps the `ffprobe` prober:

- For an EBML container (`.mkv`/`.webm` family) it reads metadata in-process via
  `mkvgo` and maps it to a `library.ProbeResult` (`mapper.go`).
- For everything else, and for any `.mkv` it cannot map **field-equivalent** to
  `ffprobe`, it **delegates to the `ffprobe` prober**.

`ffprobe` remains the authority. The fast-path is a fork-free, head-only
**optimization**, never a correctness requirement. The mapper is conservative by
construction: `mapContainer` returns `ok=false` (a control-flow signal, not an
error) - falling back to `ffprobe` - whenever it cannot guarantee equivalence:
nil/zero tracks, unknown track type, an exotic/unmapped codec, missing video
dimensions, or an HDR-capable codec (hevc/av1/vp9) with no colour read. Genuine
I/O errors from `ReadMeta` also fall back. A deferred `recover` at the boundary
absorbs any parse panic on an adversarial container and falls back too.

Wiring (`cmd/evey/wire.go`):

- Only the **library scan path** is wrapped. The music command service keeps the
  bare `ffprobe` prober (it needs per-track audio bitrate, which `mkvgo` does not
  surface).
- The fast-path is gated by the env flag **`EVEY_MKV_FASTPATH`** (default
  **off**). Off = a pure pass-through to `ffprobe` - zero behavioural change.

A `chi`/`cmn`/`yue`/… → `zh` macrolanguage fold was added to the audio-track
resolver so a "zh" audio preference matches identically whether a file was
scanned by `ffprobe` (ISO 639-2 `chi`) or by the mkvgo path (BCP-47 `cmn`).

## Consequences

- **Equivalence is verified, not assumed.** `parity_realfile_test.go`
  (PATH-gated on `EVEY_PARITY_DIR`, no real-file paths committed) maps every
  `.mkv` in a directory through both probers and asserts the decision-affecting
  fields agree. On the reference set every file took the fast-path with all fields equal.
- **Per-stream video bitrate stays `nil`** on the fast-path - but `ffprobe`
  itself reports no video bitrate for MKV, so this is equivalent, not a
  regression. (Audio bitrate can differ in rare edge cases; another reason music
  keeps `ffprobe`.)
- **Gain is real where the prober phase dominates** - re-scan, repair, network
  mounts, weak NAS (~35× faster, ~1000× less I/O per MKV). It is **marginal on
  an initial scan**, which is bottlenecked by TMDB metadata calls, not probing.
- **Rollback is a flag.** Set `EVEY_MKV_FASTPATH=0` (or unset) and restart; the
  scanner is back to pure `ffprobe` with no rebuild.
- HDR derivation lives in the pure `internal/pkg/hdr` helper (`DetectHDR`), not
  in the mapper; the mapper only carries colour metadata across the boundary.

## Alternatives considered

- **Replace `ffprobe` outright.** Rejected: `mkvgo` covers Matroska only, and
  `ffprobe` is the cross-format authority. The dispatcher keeps it as the floor.
- **Wire `mkvgo`'s full `Read`/`Open`.** Rejected: full read traverses
  `Cues`+`Clusters` and is ~360× *slower* than `ffprobe` (measured). Only the
  head-only `ReadMeta` is a win.
- **On by default.** Rejected for the first release: ship behind a flag, validate
  parity in the field, flip the default later.
