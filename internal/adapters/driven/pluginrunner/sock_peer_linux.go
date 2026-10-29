//go:build linux

package pluginrunner

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// sockListenerPID returns the pid of the process that listens on the other end
// of a connected unix socket. The kernel records it at listen() time, so a
// plugin cannot forge it.
func sockListenerPID(c net.Conn) (int, error) {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("not a unix socket: %T", c)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return 0, err
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return 0, err
	}
	if credErr != nil {
		return 0, credErr
	}
	return int(cred.Pid), nil
}
