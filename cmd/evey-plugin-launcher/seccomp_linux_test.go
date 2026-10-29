//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"unsafe"

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
	// Every way to signal (or steal an fd from) ANOTHER same-uid process is
	// refused; the args would otherwise reach the kernel and fail differently.
	self, other := uintptr(os.Getpid()), uintptr(os.Getppid())
	tid := uintptr(unix.Gettid())
	failed := false
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [3]uintptr
	}{
		{"kill parent", unix.SYS_KILL, [3]uintptr{other, 0, 0}},
		{"kill own group", unix.SYS_KILL, [3]uintptr{0, 0, 0}},
		{"kill everyone", unix.SYS_KILL, [3]uintptr{^uintptr(0), 0, 0}},
		{"tgkill parent", unix.SYS_TGKILL, [3]uintptr{other, other, 0}},
		{"tkill", unix.SYS_TKILL, [3]uintptr{tid, 0, 0}},
		{"rt_sigqueueinfo parent", unix.SYS_RT_SIGQUEUEINFO, [3]uintptr{other, 0, 0}},
		{"rt_tgsigqueueinfo parent", unix.SYS_RT_TGSIGQUEUEINFO, [3]uintptr{other, other, 0}},
		{"pidfd_send_signal", unix.SYS_PIDFD_SEND_SIGNAL, [3]uintptr{^uintptr(0), 0, 0}},
		{"pidfd_getfd", unix.SYS_PIDFD_GETFD, [3]uintptr{^uintptr(0), 0, 0}},
	} {
		if _, _, errno := unix.Syscall(sc.nr, sc.args[0], sc.args[1], sc.args[2]); errno != unix.EPERM {
			fmt.Fprintf(os.Stderr, "%s not denied: errno=%v\n", sc.name, errno)
			failed = true
		}
	}
	// Signals to itself must reach the kernel: node's SIGTERM handler re-raises
	// with tgkill(self) and stayed alive when refused (orphans after a server
	// restart), and the Go runtime preempts its own threads the same way.
	// Signal 0 probes delivery without sending anything.
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [3]uintptr
	}{
		{"kill self", unix.SYS_KILL, [3]uintptr{self, 0, 0}},
		{"tgkill self", unix.SYS_TGKILL, [3]uintptr{self, tid, 0}},
		{"rt_sigqueueinfo self", unix.SYS_RT_SIGQUEUEINFO, [3]uintptr{self, 0, 0}},
		{"rt_tgsigqueueinfo self", unix.SYS_RT_TGSIGQUEUEINFO, [3]uintptr{self, tid, 0}},
	} {
		if _, _, errno := unix.Syscall(sc.nr, sc.args[0], sc.args[1], sc.args[2]); errno == unix.EPERM {
			fmt.Fprintf(os.Stderr, "%s refused: errno=%v\n", sc.name, errno)
			failed = true
		}
	}
	if failed {
		os.Exit(17)
	}
	if !probeOtherProcessTuning() {
		os.Exit(20)
	}
	if !probeSignalOwner() {
		os.Exit(21)
	}
	// Namespaces stay closed even for a plugin allowed to fork: clone with a
	// CLONE_NEW* flag is unshare by another name, and clone3 hides its flags
	// from the filter. CLONE_NEWUSER|CLONE_FS is EINVAL in the kernel, so a
	// call the filter lets through cannot create anything.
	if _, _, errno := unix.Syscall(unix.SYS_CLONE, unix.CLONE_NEWUSER|unix.CLONE_FS, 0, 0); errno != unix.EPERM {
		fmt.Fprintf(os.Stderr, "clone(CLONE_NEWUSER) not denied: errno=%v\n", errno)
		os.Exit(18)
	}
	if _, _, errno := unix.Syscall(unix.SYS_CLONE3, 0, 0, 0); errno != unix.ENOSYS {
		fmt.Fprintf(os.Stderr, "clone3 not ENOSYS: errno=%v\n", errno)
		os.Exit(19)
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

// probeOtherProcessTuning checks that resource and scheduling knobs only reach
// the plugin itself. The foreign target never exists (pids stay below 2^22), so
// a call that slips past the filter fails with ESRCH and touches nothing.
func probeOtherProcessTuning() bool {
	const missingPid = 1 << 22
	const ioprioWhoProcess, ioprioWhoUser = 1, 3
	var lim unix.Rlimit
	var param [8]byte
	var mask unix.CPUSet
	if err := unix.SchedGetaffinity(0, &mask); err != nil {
		fmt.Fprintf(os.Stderr, "sched_getaffinity: %v\n", err)
		return false
	}
	nice, _, _ := unix.Syscall(unix.SYS_GETPRIORITY, unix.PRIO_PROCESS, 0, 0)
	ioprio, _, _ := unix.Syscall(unix.SYS_IOPRIO_GET, ioprioWhoProcess, 0, 0)
	limPtr := uintptr(unsafe.Pointer(&lim))
	paramPtr := uintptr(unsafe.Pointer(&param))
	maskPtr := uintptr(unsafe.Pointer(&mask))
	ok := true
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [4]uintptr
	}{
		{"prlimit64 other", unix.SYS_PRLIMIT64, [4]uintptr{missingPid, unix.RLIMIT_NOFILE, 0, limPtr}},
		{"setpriority other", unix.SYS_SETPRIORITY, [4]uintptr{unix.PRIO_PROCESS, missingPid, 0}},
		{"setpriority user", unix.SYS_SETPRIORITY, [4]uintptr{unix.PRIO_USER, missingPid, 0}},
		{"sched_setaffinity other", unix.SYS_SCHED_SETAFFINITY, [4]uintptr{missingPid, unsafe.Sizeof(mask), maskPtr}},
		{"sched_setparam other", unix.SYS_SCHED_SETPARAM, [4]uintptr{missingPid, paramPtr}},
		{"sched_setscheduler other", unix.SYS_SCHED_SETSCHEDULER, [4]uintptr{missingPid, 0, paramPtr}},
		{"sched_setattr other", unix.SYS_SCHED_SETATTR, [4]uintptr{missingPid, 0, 0}},
		{"ioprio_set other", unix.SYS_IOPRIO_SET, [4]uintptr{ioprioWhoProcess, missingPid, ioprio}},
		{"ioprio_set user", unix.SYS_IOPRIO_SET, [4]uintptr{ioprioWhoUser, 0, 7 << 13}},
		{"migrate_pages other", unix.SYS_MIGRATE_PAGES, [4]uintptr{missingPid, 0, 0, 0}},
		{"move_pages other", unix.SYS_MOVE_PAGES, [4]uintptr{missingPid, 0, 0, 0}},
	} {
		if _, _, errno := unix.Syscall6(sc.nr, sc.args[0], sc.args[1], sc.args[2], sc.args[3], 0, 0); errno != unix.EPERM {
			fmt.Fprintf(os.Stderr, "%s not denied: errno=%v\n", sc.name, errno)
			ok = false
		}
	}
	self := uintptr(os.Getpid())
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [4]uintptr
	}{
		{"prlimit64 self", unix.SYS_PRLIMIT64, [4]uintptr{0, unix.RLIMIT_NOFILE, 0, limPtr}},
		{"prlimit64 own pid", unix.SYS_PRLIMIT64, [4]uintptr{self, unix.RLIMIT_NOFILE, 0, limPtr}},
		{"setpriority self", unix.SYS_SETPRIORITY, [4]uintptr{unix.PRIO_PROCESS, 0, 20 - nice}},
		{"sched_setaffinity self", unix.SYS_SCHED_SETAFFINITY, [4]uintptr{0, unsafe.Sizeof(mask), maskPtr}},
		{"sched_setparam self", unix.SYS_SCHED_SETPARAM, [4]uintptr{0, paramPtr}},
		{"ioprio_set self", unix.SYS_IOPRIO_SET, [4]uintptr{ioprioWhoProcess, self, ioprio}},
		{"move_pages self", unix.SYS_MOVE_PAGES, [4]uintptr{0, 0, 0, 0}},
	} {
		if _, _, errno := unix.Syscall6(sc.nr, sc.args[0], sc.args[1], sc.args[2], sc.args[3], 0, 0); errno == unix.EPERM {
			fmt.Fprintf(os.Stderr, "%s refused: errno=%v\n", sc.name, errno)
			ok = false
		}
	}
	return ok
}

// probeSignalOwner checks that an fd's signal owner stays on the plugin:
// F_SETOWN(parent) + F_SETSIG(SIGKILL) + O_ASYNC would otherwise let one write
// to its own pipe kill the server, which shares the uid.
func probeSignalOwner() bool {
	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		fmt.Fprintf(os.Stderr, "pipe: %v\n", err)
		return false
	}
	defer unix.Close(p[0])
	defer unix.Close(p[1])
	fd := uintptr(p[0])
	self, parent := uintptr(os.Getpid()), uintptr(os.Getppid())
	var owner [8]byte
	ownerPtr := uintptr(unsafe.Pointer(&owner))
	ok := true
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [3]uintptr
	}{
		{"F_SETOWN parent", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETOWN, parent}},
		{"F_SETOWN parent group", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETOWN, uintptr(-int(parent))}},
		{"F_SETOWN_EX", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETOWN_EX, ownerPtr}},
		{"F_SETSIG SIGKILL", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETSIG, uintptr(unix.SIGKILL)}},
		{"ioctl FIOSETOWN", unix.SYS_IOCTL, [3]uintptr{fd, 0x8901, ownerPtr}},
		{"ioctl SIOCSPGRP", unix.SYS_IOCTL, [3]uintptr{fd, 0x8902, ownerPtr}},
	} {
		if _, _, errno := unix.Syscall(sc.nr, sc.args[0], sc.args[1], sc.args[2]); errno != unix.EPERM {
			fmt.Fprintf(os.Stderr, "%s not denied: errno=%v\n", sc.name, errno)
			ok = false
		}
	}
	for _, sc := range []struct {
		name string
		nr   uintptr
		args [3]uintptr
	}{
		{"F_SETOWN self", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETOWN, self}},
		{"F_SETOWN 0", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETOWN, 0}},
		{"F_SETSIG default", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETSIG, 0}},
		{"F_GETFL", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_GETFL, 0}},
		{"F_SETFL O_NONBLOCK", unix.SYS_FCNTL, [3]uintptr{fd, unix.F_SETFL, unix.O_NONBLOCK}},
		{"ioctl FIONREAD", unix.SYS_IOCTL, [3]uintptr{fd, 0x541B, ownerPtr}}, // same number on amd64 and arm64
	} {
		if _, _, errno := unix.Syscall(sc.nr, sc.args[0], sc.args[1], sc.args[2]); errno != 0 {
			fmt.Fprintf(os.Stderr, "%s refused: errno=%v\n", sc.name, errno)
			ok = false
		}
	}
	return ok
}
