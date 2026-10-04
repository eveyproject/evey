//go:build !linux

package loadtest

// mergerfsBranchOf has no xattr to ask on this platform: the branch is unknown.
func mergerfsBranchOf(string) string { return "" }
