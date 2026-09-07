//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

// TestLauncherSeccomp re-execs itself as a child that installs the seccomp
// filter and probes it from the (locked, filtered) thread: a denied syscall
// must EPERM, AF_UNIX sockets must stay allowed (HostService gRPC), and AF_INET
// must be denied iff -deny-inet. The child reports via exit code; 0 = all pass.
func TestLauncherSeccomp(t *testing.T) {
	switch os.Getenv("SECCOMP_CHILD") {
	case "base":
		seccompChild(false)
		return
	case "inet":
		seccompChild(true)
		return
	}
	// Process creation has its own end-to-end probe behind a real launcher exec
	// (denyfork_linux_test.go): it needs the filter to be INHERITED across
	// execve, which this in-process child cannot show.

	for _, tc := range []struct{ name, mode string }{
		{"base denylist", "base"},
		{"deny-inet", "inet"},
	} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLauncherSeccomp$")
		cmd.Env = append(os.Environ(), "SECCOMP_CHILD="+tc.mode)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s: child failed: %v\n%s", tc.name, err, out)
		}
	}
}

func seccompChild(denyInet bool) {
	if err := applySeccomp(denyInet, false); err != nil {
		fmt.Fprintf(os.Stderr, "applySeccomp: %v\n", err)
		os.Exit(10)
	}
	// unshare(0) is a harmless no-op normally; the denylist must EPERM it.
	if _, _, errno := unix.Syscall(unix.SYS_UNSHARE, 0, 0, 0); errno != unix.EPERM {
		fmt.Fprintf(os.Stderr, "unshare not denied: errno=%v\n", errno)
		os.Exit(11)
	}
	// io_uring and userfaultfd are kernel-exploit primitives - must EPERM.
	if _, _, errno := unix.Syscall(unix.SYS_IO_URING_SETUP, 0, 0, 0); errno != unix.EPERM {
		fmt.Fprintf(os.Stderr, "io_uring_setup not denied: errno=%v\n", errno)
		os.Exit(15)
	}
	if _, _, errno := unix.Syscall(unix.SYS_USERFAULTFD, 0, 0, 0); errno != unix.EPERM {
		fmt.Fprintf(os.Stderr, "userfaultfd not denied: errno=%v\n", errno)
		os.Exit(16)
	}
	// AF_UNIX must always work - the plugin talks to HostService over it.
	if fd, _, errno := unix.Syscall(unix.SYS_SOCKET, uintptr(unix.AF_UNIX), uintptr(unix.SOCK_STREAM), 0); errno != 0 {
		fmt.Fprintf(os.Stderr, "AF_UNIX socket denied: errno=%v\n", errno)
		os.Exit(12)
	} else {
		unix.Close(int(fd))
	}
	// AF_INET denied iff denyInet.
	fd, _, errno := unix.Syscall(unix.SYS_SOCKET, uintptr(unix.AF_INET), uintptr(unix.SOCK_STREAM), 0)
	if errno == 0 {
		unix.Close(int(fd))
	}
	switch {
	case denyInet && errno != unix.EPERM:
		fmt.Fprintf(os.Stderr, "AF_INET not denied under -deny-inet: errno=%v\n", errno)
		os.Exit(13)
	case !denyInet && errno != 0:
		fmt.Fprintf(os.Stderr, "AF_INET wrongly denied without -deny-inet: errno=%v\n", errno)
		os.Exit(14)
	}
	os.Exit(0)
}
