#!/bin/sh
# gomobile bind of this package into <out> (default ../android/build/gobind): offlineaudio.jar
# + jni/arm64-v8a/libgojni.so. Go, gomobile and the NDK are pinned so a fresh clone rebuilds
# the same binding; the Gradle task bindGo in ../android/build.gradle runs this.
set -eu
cd "$(dirname "$0")"
out=${1:-$(pwd)/../android/build/gobind}

: "${JAVA_HOME:=$(echo "$HOME"/Applications/toolchain/jdk-17*)}"
: "${ANDROID_HOME:=$HOME/Applications/Android/Sdk}"
: "${ANDROID_NDK_HOME:=$ANDROID_HOME/ndk/27.1.12297006}"
export JAVA_HOME ANDROID_HOME ANDROID_NDK_HOME
export GOTOOLCHAIN=go1.26.2
MOBILE=v0.0.0-20260730202154-c700fe717e6e   # golang.org/x/mobile, same as go.mod

command -v go >/dev/null || { echo "refresh-android: go not in PATH (any >= 1.21, pins itself to $GOTOOLCHAIN)" >&2; exit 2; }
[ -x "$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin/clang" ] || { echo "refresh-android: NDK not found at $ANDROID_NDK_HOME" >&2; exit 2; }
[ -x "$JAVA_HOME/bin/javac" ] || { echo "refresh-android: no javac under JAVA_HOME=$JAVA_HOME" >&2; exit 2; }

tools="$out/tools"
mkdir -p "$tools"
CGO_ENABLED=0 GOBIN="$tools" go install "golang.org/x/mobile/cmd/gomobile@$MOBILE" "golang.org/x/mobile/cmd/gobind@$MOBILE"
PATH="$tools:$JAVA_HOME/bin:$PATH"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Go links through NDK clang directly; r27 defaults to 4 KiB LOAD segments, Play wants 16 KiB.
gomobile bind -target=android/arm64 -androidapi 24 -javapkg=com.evey \
  -ldflags '-extldflags=-Wl,-z,max-page-size=16384' \
  -o "$tmp/offlineaudio.aar" .

unzip -q -o "$tmp/offlineaudio.aar" classes.jar jni/arm64-v8a/libgojni.so -d "$tmp/unpack"
rm -rf "$out/jni"
mkdir -p "$out/jni/arm64-v8a"
cp "$tmp/unpack/classes.jar" "$out/offlineaudio.jar"
cp "$tmp/unpack/jni/arm64-v8a/libgojni.so" "$out/jni/arm64-v8a/libgojni.so"
../../../scripts/check-16k-elf.sh "$out/jni"
echo "refreshed: $out/offlineaudio.jar + $out/jni/arm64-v8a/libgojni.so"
