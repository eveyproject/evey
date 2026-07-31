#!/bin/sh
# Regenerate the Android binding after a Go change: gomobile bind to a temp
# AAR, then unpack it into the two shapes an Android LIBRARY module is allowed
# to consume (a local .aar dependency is refused by AGP): the classes jar and
# the JNI shared object.
set -eu
cd "$(dirname "$0")"

: "${JAVA_HOME:=$(echo "$HOME"/Applications/toolchain/jdk-17*)}"
: "${ANDROID_HOME:=$HOME/Applications/Android/Sdk}"
: "${ANDROID_NDK_HOME:=$ANDROID_HOME/ndk/27.1.12297006}"
export JAVA_HOME ANDROID_HOME ANDROID_NDK_HOME
PATH="$JAVA_HOME/bin:$PATH:$HOME/go/bin"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

gomobile bind -target=android/arm64 -androidapi 24 -javapkg=com.evey \
  -o "$tmp/offlineaudio.aar" .

unzip -q -o "$tmp/offlineaudio.aar" classes.jar jni/arm64-v8a/libgojni.so -d "$tmp/unpack"
mkdir -p ../android/libs ../android/src/main/jniLibs/arm64-v8a
cp "$tmp/unpack/classes.jar" ../android/libs/offlineaudio.jar
cp "$tmp/unpack/jni/arm64-v8a/libgojni.so" ../android/src/main/jniLibs/arm64-v8a/libgojni.so
echo "refreshed: android/libs/offlineaudio.jar + android/src/main/jniLibs/arm64-v8a/libgojni.so"
