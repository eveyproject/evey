//go:build linux

package main

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

// armParentDeathSignal sets PR_SET_PDEATHSIG(SIGTERM) on the CALLING thread.
// It must run on the thread that will execve (pin with runtime.LockOSThread
// first): the attribute is per-thread, survives execve into the plugin (which
// carries no setuid/file caps), and makes the kernel SIGTERM the plugin when
// the server dies - the same contract the runner declares via SysProcAttr.
func armParentDeathSignal() error {
	if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_PDEATHSIG): %w", err)
	}
	return nil
}
