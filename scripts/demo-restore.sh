#!/usr/bin/env bash
#
# demo-restore.sh -- put the public demo back to its seeded state, when nobody
# is watching. Optional: the server already resets itself hourly when idle
# (ADR-0031). This script is the heavier tool, a full restore of the data
# directory from a seed archive, for when the in-server reset is not enough.
#
# What it does: asks the server whether anything is playing (the dashboard's
# active-stream count, read with the demo account, which is an administrator),
# and if nothing is, stops the stack, replaces the data directory with the seed
# and starts it again. A daily floor restores regardless, so a demo that is busy
# for eighteen hours straight does not accumulate a day of edits.
#
# Not on logout, on purpose: the account is shared, so a logout-triggered
# restore would eject every other visitor, and closing the tab triggers nothing.
#
# Runs on the host (it needs the Docker daemon). DRY-RUN by default: prints the
# plan and does nothing. Add --apply to run it. Suggested crontab, hourly:
#   17 * * * *  cd /srv/evey && scripts/demo-restore.sh --seed /srv/evey-seed.tar.gz --apply
#
# Usage:
#   scripts/demo-restore.sh --seed <tar.gz> [--data ./evey-data] [--url http://localhost:9090]
#                           [--user demo --password demo] [--floor-hours 24] [--apply]
#
# The seed is a tarball of the data directory taken from a configured instance:
#   docker compose stop evey && tar -czf /srv/evey-seed.tar.gz -C ./evey-data . && docker compose start evey
#
# Exit codes:
#   0  restored, or nothing to do (someone is watching and the floor is not due)
#   2  the server could not be asked (unreachable, login refused) - it does NOT
#      restore blind: a restore under a live viewer is the failure this script
#      exists to avoid
#   3  usage
set -uo pipefail

SEED=""
DATA_DIR="./evey-data"
URL="http://localhost:9090"
USER_NAME="demo"
PASSWORD="demo"
FLOOR_HOURS=24
APPLY=0
STAMP="${DEMO_RESTORE_STAMP:-/var/tmp/evey-demo-last-restore}"

while [ $# -gt 0 ]; do
  case "$1" in
    --seed) SEED="$2"; shift 2 ;;
    --data) DATA_DIR="$2"; shift 2 ;;
    --url) URL="${2%/}"; shift 2 ;;
    --user) USER_NAME="$2"; shift 2 ;;
    --password) PASSWORD="$2"; shift 2 ;;
    --floor-hours) FLOOR_HOURS="$2"; shift 2 ;;
    --apply) APPLY=1; shift ;;
    -h|--help) sed -n '2,32p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 3 ;;
  esac
done

if [ -z "$SEED" ] || [ ! -f "$SEED" ]; then
  echo "--seed <tar.gz> is required and must exist" >&2
  exit 3
fi
if [ ! -d "$DATA_DIR" ]; then
  echo "data directory $DATA_DIR does not exist" >&2
  exit 3
fi

# floor_due: has the daily floor elapsed since the last restore this script did?
floor_due() {
  [ -f "$STAMP" ] || return 0
  local last now
  last=$(stat -c %Y "$STAMP" 2>/dev/null || echo 0)
  now=$(date +%s)
  [ $(( now - last )) -ge $(( FLOOR_HOURS * 3600 )) ]
}

# active_streams prints the dashboard's active-stream count, or nothing when
# the server could not be asked. GetDashboard is one of the reads the demo
# policy keeps open, and the demo account is an administrator.
active_streams() {
  local token body
  body=$(curl -fsS -m 10 -H 'Content-Type: application/json' \
    -d "{\"username\":\"$USER_NAME\",\"password\":\"$PASSWORD\"}" \
    "$URL/api/v1/auth/login") || return 1
  token=$(printf '%s' "$body" | sed -n 's/.*"accessToken":"\([^"]*\)".*/\1/p')
  [ -n "$token" ] || return 1
  body=$(curl -fsS -m 10 -H "Authorization: Bearer $token" "$URL/api/v1/admin/dashboard") || return 1
  printf '%s' "$body" | sed -n 's/.*"activeStreams":\([0-9]*\).*/\1/p'
}

if floor_due; then
  echo "daily floor is due: restoring whether or not someone is watching"
else
  streams=$(active_streams)
  if [ -z "$streams" ]; then
    echo "could not ask $URL whether anything is playing - not restoring blind" >&2
    exit 2
  fi
  if [ "$streams" != "0" ]; then
    echo "$streams active stream(s): nothing to do"
    exit 0
  fi
  echo "nobody is watching"
fi

run() {
  echo "+ $*"
  [ "$APPLY" = 1 ] && "$@"
}

echo "plan: stop the stack, replace $DATA_DIR with $SEED, start the stack"
run docker compose stop
# The seed is the whole directory: everything a visitor changed goes with it.
run find "$DATA_DIR" -mindepth 1 -delete
run tar -xzf "$SEED" -C "$DATA_DIR"
run docker compose up -d
if [ "$APPLY" = 1 ]; then
  touch "$STAMP"
  echo "restored $(date -Is)"
else
  echo "(dry run - add --apply to execute)"
fi
