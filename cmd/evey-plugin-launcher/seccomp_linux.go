//go:build linux && (amd64 || arm64)

package main

import (
	"fmt"
	"os"
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
	bpfJGE = 0x30
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
	offArg1 = 24
)

// seccomp_ret actions.
const (
	retKillProcess = 0x80000000 // SECCOMP_RET_KILL_PROCESS
	retErrno       = 0x00050000 // SECCOMP_RET_ERRNO (| errno)
	retAllow       = 0x7fff0000 // SECCOMP_RET_ALLOW
	x32SysBit      = 0x40000000 // __X32_SYSCALL_BIT: x32 calls pass the x86_64 arch check
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
		// Signal paths whose target the filter cannot pin to the plugin (tkill
		// names a thread id, a pidfd hides the pid), plus pidfd_getfd, which
		// copies an fd out of a same-uid process. See selfSignalSyscalls.
		unix.SYS_TKILL, unix.SYS_PIDFD_SEND_SIGNAL, unix.SYS_PIDFD_GETFD,
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

// selfSignalSyscalls take the target process id as their first argument: the
// filter lets them through only when it is the plugin itself. Refusing them
// outright kept node alive on SIGTERM (its handler re-raises with tgkill(self))
// and broke the Go runtime's self-preemption; any other target (the server,
// Postgres, Meilisearch, the process group, -1) stays EPERM.
func selfSignalSyscalls() []uint32 {
	return []uint32{unix.SYS_KILL, unix.SYS_TGKILL, unix.SYS_RT_SIGQUEUEINFO, unix.SYS_RT_TGSIGQUEUEINFO}
}

// appendSelfSignalRules adds one block per selfSignalSyscalls entry: ALLOW when
// args[0] (64-bit, little-endian on both archs) equals self, EPERM otherwise.
// A non-matching syscall skips the block with A still holding its number.
func appendSelfSignalRules(f []unix.SockFilter, self uint32) []unix.SockFilter {
	for _, nr := range selfSignalSyscalls() {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 6, K: nr},
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0}, // low 32 bits of the target
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 3, K: self},
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0 + 4}, // high 32 bits: 0 for a real pid
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: 0},
			unix.SockFilter{Code: bpfRET | bpfK, K: retAllow},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
		)
	}
	return f
}

// selfTargetSyscalls tune limits, scheduling or memory placement of the
// process named by args[0], where 0 means the caller. Under a shared uid the
// kernel lets a plugin aim them at its neighbours, so only 0 and self pass.
func selfTargetSyscalls() []uint32 {
	return []uint32{
		unix.SYS_PRLIMIT64, unix.SYS_SCHED_SETAFFINITY, unix.SYS_SCHED_SETPARAM,
		unix.SYS_SCHED_SETSCHEDULER, unix.SYS_SCHED_SETATTR,
		unix.SYS_MIGRATE_PAGES, unix.SYS_MOVE_PAGES,
	}
}

// whichTargetSyscalls take (which, who): only the per-process selector passes,
// since the group and user selectors with who=0 reach every same-uid process.
var whichTargetSyscalls = []struct{ nr, perProcess uint32 }{
	{unix.SYS_SETPRIORITY, unix.PRIO_PROCESS},
	{unix.SYS_IOPRIO_SET, 1}, // IOPRIO_WHO_PROCESS
}

// appendSelfTargetRules ALLOWs those syscalls when the target pid (low 32
// bits: the kernel reads a pid_t) is 0 or self and EPERMs them otherwise. A
// non-matching syscall skips each block with A still holding its number.
func appendSelfTargetRules(f []unix.SockFilter, self uint32) []unix.SockFilter {
	for _, nr := range selfTargetSyscalls() {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 5, K: nr},
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0},
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 2, Jf: 0, K: 0},
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: self},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
			unix.SockFilter{Code: bpfRET | bpfK, K: retAllow},
		)
	}
	for _, sc := range whichTargetSyscalls {
		f = append(f,
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 7, K: sc.nr},
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0}, // which
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 3, K: sc.perProcess},
			unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg1}, // who
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 2, Jf: 0, K: 0},
			unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: self},
			unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
			unix.SockFilter{Code: bpfRET | bpfK, K: retAllow},
		)
	}
	return f
}

// cloneNamespaceFlags are the CLONE_NEW* bits clone(2) shares with unshare(2).
const cloneNamespaceFlags = unix.CLONE_NEWNS | unix.CLONE_NEWCGROUP | unix.CLONE_NEWUTS |
	unix.CLONE_NEWIPC | unix.CLONE_NEWUSER | unix.CLONE_NEWPID | unix.CLONE_NEWNET

// appendDenyNamespaceRules closes the namespace door for every plugin, the
// ones allowed to fork included: clone with a CLONE_NEW* flag is unshare (on
// the denylist) by another name, and clone3 keeps its flags in a struct the
// filter cannot read, so it gets ENOSYS and glibc falls back to clone (the
// -deny-fork contract, see appendDenyForkRules). Leaves A = the syscall nr.
func appendDenyNamespaceRules(f []unix.SockFilter) []unix.SockFilter {
	return append(f,
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 1, K: uint32(unix.SYS_CLONE3)},
		unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | enosysData},
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 0, Jf: 4, K: uint32(unix.SYS_CLONE)},
		unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offArg0}, // clone flags (low 32 bits hold every CLONE_NEW*)
		unix.SockFilter{Code: bpfALU | bpfAND | bpfK, K: cloneNamespaceFlags},
		unix.SockFilter{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: 0},
		unix.SockFilter{Code: bpfRET | bpfK, K: retErrno | epermData},
		unix.SockFilter{Code: bpfLD | bpfW | bpfABS, K: offNR}, // restore A for the blocks below
	)
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
func buildSeccompFilter(self uint32, denyInet, denyFork bool) []unix.SockFilter {
	f := []unix.SockFilter{
		{Code: bpfLD | bpfW | bpfABS, K: offArch},
		{Code: bpfJMP | bpfJEQ | bpfK, Jt: 1, Jf: 0, K: auditArch}, // arch ok → skip kill
		{Code: bpfRET | bpfK, K: retKillProcess},
		{Code: bpfLD | bpfW | bpfABS, K: offNR}, // A = syscall nr
		// x32 numbers would never match the denylist below; no arm64 nr is this high.
		{Code: bpfJMP | bpfJGE | bpfK, Jt: 0, Jf: 1, K: x32SysBit},
		{Code: bpfRET | bpfK, K: retKillProcess},
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
	f = appendSelfSignalRules(f, self)
	f = appendSelfTargetRules(f, self)
	f = appendDenyNamespaceRules(f)
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
	// The launcher execs the plugin in place: its pid is the plugin's pid.
	filter := buildSeccompFilter(uint32(os.Getpid()), denyInet, denyFork) //nolint:gosec // G115: a Linux pid fits in 32 bits (pid_max <= 2^22)
	prog := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	err := unix.Prctl(unix.PR_SET_SECCOMP, uintptr(unix.SECCOMP_MODE_FILTER), uintptr(unsafe.Pointer(&prog)), 0, 0)
	runtime.KeepAlive(&prog)
	runtime.KeepAlive(filter)
	if err != nil {
		return fmt.Errorf("PR_SET_SECCOMP: %w", err)
	}
	return nil
}
