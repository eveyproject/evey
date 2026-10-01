//go:build !linux

package main

import (
	"fmt"
	"os"
)

// runSignal is Linux-only, like the uid drop it relies on.
func runSignal(_ []string) int {
	fmt.Fprintln(os.Stderr, "evey-plugin-launcher signal: linux only")
	return 2
}
