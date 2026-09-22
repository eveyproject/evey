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
	bpfALU = 0x04
	bpfAND = 0x50
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
	enosysData     = 38         // ENOSYS, fits in SECCOMP_RET_DATA
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
		unix.SYS_KEXEC_LOAD, unix.SYS_KEXEC_FILE_LOAD,
		unix.SYS_SETUID, unix.SYS_SETGID, unix.SYS_SETREUID, unix.SYS_SETREGID,
		unix.SYS_SETRESUID, unix.SYS_SETRESGID, unix.SYS_CAPSET,
		unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY, unix.SYS_KEYCTL,
		// io_uring and userfaultfd are premier kernel-exploit primitives (a long
		// LPE CVE history; ChromeOS/Android/GKE disable io_uring for this). No
		// plugin runtime (node/python/Go) needs them by default, and Docker's
		// default profile does not reliably block them - so deny them here, where
		// the filter also covers static/compiled plugins the LD_PRELOAD shim misses.
		unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
		unix.SYS_USERFAULTFD,
	}
}

// buildSeccompFilter returns a classic-BPF program: validate the ABI (kill on
// mismatch to foil i386/x32 syscall-number confusion), then EPERM every denied
// syscall. When denyInet is set, socket(2) with AF_INET/AF_INET6 is also EPERM'd
// (the kernel equivalent of bubblewrap's --unshare-net: a plugin without the
// network permission cannot open a TCP/UDP socket at all, so IP-literal egress
// can't sidestep the DNS allowlist). AF_UNIX is untouched so HostService gRPC works.
//
// denyFork refuses PROCESS creation while leaving THREAD creation alone, which
// is what makes it safe to apply to every plugin that has not asked for the
// "subprocess" permission: node, python and Go all create threads and none of
// them needs to fork to run. It is the kernel-hard cap the userspace resource
// sweep cannot provide (it polls every ~2s; a fork bomb is faster than that).
// See appendDenyForkRules for the per-syscall reasoning.
func buildSeccompFilter(denyInet, denyFork bool) []unix.SockFilter {
	f := []unix.SockFilter{
		{Code: bpfLD | bpfW | bpfABS, K: offArch},
		{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: auditArch}, // arch ok → skip kill
		{Code: bpfRET | bpfK, K: retKillProcess},
		{Code: bpfLD | bpfW | bpfABS, K: offNR}, // A = syscall nr
	}
	// Self-contained 2-instruction DENY block per syscall (sandbox.c pattern):
	// match → fall through to the EPERM ret; no match → skip it.
	// archDeniedSyscalls adds arch-only numbers (e.g. x86 ioperm/iopl that do not
	// exist on arm64) - see legacyForkSyscalls for the same per-arch pattern.
	for _, nr := range append(deniedSyscalls(), archDeniedSyscalls()...) {
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
	if denyFork {
		f = appendDenyForkRules(f)
	}
	f = append(f, unix.SockFilter{Code: bpfRET | bpfK, K: retAllow})
	return f
}

// appendDenyForkRules blocks the three ways a process is created on Linux while
// leaving pthread_create working. Each block is self-contained (local jump
// offsets only), so it composes with the denylist above and the final ALLOW.
//
// clone3 gets ENOSYS, NOT EPERM. Seccomp cannot dereference the struct
// clone_args pointer clone3 takes, so its flags are invisible to a classic-BPF
// filter and the syscall cannot be inspected the way clone(2) can. ENOSYS is
// the contract glibc >= 2.34 already handles: pthread_create tries clone3 first
// and falls back to clone(2) when the kernel reports it missing (the same trick
// Docker's default profile uses). EPERM here would abort thread creation and
// break every node and python plugin - exactly the class of breakage this whole
// change exists to undo.
//
// fork/vfork carry no flags to inspect and are always a new process, so they
// are denied outright. They only exist on some arches (arm64 has neither;
// glibc goes through clone there), hence the per-arch list.
//
// clone(2) is the one that needs argument inspection: the SAME syscall creates
// a thread and a process, told apart by CLONE_THREAD in args[0]. Allow it set,
// deny it clear.
func appendDenyForkRules(f []unix.SockFilter) []unix.SockFilter {
	f = append(f,
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: uint32(unix.SYS_CLONE3)},
		unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | enosysData},
	)
	for _, nr := range legacyForkSyscalls() {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: nr},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
		)
	}
	return append(f,
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 5, K: uint32(unix.SYS_CLONE)}, // not clone → ALLOW
		unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0},                               // A = clone flags (low 32)
		unix.SockFilter{Code: bpfALU | bpfAND | bpfK, K: uint32(unix.CLONE_THREAD)},
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: 0}, // CLONE_THREAD clear → EPERM
		unix.SockFilter{Code: bpfJMP | bpfJA, K: 1},                       // thread → ALLOW
		unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
	)
}

// applySeccomp installs the denylist on the CALLING thread. It must be the last
// confinement step before execve: the filter is per-thread (no TSYNC), so only
// the exec-ing thread is constrained here - which is correct, because execve
// preserves the filter into the (single-threaded) plugin image, and the rest of
// the launcher's Go runtime threads stay unfiltered until they're replaced.
func applySeccomp(denyInet, denyFork bool) error {
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
	}
	filter := buildSeccompFilter(denyInet, denyFork)
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	err := unix.Prctl(unix.PR_SET_SECCOMP, uintptr(unix.SECCOMP_MODE_FILTER), uintptr(unsafe.Pointer(&prog)), 0, 0)
	runtime.KeepAlive(&prog)
	runtime.KeepAlive(filter)
	if err != nil {
		return fmt.Errorf("PR_SET_SECCOMP: %w", err)
	}
	return nil
}
