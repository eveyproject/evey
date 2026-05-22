//go:build !linux

package main

import "errors"

// The launcher only ever runs inside the Linux server image. These stubs let
// the package cross-compile (and `go vet ./...`) on non-Linux dev machines.

func applyLandlock(config) error { return errors.New("landlock: linux only") }

func applyRlimits(config) {}
