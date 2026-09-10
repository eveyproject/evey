#!/usr/bin/env sh
# Web bundle budget: the built app must stay under a total and a largest-chunk
# size so a dependency creep (a second hls.js, a duplicated i18n bundle) fails
# the build instead of slowing every first paint.
set -eu
DIST="${1:-frontend/apps/web/dist}"
MAX_TOTAL_MB="${BUNDLE_MAX_TOTAL_MB:-10}"
MAX_CHUNK_MB="${BUNDLE_MAX_CHUNK_MB:-3}"
[ -d "$DIST" ] || { echo "bundle budget: $DIST missing (build first)"; exit 1; }
total_kb=$(du -sk "$DIST" | cut -f1)
largest=$(find "$DIST" -type f -name '*.js' -printf '%s %p\n' | sort -n | tail -1)
largest_b=${largest%% *}
echo "bundle budget: total=$((total_kb/1024)) MB (max $MAX_TOTAL_MB), largest chunk=$((largest_b/1048576)) MB (max $MAX_CHUNK_MB) ${largest#* }"
[ "$total_kb" -le "$((MAX_TOTAL_MB*1024))" ] || { echo "bundle budget: total over budget"; exit 1; }
[ "$largest_b" -le "$((MAX_CHUNK_MB*1048576))" ] || { echo "bundle budget: largest chunk over budget"; exit 1; }
