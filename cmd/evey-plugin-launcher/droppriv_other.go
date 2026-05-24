//go:build !linux

package main

import "errors"

// The launcher only ever runs inside the Linux server image. This stub lets the
// package cross-compile (and `go vet ./...`) on non-Linux dev machines.
func dropPrivileges(uid, _ int) error {
	if uid > 0 {
		return errors.New("privilege drop: linux only")
	}
	return nil
}
