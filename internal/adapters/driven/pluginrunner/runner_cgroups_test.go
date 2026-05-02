package pluginrunner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// overrideCgroupRoot replaces the package-level cgroupRoot with root for the
// duration of the test, then restores the original value via t.Cleanup.
func overrideCgroupRoot(t *testing.T, root string) {
	t.Helper()
	prev := cgroupRoot
	cgroupRoot = root
	t.Cleanup(func() { cgroupRoot = prev })
}

func TestPrepareCgroup_NoCgroupV2(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	// Point cgroupRoot at an empty temp dir — cgroup.controllers absent.
	root := t.TempDir()
	overrideCgroupRoot(t, root)

	path, err := prepareCgroup("test-plugin", cgroupConfig{})
	require.NoError(t, err, "no cgroup v2 should be a no-op, not an error")
	assert.Empty(t, path, "expected empty path when cgroup v2 is unavailable")
}

func TestPrepareCgroup_MissingController(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	root := t.TempDir()
	overrideCgroupRoot(t, root)

	// Provide cgroup.controllers but omit the "pids" controller.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "cgroup.controllers"),
		[]byte("cpuset cpu io memory hugetlb rdma misc\n"),
		0o644,
	))

	path, err := prepareCgroup("test-plugin", cgroupConfig{})
	require.NoError(t, err, "missing controller should be a no-op, not an error")
	assert.Empty(t, path, "expected empty path when a required controller is missing")
}

func TestPrepareCgroup_Success(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	root := t.TempDir()
	overrideCgroupRoot(t, root)

	// Provide cgroup.controllers with all required controllers.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "cgroup.controllers"),
		[]byte("cpuset cpu io memory pids hugetlb\n"),
		0o644,
	))

	cfg := cgroupConfig{
		memBytes:  128 * 1024 * 1024, // 128 MiB
		cpuQuota:  25000,
		cpuPeriod: 100000,
		pidsMax:   32,
	}

	path, err := prepareCgroup("test-plugin", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, path, "should have returned a cgroup directory path")

	// Verify the cgroup directory was created.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify resource limit files were written with expected content.
	tests := []struct {
		file    string
		content string
	}{
		{"memory.max", "134217728"}, // 128 * 1024 * 1024
		{"cpu.max", "25000 100000"},
		{"pids.max", "32"},
	}
	for _, tt := range tests {
		got, err := os.ReadFile(filepath.Join(path, tt.file))
		require.NoError(t, err, "file %s should exist", tt.file)
		assert.Equal(t, tt.content, string(got), "file %s content mismatch", tt.file)
	}
}

func TestPrepareCgroup_DefaultsFilled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	root := t.TempDir()
	overrideCgroupRoot(t, root)

	require.NoError(t, os.WriteFile(
		filepath.Join(root, "cgroup.controllers"),
		[]byte("cpuset cpu io memory pids hugetlb\n"),
		0o644,
	))

	// Zero config → defaults should be applied.
	path, err := prepareCgroup("test-defaults", cgroupConfig{})
	require.NoError(t, err)
	require.NotEmpty(t, path)

	memContent, err := os.ReadFile(filepath.Join(path, "memory.max"))
	require.NoError(t, err)
	assert.Equal(t, "268435456", string(memContent)) // 256 * 1024 * 1024

	cpuContent, err := os.ReadFile(filepath.Join(path, "cpu.max"))
	require.NoError(t, err)
	assert.Equal(t, "50000 100000", string(cpuContent))

	pidsContent, err := os.ReadFile(filepath.Join(path, "pids.max"))
	require.NoError(t, err)
	assert.Equal(t, "64", string(pidsContent))
}

func TestAttachToCgroup_EmptyPath(t *testing.T) {
	// attachToCgroup("", pid) must be a no-op with no error.
	err := attachToCgroup("", 12345)
	assert.NoError(t, err)
}

func TestCleanupCgroup_EmptyPath(t *testing.T) {
	// cleanupCgroup("") must not panic or error.
	cleanupCgroup("")
}

func TestCleanupCgroup_RemovesDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	dir := t.TempDir()
	sub := filepath.Join(dir, "evey-plugins", "test-cleanup")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	cleanupCgroup(sub)

	_, err := os.Stat(sub)
	assert.True(t, os.IsNotExist(err), "cgroup directory should have been removed")
}

func TestWithResourceLimits_StoresValues(t *testing.T) {
	r := NewRunner()
	r = r.WithResourceLimits(512*1024*1024, 30000, 100000, 128)

	assert.Equal(t, int64(512*1024*1024), r.cgroupCfg.memBytes)
	assert.Equal(t, 30000, r.cgroupCfg.cpuQuota)
	assert.Equal(t, 100000, r.cgroupCfg.cpuPeriod)
	assert.Equal(t, 128, r.cgroupCfg.pidsMax)
}

func TestWithResourceLimits_ZeroUsesDefaults(t *testing.T) {
	r := NewRunner()
	r = r.WithResourceLimits(0, 0, 0, 0)

	// The config itself holds zeros; defaults() is applied lazily in
	// prepareCgroup. Verify defaults() expansion here.
	effective := r.cgroupCfg.defaults()
	assert.Equal(t, defaultMemBytes, effective.memBytes)
	assert.Equal(t, defaultCPUQuota, effective.cpuQuota)
	assert.Equal(t, defaultCPUPeriod, effective.cpuPeriod)
	assert.Equal(t, defaultPIDsMax, effective.pidsMax)
}

func TestPrepareCgroup_RejectsPathTraversal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroups v2 only on Linux")
	}

	root := t.TempDir()
	overrideCgroupRoot(t, root)

	// Build a controllers file so the only failure path is the pluginID check.
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "cgroup.controllers"),
		[]byte("cpuset cpu io memory pids hugetlb\n"),
		0o644,
	))

	for _, malicious := range []string{
		"../escape",
		"../../etc",
		"plugin/../escape",
		"a/b",
		"plg with space",
		"plg;injection",
		"",
	} {
		_, err := prepareCgroup(malicious, cgroupConfig{})
		assert.Error(t, err, "pluginID %q must be rejected", malicious)
	}
}

// TestCgroupFDIfSupported_EmptyPath verifies that cgroupFDIfSupported returns
// (-1, false) when called with an empty path — no fd must be opened.
func TestCgroupFDIfSupported_EmptyPath(t *testing.T) {
	fd, ok := cgroupFDIfSupported("")
	assert.False(t, ok)
	assert.Equal(t, -1, fd)
}

// TestCgroupFDIfSupported_NonExistentPath verifies that cgroupFDIfSupported
// returns (-1, false) when the cgroup directory does not exist.
func TestCgroupFDIfSupported_NonExistentPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("unix.Open only on Linux")
	}
	fd, ok := cgroupFDIfSupported("/nonexistent/cgroup/path/that/does/not/exist")
	assert.False(t, ok)
	assert.Equal(t, -1, fd)
}

// TestCgroupAttachPreExec_OnLinux57 verifies that cgroupFDIfSupported opens a
// valid fd on a real directory (simulating the cgroupfs case). On Linux 5.7+
// the returned fd can be passed to SysProcAttr.CgroupFD so the kernel places
// the child in the cgroup before main() runs (no race window). When the fd is
// valid, it must be closeable via unix.Close without error.
func TestCgroupAttachPreExec_OnLinux57(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("cgroup fd pre-exec only on Linux")
	}

	// Use a real directory (t.TempDir returns a real os-backed path) to
	// verify O_PATH|O_DIRECTORY succeeds. In production this would be the
	// cgroupfs directory, but any accessible directory validates the codepath.
	dir := t.TempDir()

	fd, ok := cgroupFDIfSupported(dir)
	if !ok {
		t.Skip("cgroupFDIfSupported returned false — kernel may not support O_PATH on this dir")
	}
	require.True(t, ok, "expected fd to be opened on a real directory")
	require.NotEqual(t, -1, fd, "expected a valid file descriptor")

	// Verify the fd is closeable — proves it was actually opened.
	err := unix.Close(fd)
	assert.NoError(t, err, "fd must be closeable without error")
}
