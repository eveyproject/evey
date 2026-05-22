//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
