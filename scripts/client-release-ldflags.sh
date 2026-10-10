#!/bin/sh
# Prints the -ldflags stamping the TV and mobile app builds released with this
# server (cmd/evey tvVersion / tvVersionCode / mobileVersion /
# mobileVersionCode), read from each app's app.json. The server advertises
# them on /healthz so an app can tell it is behind: the Play Store auto-update
# rarely runs on a television, and on a phone it waits for an idle device on
# Wi-Fi (10-10, a phone a month on beta.74 with a known silent-DTS bug).
# Prints nothing when a file or a value is missing, or when the mobile
# app.json disagrees with the versionCode Gradle actually builds - the server
# then advertises no release, never a guess. POSIX sh + sed on purpose: runs
# on the host (Makefile) and in the Go builder.
tv="${1:-frontend/apps/tv/app.json}"
mobile="${2:-frontend/apps/mobile/app.json}"
gradle="${3:-frontend/apps/mobile/android/app/build.gradle}"

read_app_json() {
  [ -r "$1" ] || return 1
  v=$(sed -n 's/^ *"version": *"\([0-9A-Za-z.+-]*\)".*/\1/p' "$1" | head -n 1)
  c=$(sed -n 's/^ *"versionCode": *\([0-9][0-9]*\).*/\1/p' "$1" | head -n 1)
  [ -n "$v" ] && [ -n "$c" ]
}

read_app_json "$tv" || exit 0
tv_v=$v; tv_c=$c
read_app_json "$mobile" || exit 0
mobile_v=$v; mobile_c=$c

# The mobile app.json has no say over the APK: Gradle reads its own
# versionCode. The two must agree, or the phone would compare against a
# number nobody shipped and nag forever (or never).
if [ -r "$gradle" ]; then
  g=$(sed -n 's/^ *versionCode *\([0-9][0-9]*\).*/\1/p' "$gradle" | head -n 1)
  if [ -n "$g" ] && [ "$g" != "$mobile_c" ]; then
    echo "client-release-ldflags: $mobile versionCode $mobile_c != $gradle versionCode $g" >&2
    exit 0
  fi
fi

printf -- '-X main.tvVersion=%s -X main.tvVersionCode=%s -X main.mobileVersion=%s -X main.mobileVersionCode=%s\n' \
  "$tv_v" "$tv_c" "$mobile_v" "$mobile_c"
