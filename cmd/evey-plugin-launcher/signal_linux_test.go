//go:build linux

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })
	return cmd
}

// The server asks the launcher to signal a plugin it cannot reach itself (a
// plugin under its own uid, no CAP_KILL): the launcher takes that uid and
// signals the plugin's group.
func TestSignalSubcommand_SignalsATargetOfItsUID(t *testing.T) {
	if os.Getuid() < 1000 {
		t.Skip("needs a regular uid (the subcommand refuses system uids)")
	}
	cmd := startSleeper(t)
	code := run([]string{"signal", "-uid", strconv.Itoa(os.Getuid()), "-gid", strconv.Itoa(os.Getgid()),
		"-pid", strconv.Itoa(cmd.Process.Pid), "-sig", "KILL"})
	if code != 0 {
		t.Fatalf("signal subcommand exit %d", code)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		ws, _ := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if err == nil || !ws.Signaled() || ws.Signal() != syscall.SIGKILL {
			t.Fatalf("target not killed: err=%v status=%v", err, ws)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("target still running after the signal subcommand")
	}
}

// It never becomes a general kill(1): only TERM/KILL, only a real pid, only
// a process of the uid it took, never a system uid.
func TestSignalSubcommand_Refusals(t *testing.T) {
	if os.Getuid() < 1000 {
		t.Skip("needs a regular uid")
	}
	cmd := startSleeper(t)
	uid, gid, pid := strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid()), strconv.Itoa(cmd.Process.Pid)
	for name, args := range map[string][]string{
		"pid 1":       {"-uid", uid, "-gid", gid, "-pid", "1", "-sig", "KILL"},
		"pid 0":       {"-uid", uid, "-gid", gid, "-pid", "0", "-sig", "KILL"},
		"negative":    {"-uid", uid, "-gid", gid, "-pid", "-" + pid, "-sig", "KILL"},
		"other sig":   {"-uid", uid, "-gid", gid, "-pid", pid, "-sig", "HUP"},
		"no uid":      {"-gid", gid, "-pid", pid, "-sig", "KILL"},
		"system uid":  {"-uid", "999", "-gid", gid, "-pid", pid, "-sig", "KILL"},
		"stray flags": {"-uid", uid, "-gid", gid, "-pid", pid, "-sig", "KILL", "--", "sh"},
	} {
		if code := run(append([]string{"signal"}, args...)); code == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	if cmd.ProcessState != nil {
		t.Fatal("a refused call signalled the target")
	}
	if err := syscall.Kill(cmd.Process.Pid, 0); err != nil {
		t.Fatalf("a refused call signalled the target: %v", err)
	}

	// A process of another uid is refused before any signal is attempted.
	if other := foreignPid(); other > 1 {
		if code := run([]string{"signal", "-uid", uid, "-gid", gid, "-pid", strconv.Itoa(other), "-sig", "TERM"}); code == 0 {
			t.Errorf("pid %d of another uid: accepted", other)
		}
	}
}

// foreignPid returns a live process that does not belong to this uid (0 if none).
func foreignPid() int {
	entries, _ := os.ReadDir("/proc")
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}
		if owner, err := procRealUID(pid); err == nil && owner != os.Getuid() {
			return pid
		}
	}
	return 0
}
