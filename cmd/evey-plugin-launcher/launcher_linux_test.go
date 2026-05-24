//go:build linux

package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestMain lets the test binary impersonate the launcher: when re-exec'd with
// EVEY_LAUNCHER_REEXEC=1 it runs run() (applies Landlock + execs the command),
// otherwise it runs the normal test suite. This exercises the real launcher
// end-to-end without a separate `go build`.
func TestMain(m *testing.M) {
	if os.Getenv("EVEY_LAUNCHER_REEXEC") == "1" {
		os.Exit(run(os.Args[1:]))
	}
	os.Exit(m.Run())
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
// so setuid succeeds without the cap_setuid file capability — the file-cap path
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
}
