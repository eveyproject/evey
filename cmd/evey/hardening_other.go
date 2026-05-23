//go:build !linux

package main

// hardenProcess is a no-op off Linux (PR_SET_DUMPABLE is Linux-only). The
// server only runs in a Linux container in production.
func hardenProcess() {}
