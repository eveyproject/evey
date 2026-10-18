package procfork

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const deathsigParentEnv = "PROCFORK_DEATHSIG_PARENT"

// The parent half: start a child through Command, print its pid, hang until killed.
func TestDeathsigParentHelper(t *testing.T) {
	if os.Getenv(deathsigParentEnv) != "1" {
		t.Skip("helper process for TestCommand_ChildDiesWithServer")
	}
	cmd, _, err := Command(context.Background(), Options{Class: Probe, Site: "test.deathsig"}, "sleep", "60")
	if err != nil {
		fmt.Println("ERR", err)
		os.Exit(2)
	}
	if err := cmd.Start(); err != nil {
		fmt.Println("ERR", err)
		os.Exit(2)
	}
	fmt.Println(cmd.Process.Pid)
	select {}
}

// A server that crashes must not leave its ffmpegs behind: the entrypoint
// shell, not the server, is PID 1 and would adopt them.
func TestCommand_ChildDiesWithServer(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not in PATH")
	}
	parent := exec.Command(os.Args[0], "-test.run=^TestDeathsigParentHelper$") //nolint:gosec // the test binary itself
	parent.Env = append(os.Environ(), deathsigParentEnv+"=1")
	out, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = parent.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		_ = parent.Process.Kill()
		t.Fatalf("read child pid: %v", err)
	}
	child, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		_ = parent.Process.Kill()
		t.Fatalf("helper said %q", line)
	}
	if !alive(child) {
		t.Fatalf("child %d not running before the parent died", child)
	}

	_ = parent.Process.Signal(syscall.SIGKILL)
	_ = parent.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for alive(child) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(child, syscall.SIGKILL)
			t.Fatalf("child %d survived its parent", child)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// alive is false once the pid is gone or a zombie waiting for its new parent.
func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	return len(fields) > 0 && fields[0] != "Z"
}
