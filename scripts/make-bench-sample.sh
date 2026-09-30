#!/usr/bin/env bash
# make-bench-sample.sh - produce the encode-calibration reference clips.
#
# The calibration bench (internal/adapters/driven/ffmpeg/benchmark.go) measures
# the host's realtime encode ceiling against a 4K clip. A real-content clip
# calibrates far more honestly than the synthetic fallback - the synthetic
# over-states decode+encode cost and pins the cap low. This script turns a
# license-clean source into the two reference clips Evey downloads at first
# calibration (SDR + HDR), and prints the SHA256 to pin.
#
# SOURCE - must be permissively licensed so we may re-encode (derivative) and
# redistribute, including in a tool others deploy commercially:
#   * Blender Open Movies (CC-BY 4.0): Tears of Steel, Sintel, Coffee Run, ...
#     https://studio.blender.org/films/  (4K masters under "Download").
#   AVOID CC BY-NC-ND content (e.g. Netflix "Sol Levante", "Meridian"): the NC
#   clause blocks bundling in a commercially-deployable tool and ND blocks the
#   re-encode. Free to *watch* is not free to *redistribute/derive*.
#
# The Blender films are SDR Rec.709; we tag/convert to bt2020/PQ 10-bit for the
# HDR clip. That is fine for calibration: the tonemap COMPUTE cost is
# luminance-independent (per-pixel math), so a bt2020/PQ-tagged real-film clip
# exercises the 10-bit-decode + tonemap + encode path representatively. It is a
# bench fixture, not a reference HDR master.
#
# Usage:  scripts/make-bench-sample.sh <source-4k.mov> [out-dir] [start-seconds]
# Output: <out>/evey-bench-sdr.hevc, <out>/evey-bench-hdr10.hevc + their SHA256.
# Then host both as release assets and pin URL+SHA256 via the env vars in
# docs/playback/transcoding.md, or ship the defaults in cmd/evey/wire.go.
#
# Attribution (CC-BY) goes in NOTICE/THIRD_PARTY when the asset is published.
set -euo pipefail

SRC="${1:?usage: make-bench-sample.sh <source-4k> [out-dir] [start-seconds]}"
OUT="${2:-./bench-samples}"
SS="${3:-60}"          # seek into the content, past any title card
DUR=15                 # 15 s is plenty: the bench reads ~5 s per probe
BV="5M"                # ~typical 4K streaming / "4KLight" bitrate (drives decode cost)
mkdir -p "$OUT"

X265_COMMON="bframes=4:ref=3:b-adapt=2:rc-lookahead=20"

echo "==> SDR reference clip (HEVC Main, 8-bit)"
ffmpeg -hide_banner -nostats -y -ss "$SS" -i "$SRC" -t "$DUR" \
  -vf "scale=3840:-2:flags=lanczos,format=yuv420p" \
  -c:v libx265 -preset medium -x265-params "$X265_COMMON" \
  -b:v "$BV" -maxrate "$BV" -bufsize "$(( ${BV%M} * 2 ))M" \
  -tag:v hvc1 -an "$OUT/evey-bench-sdr.hevc"

echo "==> HDR reference clip (HEVC Main10, bt2020/PQ, 10-bit)"
ffmpeg -hide_banner -nostats -y -ss "$SS" -i "$SRC" -t "$DUR" \
  -vf "scale=3840:-2:flags=lanczos,format=yuv420p10le" \
  -c:v libx265 -preset medium -pix_fmt yuv420p10le \
  -x265-params "${X265_COMMON}:hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc" \
  -color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc \
  -b:v "$BV" -maxrate "$BV" -bufsize "$(( ${BV%M} * 2 ))M" \
  -tag:v hvc1 -an "$OUT/evey-bench-hdr10.hevc"

echo
echo "==> SHA256 to pin (EVEY_BENCH_*_SAMPLE_SHA256):"
for f in "$OUT/evey-bench-sdr.hevc" "$OUT/evey-bench-hdr10.hevc"; do
  printf '  %s  (%s)\n' "$(sha256sum "$f" | cut -d' ' -f1)" "$(basename "$f")"
  ls -lh "$f" | awk '{print "    size:", $5}'
done
