//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

// Regression guard for the 2026-07-28 outage AND for the hardening that
// replaced its broken fix. The launcher must be able to deny process creation
// without denying THREAD creation: the previous attempt at a kernel-hard
// resource cap (-rlimit-nproc, per-UID) killed every plugin at boot with
// "fatal error: newosproc" because the cap counted the server's own threads.
//
// The probe runs behind a REAL launcher exec (TestMain re-exec), so the filter
// under test is the production one, installed the production way and inherited
// across execve. The child is a Go binary: reaching its main at all already
// means the Go runtime created its startup threads through the filter, and it
// then forces several more.
//
// Probing choice - the clone(2) flag combinations below are all REJECTED by the
// kernel (EINVAL), so no process and no thread is ever actually created:
//   - CLONE_SIGHAND without CLONE_VM is invalid, and carries no CLONE_THREAD,
//     so our filter must EPERM it. EINVAL instead means the filter let a
//     process-creating clone reach the kernel.
//   - CLONE_THREAD without CLONE_SIGHAND|CLONE_VM is equally invalid, but it
//     DOES carry CLONE_THREAD, so the filter must let it through: EINVAL is the
//     pass condition and EPERM is the failure that would break node.
//
// That inversion is the whole point: same syscall, opposite verdicts, decided
// only by the flag bit.
const forkProbeArg = "-evey-fork-probe"

func forkProbeMain() int {
	// Force real OS thread creation under the filter. Each locked goroutine
	// needs its own M, so the runtime must clone(CLONE_THREAD) repeatedly - the
	// exact call the outage made impossible.
	var wg sync.WaitGroup
	release := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			<-release
		})
	}
	close(release)
	wg.Wait()

	// A thread-creating clone must reach the kernel (EINVAL), not be filtered.
	if _, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(unix.CLONE_THREAD), 0, 0); errno != unix.EINVAL {
		fmt.Fprintf(os.Stderr, "clone(CLONE_THREAD) blocked by the filter: errno=%v (want EINVAL)\n", errno)
		return 20
	}
	// A process-creating clone must be denied before the kernel sees it.
	if _, _, errno := unix.RawSyscall(unix.SYS_CLONE, uintptr(unix.CLONE_SIGHAND), 0, 0); errno != unix.EPERM {
		fmt.Fprintf(os.Stderr, "clone without CLONE_THREAD not denied: errno=%v (want EPERM)\n", errno)
		return 21
	}
	// clone3 must report ENOSYS, never EPERM: glibc's pthread_create probes it
	// first and only falls back to clone(2) on ENOSYS. EPERM here is what would
	// break threading for every glibc runtime.
	if _, _, errno := unix.RawSyscall(unix.SYS_CLONE3, 0, 0, 0); errno != unix.ENOSYS {
		fmt.Fprintf(os.Stderr, "clone3 wrong errno: %v (want ENOSYS so glibc falls back to clone)\n", errno)
		return 22
	}
	return 0
}

// launcherForkProbe runs the probe behind the real launcher and returns its
// exit code. extraFlags lets a caller drop -deny-fork to prove the probe is
// actually measuring the filter and not a kernel quirk.
func launcherForkProbe(t *testing.T, extraFlags ...string) (int, string) {
	t.Helper()
	self := os.Args[0]
	args := []string{
		"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-ro", "/etc",
		"-ro", filepath.Dir(self), // the test binary the launcher must exec
		"-seccomp",
	}
	args = append(args, extraFlags...)
	args = append(args, "--", self, forkProbeArg)

	cmd := exec.Command(self, args...)
	cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); !ok {
		t.Fatalf("running the launcher failed: %v\n%s", err, out)
	}
	return exitErr.ExitCode(), string(out)
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError) //nolint:errorlint // exec.Cmd returns it directly
	if ok {
		*target = e
	}
	return ok
}

func TestLauncher_DenyFork_BlocksProcessesKeepsThreads(t *testing.T) {
	code, out := launcherForkProbe(t, "-deny-fork")
	if code != 0 {
		t.Fatalf("fork probe failed under -deny-fork (exit %d):\n%s", code, out)
	}
}

// TestLauncher_WithoutDenyFork_ProcessCloneReachesKernel proves the probe reads
// the filter and not the kernel: without -deny-fork the SAME process-creating
// clone must reach the kernel and be rejected there (EINVAL), so the probe
// fails at its EPERM assertion with code 21.
func TestLauncher_WithoutDenyFork_ProcessCloneReachesKernel(t *testing.T) {
	code, out := launcherForkProbe(t)
	if code != 21 {
		t.Fatalf("expected the probe to report an unfiltered clone (exit 21), got %d:\n%s", code, out)
	}
}

// TestBuildSeccompFilter_DenyForkCoversLegacyForkSyscalls covers what the
// behavioural probe deliberately does not: fork(2)/vfork(2) take no arguments,
// so a probe that reached the kernel would really fork the test binary. Assert
// their presence structurally instead. On arm64 the list is empty by design
// (the arch has neither syscall).
func TestBuildSeccompFilter_DenyForkCoversLegacyForkSyscalls(t *testing.T) {
	filter := buildSeccompFilter(uint32(os.Getpid()), false, true)
	for _, nr := range legacyForkSyscalls() {
		found := false
		for i, ins := range filter {
			if ins.Code == bpfJMP|bpfJEQ|bpfK && ins.K == nr && i+1 < len(filter) &&
				filter[i+1].Code == bpfRET|bpfK && filter[i+1].K == retErrno|epermData {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("syscall %d has no EPERM block under -deny-fork", nr)
		}
	}
	// And nothing is denied when the flag is off (the subprocess-permission path).
	plain := buildSeccompFilter(uint32(os.Getpid()), false, false)
	for _, ins := range plain {
		if ins.Code == bpfRET|bpfK && ins.K == retErrno|enosysData {
			t.Error("clone3 must not be touched without -deny-fork: subprocess plugins need it")
		}
	}
}
