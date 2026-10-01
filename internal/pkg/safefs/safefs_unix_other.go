//go:build unix && !linux

package safefs

import "golang.org/x/sys/unix"

// statOpenFlags: no O_PATH here, the non-blocking read open stands in.
const statOpenFlags = unix.O_RDONLY | unix.O_NONBLOCK | unix.O_NOCTTY

func openat2Beneath(int, string, int) (int, bool, error) { return -1, false, nil }
