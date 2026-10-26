//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestMain lets the test binary impersonate the launcher: when re-exec'd with
// EVEY_LAUNCHER_REEXEC=1 it runs run() (applies Landlock + execs the command),
// otherwise it runs the normal test suite. This exercises the real launcher
// end-to-end without a separate `go build`.
func TestMain(m *testing.M) {
	// Checked FIRST and via argv, not env: the launcher execs the probe with its
	// own environment, so an env marker would also match the launcher process
	// itself. argv[1] tells them apart unambiguously (the launcher's own argv
	// always starts with a flag).
	if len(os.Args) > 1 && os.Args[1] == forkProbeArg {
		os.Exit(forkProbeMain())
	}
	if os.Getenv("EVEY_LAUNCHER_REEXEC") == "1" {
		if os.Getenv("EVEY_LAUNCHER_SECCOMP_FAILS") == "1" {
			installSeccomp = func(bool, bool) error { return errors.New("simulated seccomp failure") }
		}
		os.Exit(run(os.Args[1:]))
	}
	if os.Getenv("EVEY_PDEATHSIG_HELPER") == "1" {
		os.Exit(pdeathsigHelperMain())
	}
	os.Exit(m.Run())
}

// pdeathsigHelperMain plays the plugin-runner parent: it spawns the launcher
// (re-exec of this binary, via the world-executable copy in
// EVEY_LAUNCHER_BIN since the dropped uid cannot read the go-build cache)
// with Pdeathsig+Setpgid exactly like the runner does, prints the child pid
// on stdout, then exits WITHOUT killing the child. The outer test then
// checks whether parent death reached the plugin.
func pdeathsigHelperMain() int {
	launcherBin := os.Getenv("EVEY_LAUNCHER_BIN")
	if launcherBin == "" {
		launcherBin = os.Args[0]
	}
	// Mirror the production argv shape (buildLauncherArgv): the seccomp filter
	// is always installed for real plugins, and it is precisely what loses the
	// runner-armed pdeathsig across execve - without -seccomp this scenario
	// passes even unfixed.
	args := []string{
		"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-ro", "/etc",
		"-seccomp",
		"--", "/bin/sleep", "300",
	}
	cmd := exec.Command(launcherBin, args...)
	cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	// A root server drops the plugin to the evey-plugin uid (pluginSysProcAttr);
	// the uid change is part of the losing combination (uid drop + seccomp), so
	// reproduce it whenever we can (the make-test container runs as root).
	if os.Getuid() == 0 {
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 1001, Gid: 1001}
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "helper: start launcher: %v\n", err)
		return 1
	}
	fmt.Printf("childpid=%d\n", cmd.Process.Pid)
	// Give the launcher time to reach execve so the test exercises the
	// post-exec process, not the launcher's own setup window.
	time.Sleep(700 * time.Millisecond)
	return 0 // die; pdeathsig should now kill the plugin
}

// TestLauncher_ParentDeathSignalReachesPlugin locks the launcher's core
// lifecycle contract: a plugin spawned through the launcher must die when the
// server process dies. The production launcher carries the cap_setuid file
// capability (Dockerfile: setcap cap_setuid+ep), and the kernel CLEARS the
// runner-armed PR_SET_PDEATHSIG when execve'ing a binary with file caps
// (prctl(2)) - so the launcher must re-arm it on the exec'ing thread right
// before exec'ing the plugin. Regression: orphaned node plugin processes
// piling up across server restarts (~25 MB each).
func TestLauncher_ParentDeathSignalReachesPlugin(t *testing.T) {
	// The dropped plugin uid (1001) must be able to exec the launcher; the
	// go-build cache holding os.Args[0] is 0700, so re-exec a copy from a
	// world-traversable temp dir.
	launcherBin := copyToWorldExecutable(t, os.Args[0])

	// Reproduce the production trigger: file caps on the launcher binary make
	// the kernel clear the inherited pdeathsig at execve. Without them the
	// attribute happens to survive and the scenario can pass unfixed.
	setcap, err := exec.LookPath("setcap")
	if err != nil {
		t.Skip("setcap not available; cannot reproduce the file-caps pdeathsig clearing")
	}
	if os.Getuid() != 0 {
		t.Skip("needs root to setcap the launcher copy")
	}
	if out, capErr := exec.Command(setcap, "cap_setuid+ep", launcherBin).CombinedOutput(); capErr != nil {
		t.Skipf("setcap failed (%v: %s); cannot reproduce the file-caps trigger", capErr, out)
	}

	helper := exec.Command(os.Args[0])
	helper.Env = append(os.Environ(), "EVEY_PDEATHSIG_HELPER=1", "EVEY_LAUNCHER_BIN="+launcherBin)
	// Hand the helper (and transitively the plugin) the test's REAL stderr, not
	// a capture pipe: a surviving orphan would hold a pipe open and wedge
	// Output() until the go test timeout instead of failing fast below.
	helper.Stderr = os.Stderr
	out, err := helper.Output()
	if err != nil {
		t.Fatalf("helper failed: %v (output=%q)", err, out)
	}
	var childPid int
	for line := range strings.SplitSeq(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "childpid="); ok {
			childPid, _ = strconv.Atoi(rest)
		}
	}
	if childPid <= 0 {
		t.Fatalf("helper did not report a child pid (output=%q)", out)
	}
	// The helper has exited (Output waits). The plugin must now receive the
	// parent-death signal; poll briefly to absorb delivery scheduling.
	defer syscall.Kill(-childPid, syscall.SIGKILL) // cleanup either way
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(childPid, 0) != nil {
			return // died with its parent: contract holds
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("plugin process %d survived parent death: PR_SET_PDEATHSIG was lost across the launcher exec", childPid)
}

// copyToWorldExecutable copies bin into a fresh 0755 temp dir and returns the
// 0755 copy's path, cleaned up with the test.
func copyToWorldExecutable(t *testing.T, bin string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "evey-launcher-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	if chmodErr := os.Chmod(dir, 0o755); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "launcher")
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestSupportedFSAccess_PerABI(t *testing.T) {
	v1 := supportedFSAccess(1)
	if v1&unix.LANDLOCK_ACCESS_FS_REFER != 0 {
		t.Error("abi 1 must not include REFER (added in v2)")
	}
	if v1&unix.LANDLOCK_ACCESS_FS_TRUNCATE != 0 {
		t.Error("abi 1 must not include TRUNCATE (added in v3)")
	}
	if supportedFSAccess(2)&unix.LANDLOCK_ACCESS_FS_REFER == 0 {
		t.Error("abi 2 must include REFER")
	}
	if supportedFSAccess(3)&unix.LANDLOCK_ACCESS_FS_TRUNCATE == 0 {
		t.Error("abi 3 must include TRUNCATE")
	}
	if supportedFSAccess(4)&unix.LANDLOCK_ACCESS_FS_IOCTL_DEV != 0 {
		t.Error("abi 4 must not include IOCTL_DEV (added in v5)")
	}
	if supportedFSAccess(5)&unix.LANDLOCK_ACCESS_FS_IOCTL_DEV == 0 {
		t.Error("abi 5 must include IOCTL_DEV")
	}
}

func TestReadExecAccess_NoWriteBits(t *testing.T) {
	for abi := 1; abi <= 5; abi++ {
		a := readExecAccess(abi)
		for _, w := range []uint64{
			unix.LANDLOCK_ACCESS_FS_WRITE_FILE,
			unix.LANDLOCK_ACCESS_FS_MAKE_REG,
			unix.LANDLOCK_ACCESS_FS_REMOVE_FILE,
			unix.LANDLOCK_ACCESS_FS_MAKE_DIR,
		} {
			if a&w != 0 {
				t.Errorf("abi %d: read-only access must not grant write bit %#x", abi, w)
			}
		}
		if a&unix.LANDLOCK_ACCESS_FS_READ_FILE == 0 {
			t.Errorf("abi %d: read-only access must grant READ_FILE", abi)
		}
	}
}

// TestLandlock_EndToEnd verifies the real syscall path: a directory on the
// allowlist is readable/writable, anything outside it is denied.
func TestLandlock_EndToEnd(t *testing.T) {
	if abi, err := landlockABI(); err != nil || abi < 1 {
		t.Skipf("Landlock unavailable on this kernel (abi=%d err=%v)", abi, err)
	}

	tmp := t.TempDir()
	allowed := filepath.Join(tmp, "allowed")
	if err := os.Mkdir(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	okFile := filepath.Join(allowed, "ok.txt")
	if err := os.WriteFile(okFile, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(secret, []byte("top-secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// System dirs the shell + cat need to start. Missing ones are skipped.
	sysRO := []string{"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-ro", "/etc"}

	reexec := func(t *testing.T, shellCmd string) (string, error) {
		args := append([]string{}, sysRO...)
		args = append(args, "-rw", allowed, "-require-landlock", "--", "/bin/sh", "-c", shellCmd)
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("denies file outside allowlist", func(t *testing.T) {
		out, err := reexec(t, "cat "+secret)
		if err == nil {
			t.Fatalf("expected failure reading %s, got success; output=%q", secret, out)
		}
		if strings.Contains(out, "top-secret") {
			t.Fatalf("secret content leaked despite Landlock: %q", out)
		}
	})

	t.Run("allows file inside allowlist", func(t *testing.T) {
		out, err := reexec(t, "cat "+okFile)
		if err != nil {
			t.Fatalf("expected success reading %s, got err=%v output=%q", okFile, err, out)
		}
		if !strings.Contains(out, "hello") {
			t.Fatalf("expected allowed file content, got %q", out)
		}
	})

	t.Run("allows write inside allowlist", func(t *testing.T) {
		out, err := reexec(t, "echo written > "+filepath.Join(allowed, "new.txt"))
		if err != nil {
			t.Fatalf("expected write success, got err=%v output=%q", err, out)
		}
	})
}

func TestSupportedNetAccess_PerABI(t *testing.T) {
	for abi := 1; abi <= 3; abi++ {
		if supportedNetAccess(abi) != 0 {
			t.Errorf("abi %d: network access landed in v4, must be 0", abi)
		}
	}
	want := uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)
	if supportedNetAccess(4) != want {
		t.Errorf("abi 4: got %#x want %#x", supportedNetAccess(4), want)
	}
}

// TestLandlock_NetEgress verifies real TCP egress filtering. Needs Landlock ABI
// v4 (kernel ≥6.7) and python3, so it skips on older kernels (e.g. a v3 dev box)
// and runs on the deploy host.
func TestLandlock_NetEgress(t *testing.T) {
	if abi, err := landlockABI(); err != nil || abi < 4 {
		t.Skipf("Landlock network control needs ABI v4 (abi=%d err=%v)", abi, err)
	}
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port

	sysRO := []string{"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-ro", "/etc"}
	pyConnect := "import socket,sys; s=socket.socket(); s.settimeout(2); s.connect(('127.0.0.1'," +
		strconv.Itoa(port) + ")); s.close()"

	reexec := func(extra ...string) (string, error) {
		args := append([]string{}, sysRO...)
		args = append(args, "-rw", t.TempDir(), "-restrict-net")
		args = append(args, extra...)
		args = append(args, "-require-landlock", "--", py, "-c", pyConnect)
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("denies connect when port not allowed", func(t *testing.T) {
		out, err := reexec()
		if err == nil {
			t.Fatalf("expected connect to be denied, got success; output=%q", out)
		}
	})

	t.Run("allows connect to whitelisted port", func(t *testing.T) {
		out, err := reexec("-net-connect", strconv.Itoa(port))
		if err != nil {
			t.Fatalf("expected connect to succeed on allowed port, got err=%v output=%q", err, out)
		}
	})
}

func TestRun_MissingSeparator(t *testing.T) {
	if code := run([]string{"-ro", "/usr"}); code != 2 {
		t.Errorf("missing '--' should return 2, got %d", code)
	}
}

func TestRun_NoCommand(t *testing.T) {
	if code := run([]string{"-ro", "/usr", "--"}); code != 2 {
		t.Errorf("no command after '--' should return 2, got %d", code)
	}
}

// TestDropPrivileges_ReexecDropsUID re-execs the launcher with -uid/-gid and
// verifies the plugin runs as the dropped uid and loses access to a 0600 file
// owned by the original uid. Needs root (the make-test container runs as root)
// so setuid succeeds without the cap_setuid file capability - the file-cap path
// is covered by the docker PoC; this locks the launcher's drop logic + guards.
func TestDropPrivileges_ReexecDropsUID(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("needs root to setuid without the cap_setuid file capability")
	}
	tmp := t.TempDir()
	if err := os.Chmod(tmp, 0o755); err != nil { // let the dropped uid traverse to the file
		t.Fatal(err)
	}
	secret := filepath.Join(tmp, "secret")
	if err := os.WriteFile(secret, []byte("top-secret-jwt"), 0o600); err != nil {
		t.Fatal(err)
	}

	reexec := func(args ...string) (string, error) {
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	t.Run("drops to uid 1001 and loses access to the 0600 secret", func(t *testing.T) {
		out, err := reexec("-uid", "1001", "-gid", "1000", "--",
			"/bin/sh", "-c", "id -u; cat "+secret+" 2>&1 || true")
		if err != nil {
			t.Fatalf("launcher failed: %v out=%q", err, out)
		}
		if !strings.Contains(out, "1001") {
			t.Errorf("expected plugin to run as uid 1001, got %q", out)
		}
		if strings.Contains(out, "top-secret-jwt") {
			t.Errorf("0600 secret leaked to the dropped uid: %q", out)
		}
	})

	t.Run("refuses an unsafe service uid below 1000", func(t *testing.T) {
		out, err := reexec("-uid", "500", "-gid", "500", "--",
			"/bin/sh", "-c", "echo RAN")
		if err == nil {
			t.Errorf("expected the launcher to refuse uid 500, got success out=%q", out)
		}
		if strings.Contains(out, "RAN") {
			t.Errorf("plugin ran despite an unsafe uid: %q", out)
		}
	})

	t.Run("best-effort: a drop failure warns and runs same-uid instead of refusing", func(t *testing.T) {
		// dropPrivileges refuses uid 500; with -uid-besteffort the launcher warns
		// and runs the command as the original uid (no setuid happened → no
		// escalation) rather than exiting - so a host that can't drop keeps working.
		out, err := reexec("-uid", "500", "-gid", "500", "-uid-besteffort", "--",
			"/bin/sh", "-c", "echo RAN")
		if err != nil {
			t.Fatalf("best-effort must not fail the launcher: %v out=%q", err, out)
		}
		if !strings.Contains(out, "RAN") {
			t.Errorf("command should still run in best-effort mode: %q", out)
		}
		if !strings.Contains(out, "WARNING") {
			t.Errorf("best-effort degradation should warn loudly: %q", out)
		}
	})
}

// TestRun_SeccompFailureIsFatalUnderRequireLandlock: the fail-closed contract
// of -require-landlock covers the syscall layer too; a launcher that could not
// install its seccomp filter must not exec the plugin with Landlock alone.
func TestRun_SeccompFailureIsFatalUnderRequireLandlock(t *testing.T) {
	if abi, err := landlockABI(); err != nil || abi < 1 {
		t.Skipf("Landlock unavailable on this kernel (abi=%d err=%v)", abi, err)
	}
	for _, tc := range []struct {
		name     string
		require  bool
		wantExit int
	}{
		{"require-landlock refuses", true, 1},
		{"best effort still runs", false, 0},
	} {
		args := []string{"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-seccomp"}
		if tc.require {
			args = append(args, "-require-landlock")
		}
		cmd := exec.Command(os.Args[0], append(args, "--", "/bin/true")...)
		cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1", "EVEY_LAUNCHER_SECCOMP_FAILS=1")
		out, err := cmd.CombinedOutput()
		code := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: run launcher: %v", tc.name, err)
		}
		if code != tc.wantExit {
			t.Errorf("%s: exit %d, want %d\n%s", tc.name, code, tc.wantExit, out)
		}
		if !strings.Contains(string(out), "simulated seccomp failure") {
			t.Errorf("%s: the seccomp failure must be reported\n%s", tc.name, out)
		}
	}
}
