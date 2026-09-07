//go:build linux && amd64

package main

import "golang.org/x/sys/unix"

// AUDIT_ARCH_X86_64 - the seccomp filter kills the process on any other ABI so
// 32-bit (i386/x32) syscall numbers can't be used to slip past the denylist.
const auditArch = 0xC000003E

// legacyForkSyscalls are the flagless process-creation syscalls this arch still
// exposes. x86_64 keeps both; see seccomp_arch_arm64.go for the arch that has
// neither.
func legacyForkSyscalls() []uint32 {
	return []uint32{uint32(unix.SYS_FORK), uint32(unix.SYS_VFORK)}
}

// archDeniedSyscalls are x86-only numbers absent from arm64: ioperm/iopl grant
// direct access to CPU I/O ports (a device/hardware attack surface a sandboxed
// plugin never needs). Mirrors sandbox/sandbox.c's #ifdef __NR_ioperm block.
func archDeniedSyscalls() []uint32 {
	return []uint32{uint32(unix.SYS_IOPERM), uint32(unix.SYS_IOPL)}
}
