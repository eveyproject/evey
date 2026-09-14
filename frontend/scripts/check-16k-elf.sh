#!/bin/sh
# Refuses any 64-bit Android .so whose ELF LOAD segments are not 16 KiB aligned (Play
# requirement, Android 15+). Targets: directory trees, .apk (also zipalign -P 16), .aab.
#   check-16k-elf.sh <dir|file.apk|file.aab>...
set -eu

PAGE=16384
SDK=${ANDROID_HOME:-$HOME/Applications/Android/Sdk}

readelf_bin=$(command -v readelf || command -v llvm-readelf || true)
if [ -z "$readelf_bin" ]; then
  ndk=$(ls -d "$SDK"/ndk/* 2>/dev/null | tail -1 || true)
  [ -n "$ndk" ] && readelf_bin="$ndk/toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-readelf"
fi
[ -n "$readelf_bin" ] && [ -x "$readelf_bin" ] || { echo "check-16k-elf: no readelf found" >&2; exit 2; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
: > "$tmp/bad"
checked=0
skipped=0

# check_elf <file> <display name>
check_elf() {
  if "$readelf_bin" -h "$1" | grep -q 'Class:.*ELF32'; then
    skipped=$((skipped+1)); return
  fi
  aligns=$("$readelf_bin" -lW "$1" | awk '$1=="LOAD"{print $NF}' | sort -u)
  if [ -z "$aligns" ]; then
    echo "UNREADABLE  $2 (no LOAD segment)" | tee -a "$tmp/bad"; return
  fi
  for a in $aligns; do
    if [ "$((a))" -lt "$PAGE" ]; then
      echo "UNALIGNED   $2 LOAD align=$a (< $PAGE)" | tee -a "$tmp/bad"; return
    fi
  done
  checked=$((checked+1))
}

# check_tree <root> <display prefix>
check_tree() {
  find "$1" -name '*.so' -type f | sort > "$tmp/list"
  while read -r so; do
    check_elf "$so" "$2${so#"$1/"}"
  done < "$tmp/list"
}

for target in "$@"; do
  case "$target" in
    *.apk|*.aab)
      [ -f "$target" ] || { echo "check-16k-elf: no such file $target" >&2; exit 2; }
      rm -rf "$tmp/x"; mkdir -p "$tmp/x"
      unzip -q -o "$target" '*.so' -d "$tmp/x" 2>/dev/null || true
      check_tree "$tmp/x" "$target!"
      case "$target" in *.apk)
        za=$(command -v zipalign || ls "$SDK"/build-tools/*/zipalign 2>/dev/null | tail -1 || true)
        if [ -n "$za" ]; then
          if ! "$za" -c -P 16 -v 4 "$target" > "$tmp/za" 2>&1; then
            grep -E '\.so' "$tmp/za" | grep -v ' OK' | sed "s|^|ZIPALIGN    $target!|" | tee -a "$tmp/bad"
          fi
        else
          echo "note: zipalign not found, zip offset check skipped for $target"
        fi
      esac
      ;;
    *)
      [ -d "$target" ] || { echo "check-16k-elf: unsupported target $target" >&2; exit 2; }
      check_tree "$target" "$target/"
      ;;
  esac
done

if [ -s "$tmp/bad" ]; then
  echo "check-16k-elf: FAIL - $(grep -c . "$tmp/bad") shared object(s) above are not 16 KiB compatible"
  exit 1
fi
echo "check-16k-elf: OK - $checked 64-bit shared object(s), every LOAD segment aligned on $PAGE ($skipped 32-bit skipped)"
