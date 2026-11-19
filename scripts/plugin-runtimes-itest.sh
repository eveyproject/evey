#!/bin/sh
# Starts the reference plugins of a runtime added to the image (a .jar by
# "java -jar", a .dll by "dotnet") through the real runner, in every sandbox
# mode the host allows: pluginrunner's TestRuntimeLaunchers, in the image of
# internal/adapters/driven/pluginrunner/testdata/runtimes/Dockerfile.
# As in production: the sandbox library is root-owned under /app, the server
# (here the test) runs unprivileged.
set -eu
repo=$(cd "$(dirname "$0")/.." && pwd)
image=evey-plugin-runtimes-itest
docker build -q -t "$image" --build-arg BASE="${GO_CI_IMAGE:-golang:1.27-trixie}" \
  "$repo/internal/adapters/driven/pluginrunner/testdata/runtimes" >/dev/null
mkdir -p "$HOME/.cache/evey-gocache" "$HOME/go/pkg/mod"
# Unconfined, so bubblewrap can run where the host allows user namespaces;
# Landlock and basic need nothing.
exec docker run --rm --security-opt seccomp=unconfined --security-opt apparmor=unconfined \
  -v "$repo":/src -w /src \
  -v "$HOME/go/pkg/mod":/go/pkg/mod \
  -v "$HOME/.cache/evey-gocache":/gocache \
  -e RUN_UID="$(id -u)" -e RUN_GID="$(id -g)" \
  "$image" sh -ec '
    mkdir -p /app && gcc -shared -fPIC -nostartfiles -o /app/libevey-sandbox.so sandbox/sandbox.c
    exec setpriv --reuid "$RUN_UID" --regid "$RUN_GID" --clear-groups env HOME=/tmp GOCACHE=/gocache GOFLAGS=-buildvcs=false sh -ec "
      go build -o /tmp/evey-plugin-launcher ./cmd/evey-plugin-launcher
      EVEY_PLUGIN_LAUNCHER=/tmp/evey-plugin-launcher EVEY_TEST_RUNTIME_FIXTURES=/fixtures \
        go test ./internal/adapters/driven/pluginrunner/ -run TestRuntimeLaunchers -count=1 -v -timeout=300s
    "'
