//go:build linux && arm64

package main

// AUDIT_ARCH_AARCH64 - see seccomp_arch_amd64.go for the rationale.
const auditArch = 0xC00000B7

// legacyForkSyscalls is empty on arm64: the arch defines neither fork(2) nor
// vfork(2), so glibc implements both through clone(2), which the CLONE_THREAD
// check already covers. Denying numbers that do not exist here would silently
// match whatever syscall happens to hold them.
func legacyForkSyscalls() []uint32 { return nil }

// archDeniedSyscalls is empty on arm64: ioperm/iopl are x86-only and have no
// syscall number here, so denying a number would silently match another syscall.
func archDeniedSyscalls() []uint32 { return nil }
