//go:build linux

package main

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestIsRenderNode(t *testing.T) {
	good := []string{"/dev/dri/renderD128", "/dev/dri/renderD129", "/dev/dri/renderD256"}
	for _, p := range good {
		if !isRenderNode(p) {
			t.Errorf("isRenderNode(%q) = false, want true", p)
		}
	}
	// Anything that is not a render node must be refused — especially the
	// privileged card/primary node, the WSL and NVIDIA nodes, and traversal.
	bad := []string{
		"/dev/dri/card0", "/dev/dri/card1", "/dev/dri/controlD64",
		"/dev/dxg", "/dev/nvidia0", "/dev/nvidiactl", "/dev/kfd",
		"/dev/dri/renderD", "/dev/dri/renderDabc", "/dev/dri/renderD128/../card0",
		"/dev/null", "", "/dev/dri", "renderD128",
	}
	for _, p := range bad {
		if isRenderNode(p) {
			t.Errorf("isRenderNode(%q) = true, want false (non-render or non-canonical path)", p)
		}
	}
}

func TestRenderDeviceAccessGrantsIOCTLOnlyAtABI5(t *testing.T) {
	// IOCTL_DEV (DRM command submission) must be granted on the render node at
	// ABI >= 5, and must NEVER appear in the safe-pseudo-device mask.
	for abi := 1; abi <= 6; abi++ {
		render := renderDeviceAccess(abi)
		dev := deviceAccess(abi)

		if dev&uint64(unix.LANDLOCK_ACCESS_FS_IOCTL_DEV) != 0 {
			t.Errorf("abi %d: deviceAccess() leaked IOCTL_DEV to safe pseudo-devices", abi)
		}
		wantIoctl := abi >= 5
		gotIoctl := render&uint64(unix.LANDLOCK_ACCESS_FS_IOCTL_DEV) != 0
		if gotIoctl != wantIoctl {
			t.Errorf("abi %d: renderDeviceAccess IOCTL_DEV = %v, want %v", abi, gotIoctl, wantIoctl)
		}
		// Both masks always carry read+write file access.
		const rw = uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE)
		if render&rw != rw&supportedFSAccess(abi) {
			t.Errorf("abi %d: renderDeviceAccess missing read/write", abi)
		}
	}
}
