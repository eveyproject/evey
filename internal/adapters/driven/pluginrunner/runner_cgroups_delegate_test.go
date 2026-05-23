package pluginrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAvailableControllers(t *testing.T) {
	assert.Equal(t, []string{"memory", "cpu", "pids", "io"}, availableControllers("cpuset cpu io memory pids"))
	assert.Equal(t, []string{"memory", "pids"}, availableControllers("memory pids"))
	assert.Equal(t, []string{"io"}, availableControllers("cpuset io"))
}

func TestCgroupDelegationEnabled(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv(cgroupDelegateEnv, v)
		assert.True(t, cgroupDelegationEnabled(), v)
	}
	for _, v := range []string{"", "0", "false", "no"} {
		t.Setenv(cgroupDelegateEnv, v)
		assert.False(t, cgroupDelegationEnabled(), v)
	}
}

func TestMoveCgroupProcs(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "cgroup.procs")
	moved, failed := moveCgroupProcs([]int{100, 101, 102}, dst)
	assert.Equal(t, 3, moved)
	assert.Equal(t, 0, failed)

	// A path that cannot be written counts as failed, never fatal.
	moved, failed = moveCgroupProcs([]int{1}, "/proc/nonexistent/cgroup.procs")
	assert.Equal(t, 0, moved)
	assert.Equal(t, 1, failed)
}

func TestSetupCgroupDelegation_HappyPath(t *testing.T) {
	root := t.TempDir()
	overrideCgroupRoot(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset cpu io memory pids\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "cgroup.procs"), []byte("100\n101\n"), 0o644))

	ok := setupCgroupDelegation()
	require.True(t, ok, "delegation should succeed on a writable mock cgroup root")

	// Container processes relocated to a leaf.
	assert.DirExists(t, filepath.Join(root, "evey-init"))
	// Controllers delegated at the root and at the evey-plugins parent.
	rootSubtree, err := os.ReadFile(filepath.Join(root, "cgroup.subtree_control"))
	require.NoError(t, err)
	for _, c := range []string{"+memory", "+cpu", "+pids"} {
		assert.Contains(t, string(rootSubtree), c)
	}
	assert.DirExists(t, filepath.Join(root, "evey-plugins"))
	assert.FileExists(t, filepath.Join(root, "evey-plugins", "cgroup.subtree_control"))
}

func TestSetupCgroupDelegation_NoCgroupV2(t *testing.T) {
	overrideCgroupRoot(t, t.TempDir()) // empty: no cgroup.controllers
	assert.False(t, setupCgroupDelegation(), "must fail-soft when cgroup v2 is absent")
}

func TestSetupCgroupDelegation_MissingControllers(t *testing.T) {
	root := t.TempDir()
	overrideCgroupRoot(t, root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset io\n"), 0o644))
	assert.False(t, setupCgroupDelegation(), "must fail-soft when memory/cpu/pids are unavailable")
}
