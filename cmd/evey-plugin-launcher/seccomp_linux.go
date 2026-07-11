//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Classic-BPF opcodes (stable kernel ABI; not all are exported by x/sys/unix).
const (
	bpfLD  = 0x00
	bpfW   = 0x00
	bpfABS = 0x20
	bpfJMP = 0x05
	bpfJEQ = 0x10
	bpfJA  = 0x00
	bpfK   = 0x00
	bpfRET = 0x06
)

// struct seccomp_data field offsets (nr __u32 @0, arch __u32 @4, args[0] __u64
// @16 - we read the low 32 bits, which is the AF_* family for socket(2)).
const (
	offNR   = 0
	offArch = 4
	offArg0 = 16
)

// seccomp_ret actions.
const (
	retKillProcess = 0x80000000 // SECCOMP_RET_KILL_PROCESS
	retErrno       = 0x00050000 // SECCOMP_RET_ERRNO (| errno)
	retAllow       = 0x7fff0000 // SECCOMP_RET_ALLOW
	epermData      = 1          // EPERM, fits in SECCOMP_RET_DATA
)

const (
	afInet  = 2  // AF_INET
	afInet6 = 10 // AF_INET6
)

// deniedSyscalls mirrors sandbox/sandbox.c's denylist MINUS execve/execveat:
// the launcher must execve the plugin, and any child the plugin later spawns
// inherits this filter + the Landlock domain, so it stays confined regardless.
// Numbers come from x/sys/unix, which resolves them correctly per build arch.
func deniedSyscalls() []uint32 {
	return []uint32{
		unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV,
		unix.SYS_KILL, unix.SYS_TGKILL,
		unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
		unix.SYS_UNSHARE, unix.SYS_SETNS,
		unix.SYS_INIT_MODULE, unix.SYS_FINIT_MODULE, unix.SYS_DELETE_MODULE,
		unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN,
		unix.SYS_SWAPON, unix.SYS_SWAPOFF, unix.SYS_SYSLOG, unix.SYS_REBOOT,
		unix.SYS_SETTIMEOFDAY, unix.SYS_CLOCK_SETTIME, unix.SYS_CLOCK_ADJTIME, unix.SYS_ADJTIMEX,
		unix.SYS_KEXEC_LOAD,
		unix.SYS_SETUID, unix.SYS_SETGID, unix.SYS_SETREUID, unix.SYS_SETREGID,
		unix.SYS_SETRESUID, unix.SYS_SETRESGID, unix.SYS_CAPSET,
		unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY, unix.SYS_KEYCTL,
	}
}

// buildSeccompFilter returns a classic-BPF program: validate the ABI (kill on
// mismatch to foil i386/x32 syscall-number confusion), then EPERM every denied
// syscall. When denyInet is set, socket(2) with AF_INET/AF_INET6 is also EPERM'd
// (the kernel equivalent of bubblewrap's --unshare-net: a plugin without the
// network permission cannot open a TCP/UDP socket at all, so IP-literal egress
// can't sidestep the DNS allowlist). AF_UNIX is untouched so HostService gRPC works.
func buildSeccompFilter(denyInet bool) []unix.SockFilter {
	f := []unix.SockFilter{
		{Code: bpfLD | bpfW | bpfABS, K: offArch},
		{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: auditArch}, // arch ok → skip kill
		{Code: bpfRET | bpfK, K: retKillProcess},
		{Code: bpfLD | bpfW | bpfABS, K: offNR}, // A = syscall nr
	}
	// Self-contained 2-instruction DENY block per syscall (sandbox.c pattern):
	// match → fall through to the EPERM ret; no match → skip it.
	for _, nr := range deniedSyscalls() {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: nr},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
		)
	}
	if denyInet {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 5, K: uint32(unix.SYS_SOCKET)}, // not socket → ALLOW
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0},                                // A = domain
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 2, Jf: 0, K: afInet},                  // AF_INET  → EPERM
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: afInet6},                 // AF_INET6 → EPERM
			unix.SockFilter{Code: bpfJMP | bpfJA, K: 1},                                             // else (AF_UNIX, …) → ALLOW
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
		)
	}
	f = append(f, unix.SockFilter{Code: bpfRET | bpfK, K: retAllow})
	return f
}

// applySeccomp installs the denylist on the CALLING thread. It must be the last
// confinement step before execve: the filter is per-thread (no TSYNC), so only
// the exec-ing thread is constrained here - which is correct, because execve
// preserves the filter into the (single-threaded) plugin image, and the rest of
// the launcher's Go runtime threads stay unfiltered until they're replaced.
func applySeccomp(denyInet bool) error {
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
	}
	filter := buildSeccompFilter(denyInet)
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	err := unix.Prctl(unix.PR_SET_SECCOMP, uintptr(unix.SECCOMP_MODE_FILTER), uintptr(unsafe.Pointer(&prog)), 0, 0)
	runtime.KeepAlive(&prog)
	runtime.KeepAlive(filter)
	if err != nil {
		return fmt.Errorf("PR_SET_SECCOMP: %w", err)
	}
	return nil
}
