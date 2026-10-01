//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

const shimProbeSkip = 3 // shim not loaded (static test binary): nothing measured

// TestSandboxShim_Signals builds sandbox/sandbox.c the way the Dockerfile does
// and LD_PRELOADs it into a re-exec of this binary: signalling itself must keep
// working (raise/abort/Go preemption), signalling the same-uid parent must not.
func TestSandboxShim_Signals(t *testing.T) {
	if os.Getenv("EVEY_SHIM_PROBE") == "1" {
		os.Exit(shimProbe())
	}
	out, err := runUnderShim(t, "TestSandboxShim_Signals")
	if err != nil {
		t.Fatalf("shim probe failed: %v\n%s", err, out)
	}
}

// runUnderShim builds the shim and re-execs this binary's test under LD_PRELOAD
// with EVEY_SHIM_PROBE=1; it skips when the shim cannot be measured here.
func runUnderShim(t *testing.T, test string) ([]byte, error) {
	t.Helper()
	cc, err := exec.LookPath("gcc")
	if err != nil {
		if cc, err = exec.LookPath("cc"); err != nil {
			t.Skip("no C compiler; cannot build libevey-sandbox.so")
		}
	}
	so := filepath.Join(t.TempDir(), "libevey-sandbox.so")
	if out, err := exec.Command(cc, "-shared", "-fPIC", "-nostartfiles", "-o", so, "../../sandbox/sandbox.c").CombinedOutput(); err != nil {
		t.Fatalf("build sandbox.c: %v\n%s", err, out)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+test+"$")
	cmd.Env = append(os.Environ(), "EVEY_SHIM_PROBE=1", "LD_PRELOAD="+so)
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == shimProbeSkip {
		t.Skipf("shim not loaded into the test binary (static build?): %s", out)
	}
	return out, err
}

// shimActive is false when LD_PRELOAD had no effect (static test binary).
func shimActive() bool {
	_, _, e := unix.RawSyscall(unix.SYS_UNSHARE, 0, 0, 0)
	return e == unix.EPERM
}

func shimProbe() int {
	if !shimActive() {
		fmt.Fprintln(os.Stderr, "shim filter not active")
		return shimProbeSkip
	}
	self, parent := uintptr(os.Getpid()), uintptr(os.Getppid())
	bad := false
	expect := func(name string, want unix.Errno, nr uintptr, a ...uintptr) {
		a = append(a, 0, 0, 0)
		if _, _, e := unix.RawSyscall6(nr, a[0], a[1], a[2], 0, 0, 0); e != want {
			fmt.Fprintf(os.Stderr, "%s: errno=%v want %v\n", name, e, want)
			bad = true
		}
	}
	// Self-directed: reaches the kernel (0, or EFAULT for the NULL siginfo).
	expect("tgkill(self)", 0, unix.SYS_TGKILL, self, self, 0)
	expect("tkill(self)", 0, unix.SYS_TKILL, self, 0)
	expect("rt_sigqueueinfo(self)", unix.EFAULT, unix.SYS_RT_SIGQUEUEINFO, self, 0, 0)
	// Another same-uid process (the test runner standing in for the server).
	expect("kill(parent)", unix.EPERM, unix.SYS_KILL, parent, 0)
	expect("tgkill(parent)", unix.EPERM, unix.SYS_TGKILL, parent, parent, 0)
	expect("tkill(parent)", unix.EPERM, unix.SYS_TKILL, parent, 0)
	expect("rt_sigqueueinfo(parent)", unix.EPERM, unix.SYS_RT_SIGQUEUEINFO, parent, 0, 0)
	expect("rt_tgsigqueueinfo(parent)", unix.EPERM, unix.SYS_RT_TGSIGQUEUEINFO, parent, parent, 0)
	expect("pidfd_send_signal", unix.EPERM, unix.SYS_PIDFD_SEND_SIGNAL, ^uintptr(0), 0, 0)
	expect("pidfd_getfd", unix.EPERM, unix.SYS_PIDFD_GETFD, ^uintptr(0), 0, 0)
	if bad {
		return 1
	}
	return 0
}
