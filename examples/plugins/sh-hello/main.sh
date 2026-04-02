#!/bin/sh

PLUGIN_ID="${EVEY_PLUGIN_ID:-sh-hello}"
SOCK="${EVEY_PLUGIN_SOCK:-/tmp/evey/plugins/sh-hello.sock}"

cleanup() {
  echo "[$PLUGIN_ID] SIGTERM received, shutting down" >&2
  rm -f "$SOCK"
  exit 0
}
trap cleanup TERM INT

echo "[$PLUGIN_ID] listening on $SOCK" >&2

while true; do
  sleep 5
done
