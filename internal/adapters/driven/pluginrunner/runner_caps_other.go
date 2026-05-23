//go:build !linux

package pluginrunner

// canSetUIDGID is always false off Linux (no /proc, no Linux capabilities). The
// server only runs in a Linux container in production.
func canSetUIDGID() bool { return false }
