//go:build linux && !amd64 && !arm64

package main

// Companion stubs for the arches that ship no verified BPF denylist (see
// seccomp_stub.go). TestMain lives in a `linux`-wide file and dispatches on
// forkProbeArg, so both symbols must exist everywhere that file compiles.
const forkProbeArg = "-evey-fork-probe"

func forkProbeMain() int { return 0 }
