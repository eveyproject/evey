//go:build !linux

package pluginrunner

import (
	"errors"
	"net"
)

// sockListenerPID needs SO_PEERCRED, which only Linux has; plugins only run
// there.
func sockListenerPID(net.Conn) (int, error) {
	return 0, errors.New("plugin socket peer check needs linux")
}
