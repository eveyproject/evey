#!/bin/sh
# Lint gate companion: the rules-of-hooks pass (package.json "lint") is only as
# good as the absence of suppressions - a `// eslint-disable-next-line
# react-hooks/rules-of-hooks` hides a real conditional hook from it (three of
# them sat on a mobile home row until 2026-09-01). The fix for a hook below an
# early return is a dispatch component + a hook-bearing sub-component, never a
# comment. POSIX sh + busybox find/grep on purpose (runs inside evey-web).
hits=$(find apps packages -path '*/node_modules' -prune -o \( -name '*.ts' -o -name '*.tsx' \) -print0 \
  | xargs -0 grep -En 'disable[^[:cntrl:]]*rules-of-hooks' 2>/dev/null)
if [ -n "$hits" ]; then
  echo "$hits"
  echo "rules-of-hooks suppressions are forbidden - split the component instead"
  exit 1
fi
echo "hooks: no rules-of-hooks suppression"
