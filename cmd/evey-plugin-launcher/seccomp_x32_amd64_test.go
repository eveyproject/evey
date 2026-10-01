//go:build linux && amd64

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

const x32SyscallBit = 0x40000000 // __X32_SYSCALL_BIT

// TestLauncherSeccomp_X32Rejected: x32 syscalls report AUDIT_ARCH_X86_64, so the
// arch check alone lets them through with a number the denylist never matches.
// A denied syscall issued via its x32 number must kill the process (SIGSYS).
func TestLauncherSeccomp_X32Rejected(t *testing.T) {
	if os.Getenv("SECCOMP_X32_CHILD") == "1" {
		if err := applySeccomp(false, false); err != nil {
			fmt.Fprintf(os.Stderr, "applySeccomp: %v\n", err)
			os.Exit(10)
		}
		_, _, errno := unix.RawSyscall(x32SyscallBit|unix.SYS_UNSHARE, 0, 0, 0)
		fmt.Fprintf(os.Stderr, "x32 unshare reached the kernel: errno=%v\n", errno)
		os.Exit(30)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLauncherSeccomp_X32Rejected$")
	cmd.Env = append(os.Environ(), "SECCOMP_X32_CHILD=1")
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("child: %v\n%s", err, out)
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGSYS {
		t.Fatalf("x32 syscall not killed by the filter (exit %d): %s", ee.ExitCode(), out)
	}
}

// TestSandboxShim_X32Rejected: same bypass against the LD_PRELOAD shim, where an
// x32 execve would skip the whole denylist.
func TestSandboxShim_X32Rejected(t *testing.T) {
	if os.Getenv("EVEY_SHIM_PROBE") == "1" {
		if !shimActive() {
			os.Exit(shimProbeSkip)
		}
		_, _, errno := unix.RawSyscall(x32SyscallBit|unix.SYS_UNSHARE, 0, 0, 0)
		fmt.Fprintf(os.Stderr, "x32 unshare reached the kernel: errno=%v\n", errno)
		os.Exit(30)
	}
	out, err := runUnderShim(t, "TestSandboxShim_X32Rejected")
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("child: %v\n%s", err, out)
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGSYS {
		t.Fatalf("x32 syscall not killed by the shim (exit %d): %s", ee.ExitCode(), out)
	}
}
