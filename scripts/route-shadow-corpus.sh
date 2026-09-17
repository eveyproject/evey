#!/usr/bin/env bash
#
# route-shadow-corpus.sh - fill the RouteExpressibility coverage table on a
# staging host, then read the go/no-go (ADR-0030, 6).
#
# Expressibility is deterministic, so leaving the shadow is not a matter of
# weeks in production but of COVERAGE: every {backend, topology, needs} cell
# this host can reach, checked at both sites (the plan resolver and the ffmpeg
# builder) with zero disagreements. Three things fill the table:
#   1. a recalibration - the calibration runs every proven (backend x topology)
#      through the builder on its bench clips (the builder site, plain cells);
#   2. this corpus - master-playlist requests that RESOLVE a plan for a handful
#      of real titles under client profiles chosen to reach each topology (the
#      resolver site; the pre-warm of segment 0 also reaches the builder site
#      on real bytes) - this encodes for real, hence STAGING;
#   3. normal viewing, which adds nothing the two above did not.
#
# Usage:
#   scripts/route-shadow-corpus.sh --url http://staging:9091 --user admin --pass ... \
#       --titles titles.env [--recalibrate] [--sleep 6]
#
# titles.env names one media id per ROLE (a role you have no title for is
# skipped, and its cells will simply stay missing until you add one):
#   H264_SDR=<media id of an H.264 SDR title the client could copy>
#   HEVC_HDR10=<media id of an HDR10 (Main10 PQ) title>
#   GPU_UNDECODABLE=<media id of a VP9 / Hi10P / 4:2:2 title -> the decoupled route>
#   PGS_BURNIN=<media id of a title with a bitmap (PGS/VobSub) subtitle as subtitle track 0>
#   INTERLACED=<media id of a title flagged/idet interlaced (QSV deinterlacer)>
#
# Exit codes: 0 ready_to_leave_shadow, 1 not ready (missing cells or
# disagreements, both printed), 2 usage / unreachable.
#
# What it must NOT do: touch the library, the caches or the capability beyond
# the recalibration you asked for. Run it against a staging host.
set -uo pipefail

URL="" USER="" PASS="" TITLES="" RECAL=0 SLEEP=6
while [ $# -gt 0 ]; do
  case "$1" in
    --url) URL="$2"; shift 2 ;;
    --user) USER="$2"; shift 2 ;;
    --pass) PASS="$2"; shift 2 ;;
    --titles) TITLES="$2"; shift 2 ;;
    --recalibrate) RECAL=1; shift ;;
    --sleep) SLEEP="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$URL" ] && [ -n "$USER" ] && [ -n "$PASS" ] && [ -n "$TITLES" ] || { echo "usage: --url --user --pass --titles titles.env [--recalibrate] [--sleep N]" >&2; exit 2; }
command -v curl >/dev/null && command -v python3 >/dev/null || { echo "curl and python3 are required" >&2; exit 2; }
# shellcheck disable=SC1090
. "$TITLES"

TOKEN=$(curl -sS -X POST "$URL/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"$USER\",\"password\":\"$PASS\"}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("accessToken",""))')
[ -n "$TOKEN" ] || { echo "login failed" >&2; exit 2; }
auth=(-H "Authorization: Bearer $TOKEN")

if [ "$RECAL" = 1 ]; then
  echo "→ recalibration (fills the builder site for every proven backend x topology)"
  curl -sS -o /dev/null -w "  recalibrate: HTTP %{http_code}\n" -X POST "${auth[@]}" "$URL/api/v1/admin/transcode/calibration/recalibrate"
fi

# master <media id> <label> <query>: resolve a plan (and pre-warm segment 0).
master() {
  local id="$1" label="$2" query="$3"
  [ -n "$id" ] || { echo "  skip $label (no title)"; return; }
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' "${auth[@]}" "$URL/api/v1/media/$id/hls/master.m3u8?$query")
  echo "  $label: HTTP $code"
  sleep "$SLEEP" # let the pre-warm reach the builder before the next plan
}

# Client profiles. vd=transcode pins a video TRANSCODE (a copyable H.264 title
# would otherwise be remuxed and reach no GPU route). hdr=1 declares an HDR
# client; without it an HDR title tone-maps (GPU tone-map on VAAPI OpenCL /
# QSV VPP, the CPU bridge on an NVENC without tonemap_cuda).
web_h264="videoCodecs=h264&audioCodecs=aac&containers=mp4&vd=transcode"
web_hevc="videoCodecs=h264,hevc&audioCodecs=aac&containers=mp4&vd=transcode"
hdr_tv="videoCodecs=h264,hevc&audioCodecs=aac,ac3,eac3&containers=mp4&hdr=1&vd=transcode"

echo "→ corpus"
master "${H264_SDR:-}"        "zero-copy h264"                "$web_h264"
master "${H264_SDR:-}"        "zero-copy hevc (upgrade)"      "$web_hevc"
master "${HEVC_HDR10:-}"      "gpu-tonemap / bridge (SDR client)" "$web_hevc"
master "${HEVC_HDR10:-}"      "passthrough (HDR client)"      "$hdr_tv"
master "${GPU_UNDECODABLE:-}" "decoupled"                     "$web_hevc"
master "${PGS_BURNIN:-}"      "burn-in (+overlay if probed)"  "$web_h264&subtitleTrack=0"
master "${INTERLACED:-}"      "deint (QSV)"                   "$web_h264"

echo "→ report"
curl -sS "${auth[@]}" "$URL/api/v1/admin/transcode/calibration" | python3 -c '
import json, sys
rep = json.load(sys.stdin).get("expressibility")
if rep is None:
    print("  no expressibility report: the builder port is not wired on this host"); sys.exit(2)
print(f"  expected cells: {len(rep[\"expected\"])}  disagreements: {rep[\"disagreements\"]}")
for c in rep["cells"]:
    print(f"  {c[\"backend\"]:6} {c[\"topology\"]:12} {c[\"needs\"]:16} resolver={c[\"resolver_checks\"]:>3} builder={c[\"builder_checks\"]:>3}"
          + ("  DISAGREE" if c["resolver_disagreements"] or c["builder_disagreements"] else ""))
if rep["missing"]:
    print("  missing (never seen by both sites):")
    for c in rep["missing"]:
        print(f"    {c[\"backend\"]}/{c[\"topology\"]}/{c[\"needs\"]}")
print("  ready_to_leave_shadow:", rep["ready_to_leave_shadow"])
sys.exit(0 if rep["ready_to_leave_shadow"] else 1)
'
