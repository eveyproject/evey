package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestDirOwnerResolvesTheDirectoryOwner(t *testing.T) {
	dir := t.TempDir()
	uid, gid, err := dirOwner(dir)
	if err != nil {
		t.Fatal(err)
	}
	if uid != os.Getuid() || gid != os.Getgid() {
		t.Errorf("dirOwner = %d:%d, want %d:%d", uid, gid, os.Getuid(), os.Getgid())
	}
	if _, _, err := dirOwner(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing dir must fail")
	}
}

func TestHandBackToDirOwnerIsNoOpWhenAlreadyOwned(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "b.tar.gz")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	handBackToDirOwner(dir)
	handBackToDirOwner("")
	fi, err := os.Stat(f)
	if err != nil {
		t.Fatal(err)
	}
	if st := fi.Sys().(*syscall.Stat_t); int(st.Uid) != os.Getuid() {
		t.Errorf("owner changed to %d", st.Uid)
	}
}

func TestHandBackToDirOwnerChownsFilesWhenRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to chown")
	}
	dir := t.TempDir()
	if err := os.Chown(dir, 1000, 1000); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "b.json")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	handBackToDirOwner(dir)
	fi, _ := os.Stat(f)
	if st := fi.Sys().(*syscall.Stat_t); st.Uid != 1000 || st.Gid != 1000 {
		t.Errorf("owner = %d:%d, want 1000:1000", st.Uid, st.Gid)
	}
}
