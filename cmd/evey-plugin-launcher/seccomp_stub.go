//go:build !linux || (linux && !amd64 && !arm64)

package main

import "errors"

// applySeccomp is a no-op on platforms/arches we don't ship a verified BPF
// denylist for (the prebuilt image is linux/amd64 + linux/arm64). It returns an
// error rather than installing nothing silently, so the caller can log it.
func applySeccomp(_, _ bool) error {
	return errors.New("seccomp filter unsupported on this platform/arch")
}
