package procfork

import (
	"os/exec"
	"syscall"
)

// DieWithParent makes the kernel SIGKILL cmd's process when the server dies.
// In the image the server is not PID 1: docker/entrypoint.sh is, and it
// relaunches the server after an admin restart. A server that crashed or was
// OOM-killed left its ffmpegs reparented to that shell, still encoding next
// to the new server's. The prlimit/ionice/nice prefixes exec in place, so the
// signal reaches the ffmpeg itself.
//
// The kernel ties it to the THREAD that forked: Go keeps its threads alive
// unless a goroutine exits while locked to one, which no caller does.
func DieWithParent(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Pdeathsig = syscall.SIGKILL
}
