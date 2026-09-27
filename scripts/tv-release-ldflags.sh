#!/bin/sh
# Prints the -ldflags stamping the TV app build released with this server
# (cmd/evey tvVersion / tvVersionCode), read from the TV app.json. The server
# advertises it on /healthz so a TV can tell it is behind: the Play Store
# auto-update rarely runs on a television. Prints nothing when the file or a
# value is missing - the server then advertises no TV release, never a guess.
# POSIX sh + sed on purpose: runs on the host (Makefile) and in the Go builder.
f="${1:-frontend/apps/tv/app.json}"
[ -r "$f" ] || exit 0
v=$(sed -n 's/^ *"version": *"\([0-9A-Za-z.+-]*\)".*/\1/p' "$f" | head -n 1)
c=$(sed -n 's/^ *"versionCode": *\([0-9][0-9]*\).*/\1/p' "$f" | head -n 1)
[ -n "$v" ] && [ -n "$c" ] || exit 0
printf -- '-X main.tvVersion=%s -X main.tvVersionCode=%s\n' "$v" "$c"
