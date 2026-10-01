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

// specialNodeRights may never be granted on a plugin-writable dir: a symlink
// planted under web_root is followed by the server (secret disclosure), and
// FIFOs/device nodes have no plugin use.
const specialNodeRights = uint64(unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
	unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK)

func TestReadWriteAccess_NoSpecialNodes(t *testing.T) {
	for abi := 1; abi <= 6; abi++ {
		rw := readWriteAccess(abi)
		if got := rw & specialNodeRights; got != 0 {
			t.Errorf("abi %d: rw grant includes special-node rights %#x", abi, got)
		}
		// Still HANDLED, so the kernel denies them instead of ignoring them.
		if supportedFSAccess(abi)&specialNodeRights != specialNodeRights {
			t.Errorf("abi %d: special-node rights must stay in the handled set", abi)
		}
		for _, need := range []uint64{
			unix.LANDLOCK_ACCESS_FS_MAKE_SOCK, // plugin listens on its gRPC socket
			unix.LANDLOCK_ACCESS_FS_MAKE_REG,
			unix.LANDLOCK_ACCESS_FS_MAKE_DIR,
			unix.LANDLOCK_ACCESS_FS_WRITE_FILE,
		} {
			if rw&need == 0 {
				t.Errorf("abi %d: rw grant lost required right %#x", abi, need)
			}
		}
	}
}

// TestLandlock_NoSymlinkOrFifoInPluginDir runs the real launcher under the
// kernel's Landlock: symlink and FIFO creation in the rw dir must be refused
// (EACCES) while a regular file is still created.
func TestLandlock_NoSymlinkOrFifoInPluginDir(t *testing.T) {
	if abi, err := landlockABI(); err != nil || abi < 1 {
		t.Skipf("Landlock unavailable on this kernel (abi=%d err=%v)", abi, err)
	}
	tmp := t.TempDir()
	allowed := filepath.Join(tmp, "plugin")
	if err := os.Mkdir(allowed, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(tmp, "jwt.key")
	if err := os.WriteFile(secret, []byte("k"), 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(shellCmd string) (string, error) {
		args := []string{"-ro", "/usr", "-ro", "/bin", "-ro", "/lib", "-ro", "/lib64", "-ro", "/etc",
			"-rw", allowed, "-require-landlock", "--", "/bin/sh", "-c", shellCmd}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "EVEY_LAUNCHER_REEXEC=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for name, sh := range map[string]string{
		"symlink": "ln -s " + secret + " " + filepath.Join(allowed, "leak"),
		"fifo":    "mkfifo " + filepath.Join(allowed, "fifo"),
	} {
		out, err := run(sh)
		if err == nil || !strings.Contains(out, "Permission denied") {
			t.Errorf("%s: want EACCES, got err=%v output=%q", name, err, out)
		}
	}
	// A hard link from outside is a reparent: Landlock answers EXDEV, not EACCES.
	if out, err := run("ln " + secret + " " + filepath.Join(allowed, "hard")); err == nil {
		t.Errorf("hardlink to an outside file succeeded: %q", out)
	}
	for _, p := range []string{"leak", "fifo", "hard"} {
		if _, err := os.Lstat(filepath.Join(allowed, p)); err == nil {
			t.Errorf("%s was created despite Landlock", p)
		}
	}
	if out, err := run("echo ok > " + filepath.Join(allowed, "file.txt")); err != nil {
		t.Fatalf("regular file creation must still work: err=%v output=%q", err, out)
	}
}
