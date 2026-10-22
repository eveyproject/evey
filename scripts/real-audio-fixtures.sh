#!/usr/bin/env bash
# real-audio-fixtures.sh - cut one case for the real-film ac3go test
# (frontend/apps/web/src/features/playback/wasmaudio/ac3Worker.real.test.ts).
#
# Usage, where ffmpeg and the library are (the evey-server container):
#   scripts/real-audio-fixtures.sh <film> <audio-track> <start-sec> <seconds> <out-dir>
# writes <out-dir>/a_000.m4s ... (the segments under test), ref.f32 (ffmpeg,
# DRC off) and ref_drc.f32 (ffmpeg, DRC on), float32 interleaved in ffmpeg's
# channel order.
#
# The segments are the server's own recipe (hls_audio_only.go): the track
# copied into CMAF. The reference is NOT decoded from those segments alone.
# ffmpeg's AC-3/E-AC-3 decoder corrupts the first block of a session where the
# channels switch transforms differently (it starts with s->downmixed = 1 and
# "upmixes" delays that were never downmixed: the front left survives, the
# others do not). A reference whose session starts at the window carried that
# block whenever the window held one, and charged ac3go with it: 10.4 % on
# the 7.1 front right of Avatar: Fire and Ash, from one block (2026-10-22,
# confirmed by liba52, which decodes that block like ffmpeg's longer session).
# So the reference is decoded from the start of the film, as a player starting
# it would, then cut to the window to the sample; the window, decoded alone,
# is compared against it block by block and any block the short session got
# wrong is named.
set -euo pipefail

film=$1 track=$2 start=$3 dur=$4 out=$5
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$out"

codec=$(ffprobe -v error -select_streams "a:$track" -show_entries stream=codec_name -of csv=p=0 "$film")
case $codec in
  ac3 | eac3) ;;
  *) echo "audio track $track is $codec, not AC-3/E-AC-3" >&2; exit 1 ;;
esac
channels=$(ffprobe -v error -select_streams "a:$track" -show_entries stream=channels -of csv=p=0 "$film")

ffmpeg -v error -y -ss "$start" -i "$film" -t "$dur" -map "0:a:$track" -c:a copy \
  -f hls -hls_time 6 -hls_list_size 0 -hls_playlist_type vod -hls_segment_type fmp4 \
  -hls_fmp4_init_filename init.mp4 -hls_segment_filename "$tmp/a_%03d.m4s" "$tmp/a.m3u8"
cp "$tmp"/a_*.m4s "$out"/

# The window as a raw stream, and the track from its first frame to past the
# window, raw too: the window's bytes are found in it to place it exactly.
cat "$tmp/init.mp4" "$tmp"/a_*.m4s > "$tmp/win.mp4"
ffmpeg -v error -y -i "$tmp/win.mp4" -c:a copy -f "$codec" "$tmp/win.raw"
end=$(python3 -c "print(int($start + $dur + 10))")
ffmpeg -v error -y -i "$film" -t "$end" -map "0:a:$track" -c:a copy -f "$codec" "$tmp/head.raw"

ffmpeg -v error -y -drc_scale 0 -i "$tmp/head.raw" -c:a pcm_f32le -f f32le "$tmp/head.f32"
ffmpeg -v error -y -i "$tmp/head.raw" -c:a pcm_f32le -f f32le "$tmp/head_drc.f32"
# Deterministic noise for the block comparison only (the references keep the
# decoder's default, as the test compares against it).
ffmpeg -v error -y -cons_noisegen 1 -i "$tmp/head.raw" -c:a pcm_f32le -f f32le "$tmp/head_c.f32"
ffmpeg -v error -y -cons_noisegen 1 -i "$tmp/win.raw" -c:a pcm_f32le -f f32le "$tmp/win_c.f32"

python3 - "$tmp" "$out" "$channels" <<'EOF'
import os, sys
tmp, out, ch = sys.argv[1], sys.argv[2], int(sys.argv[3])
head = open(f"{tmp}/head.raw", "rb").read()
win = open(f"{tmp}/win.raw", "rb").read()
off = head.find(win[:65536])
if off < 0 or head[off:off + len(win)] != win:
    sys.exit("the window's frames are not found in the track")
fbytes = os.path.getsize(f"{tmp}/win_c.f32")
frames = fbytes // (4 * ch)
aus = frames // 1536
# Constant-size access units (AC-3 and E-AC-3 as muxed here): the byte offset
# is a whole number of them, or the placement is wrong.
per_au = len(win) / aus
if abs(off / per_au - round(off / per_au)) > 1e-9:
    sys.exit(f"window at byte {off} is not on an access unit boundary ({per_au} bytes each)")
first = round(off / per_au) * 1536 * ch * 4

def cut(src, dst):
    with open(src, "rb") as f:
        f.seek(first)
        data = f.read(fbytes)
    if len(data) != fbytes:
        sys.exit(f"{src}: short read")
    open(dst, "wb").write(data)

cut(f"{tmp}/head.f32", f"{out}/ref.f32")
cut(f"{tmp}/head_drc.f32", f"{out}/ref_drc.f32")

import array
a = array.array("f"); a.frombytes(open(f"{tmp}/win_c.f32", "rb").read())
with open(f"{tmp}/head_c.f32", "rb") as f:
    f.seek(first)
    b = array.array("f"); b.frombytes(f.read(fbytes))
step = 256 * ch
bad = []
for blk in range(1, len(a) // step):  # block 0 has no overlap before it in the window
    s = slice(blk * step, (blk + 1) * step)
    if a[s] != b[s]:
        bad.append(blk)
print(f"{aus} access units, window at access unit {round(off / per_au)} of the track")
if bad:
    print("blocks the window's own session decodes differently (now taken from the film's session):",
          ", ".join(f"AU {k // 6} block {k % 6}" for k in bad))
else:
    # Not a proof: had the film no such block before the window, both
    # sessions would get the same one wrong the same way.
    print("the window's own session agrees with the film's on every block")
EOF
ls -la "$out"
