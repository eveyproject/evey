//go:build linux && amd64

package main

// AUDIT_ARCH_X86_64 — the seccomp filter kills the process on any other ABI so
// 32-bit (i386/x32) syscall numbers can't be used to slip past the denylist.
const auditArch = 0xC000003E
