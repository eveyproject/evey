//go:build linux && arm64

package main

// AUDIT_ARCH_AARCH64 — see seccomp_arch_amd64.go for the rationale.
const auditArch = 0xC00000B7
