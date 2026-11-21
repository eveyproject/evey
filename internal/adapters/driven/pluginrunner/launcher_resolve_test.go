package pluginrunner

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newLauncherPlugin returns a runner over a fresh plugins dir and a plugin
// installed in it holding the given files.
func newLauncherPlugin(t *testing.T, entrypoint string, files ...string) (*Runner, *plugin.Plugin) {
	t.Helper()
	pluginsDir := t.TempDir()
	installDir := filepath.Join(pluginsDir, "plg-l")
	require.NoError(t, os.MkdirAll(installDir, 0o755))
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(installDir, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(installDir, f), []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test plugin file
	}
	return NewRunner(pluginsDir), &plugin.Plugin{ID: "plg-l", Entrypoint: entrypoint, InstallDir: installDir}
}

// Any launcher on PATH runs the plugin: "sh" stands for java or dotnet. Flags
// pass through, the plugin's own file is resolved inside its install dir.
func TestResolveCommand_LauncherFromImage(t *testing.T) {
	r, p := newLauncherPlugin(t, "sh -e plugin.jar --port=1", "plugin.jar")
	name, args, err := r.resolveCommand(p)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(name), name)
	assert.Equal(t, "sh", filepath.Base(name))
	assert.Equal(t, []string{"-e", filepath.Join(p.InstallDir, "plugin.jar"), "--port=1"}, args)
}

func TestResolveCommand_LauncherMissing(t *testing.T) {
	r, p := newLauncherPlugin(t, "evey-no-such-jre -jar plugin.jar", "plugin.jar")
	_, _, err := r.resolveCommand(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `runtime "evey-no-such-jre" not installed`)
}

// The plugin's own file wins over a launcher of the same name: it runs as the
// binary it is, confined like any binary entrypoint.
func TestResolveCommand_ShippedFileWinsOverLauncher(t *testing.T) {
	r, p := newLauncherPlugin(t, "sh --serve", "sh")
	name, args, err := r.resolveCommand(p)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(p.InstallDir, "sh"), name)
	assert.Equal(t, []string{"--serve"}, args)
}

func TestResolveCommand_LauncherArgsConfined(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "x.jar")
	require.NoError(t, os.WriteFile(outside, nil, 0o600))
	for _, ep := range []string{
		"sh ../other/plugin.jar",
		"sh -cp=../x.jar",
		"sh " + outside,
	} {
		r, p := newLauncherPlugin(t, ep, "plugin.jar")
		_, _, err := r.resolveCommand(p)
		require.Error(t, err, ep)
	}
	// ".." inside a word that is no path element is no traversal.
	r, p := newLauncherPlugin(t, "sh --range=1..5", "plugin.jar")
	_, _, err := r.resolveCommand(p)
	require.NoError(t, err)
}

// An absolute launcher must sit in a PATH directory: an executable elsewhere
// in the image is no launcher.
func TestLookupLauncher_AbsolutePath(t *testing.T) {
	sh, err := filepath.Abs(mustLookPath(t, "sh"))
	require.NoError(t, err)
	got, ok := lookupLauncher(sh, nil)
	assert.True(t, ok)
	assert.Equal(t, sh, got)

	elsewhere := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test binary
	_, ok = lookupLauncher(elsewhere, nil)
	assert.False(t, ok)
}

// A runtime installed outside the system dirs is granted its install root and
// the /etc directories its tree links into (the Debian JRE layout), never a
// file at the root of /etc nor /etc/ssh.
func TestComputeRuntimeReadPaths_RuntimeTree(t *testing.T) {
	etcDir := anyEtcDir(t)
	addons := t.TempDir()
	allowRuntimesBelow(t, addons)
	root := filepath.Join(addons, "jvm", "jre-x")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "conf", "security"), 0o755))
	bin := filepath.Join(root, "bin", "java")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test binary
	require.NoError(t, os.Symlink(filepath.Join(etcDir, "some.conf"), filepath.Join(root, "conf", "some.conf")))
	require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(root, "conf", "passwd")))
	require.NoError(t, os.Symlink("/etc/ssh/ssh_host_ed25519_key", filepath.Join(root, "conf", "security", "key")))

	// Reached through a link, as /usr/bin/java reaches the JRE.
	link := filepath.Join(t.TempDir(), "java")
	require.NoError(t, os.Symlink(bin, link))

	got, err := computeRuntimeReadPaths(link, []string{"/data/plugins"})
	require.NoError(t, err)
	assert.Contains(t, got, root)
	assert.Contains(t, got, etcDir)
	assert.NotContains(t, got, "/etc/passwd")
	assert.NotContains(t, got, "/etc/ssh")
	assert.True(t, slices.IsSorted(got), "%v", got)
}

// A launcher of a shared tree (/usr/bin/sh) gets nothing: /usr is never walked.
func TestComputeRuntimeReadPaths_SharedTreeNotWalked(t *testing.T) {
	sh, err := filepath.EvalSymlinks(mustLookPath(t, "sh"))
	require.NoError(t, err)
	paths, err := computeRuntimeReadPaths(sh, []string{"/data/plugins"})
	require.NoError(t, err)
	for _, p := range paths {
		assert.NotEqual(t, "/usr", p)
		assert.False(t, insideAny(p, boundSystemDirs), p)
	}
}

// A runtime whose install root would open more than itself is refused: one
// outside the add-on prefix (a data directory, a home), one whose root holds
// the plugin directories or sits in a plugin's own, one holding the server.
func TestComputeRuntimeReadPaths_RefusesAnOverreachingRoot(t *testing.T) {
	launcherAt := func(t *testing.T, dir string) string {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "bin"), 0o755))
		bin := filepath.Join(dir, "bin", "java")
		require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755)) //nolint:gosec // test binary
		return bin
	}

	t.Run("outside the add-on prefix", func(t *testing.T) {
		_, err := computeRuntimeReadPaths(launcherAt(t, filepath.Join(t.TempDir(), "data", "jre")), nil)
		require.ErrorContains(t, err, "not below")
	})

	addons := t.TempDir()
	allowRuntimesBelow(t, addons)
	site := filepath.Join(addons, "evey")
	plugins := filepath.Join(site, "plugins")
	require.NoError(t, os.MkdirAll(plugins, 0o755))

	t.Run("root holds the plugin directories", func(t *testing.T) {
		_, err := computeRuntimeReadPaths(launcherAt(t, site), []string{plugins})
		require.ErrorContains(t, err, "overlaps")
	})
	t.Run("root inside a plugin directory", func(t *testing.T) {
		_, err := computeRuntimeReadPaths(launcherAt(t, filepath.Join(plugins, "plg-other", "jre")), []string{plugins})
		require.ErrorContains(t, err, "overlaps")
	})
	t.Run("root holds the server's working directory", func(t *testing.T) {
		wd, err := os.Getwd()
		require.NoError(t, err)
		allowRuntimesBelow(t, filepath.Dir(filepath.Dir(wd)))
		_, err = runtimeRootExposableErr(filepath.Dir(wd))
		require.ErrorContains(t, err, "overlaps")
	})
	t.Run("the prefix itself is no runtime root", func(t *testing.T) {
		_, err := runtimeRootExposableErr(addons)
		require.ErrorContains(t, err, "not below")
	})
	t.Run("a runtime of its own below the prefix passes", func(t *testing.T) {
		got, err := computeRuntimeReadPaths(launcherAt(t, filepath.Join(addons, "jre-21")), []string{plugins})
		require.NoError(t, err)
		assert.Contains(t, got, filepath.Join(addons, "jre-21"))
	})
	// A refused launcher is no launcher: install and start both stop there.
	t.Run("lookupLauncher refuses it", func(t *testing.T) {
		bin := launcherAt(t, site)
		t.Setenv("PATH", filepath.Dir(bin))
		_, ok := lookupLauncher("java", []string{plugins})
		assert.False(t, ok)
	})
}

func runtimeRootExposableErr(root string) (struct{}, error) {
	return struct{}{}, runtimeRootExposable(root, nil)
}

// allowRuntimesBelow stands a test directory in for /opt.
func allowRuntimesBelow(t *testing.T, prefix string) {
	t.Helper()
	saved := runtimeRootPrefixes
	runtimeRootPrefixes = []string{prefix}
	t.Cleanup(func() { runtimeRootPrefixes = saved })
}

// Both sandboxes see the launcher's read paths; a binary the plugin ships
// never gets any (nothing is recorded for it).
func TestLauncherReadPaths_ReachBothSandboxes(t *testing.T) {
	launcher := "/opt/evey-test-runtime/bin/run"
	runtimeReadPaths.Store(launcher, []string{"/opt/evey-test-runtime"})
	t.Cleanup(func() { runtimeReadPaths.Delete(launcher) })

	p := &plugin.Plugin{ID: "plg-l", InstallDir: "/data/plugins/plg-l"}
	argv := buildLauncherArgv("/bin/true", p, exec.Command(launcher, "x"), true, 0, 0)
	assert.Subset(t, pairs(argv, "-ro"), []string{"/opt/evey-test-runtime"})
	// The plugin's /proc directory is never granted, only the listed files.
	assert.NotContains(t, pairs(argv, "-ro"), "/proc/self")
	assert.Equal(t, safeProcFiles, pairs(argv, "-ro-file"))

	prefix := withBwrapReadOnly(buildBwrapCmd("/data/plugins/plg-l", "", sandboxModeBubblewrap), launcherReadPaths(launcher))
	assert.Equal(t, "--", prefix[len(prefix)-1])
	assert.Subset(t, pairs(prefix, "--ro-bind-try"), []string{"/opt/evey-test-runtime"})

	assert.Nil(t, launcherReadPaths("/data/plugins/plg-l/plugin"))
}

func pairs(argv []string, flag string) []string {
	var out []string
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag {
			out = append(out, argv[i+1])
		}
	}
	return out
}

func anyEtcDir(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir("/etc")
	require.NoError(t, err)
	for _, e := range entries {
		if e.IsDir() && !etcDirsNeverExposed["/etc/"+e.Name()] {
			return "/etc/" + e.Name()
		}
	}
	t.Skip("no directory in /etc")
	return ""
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	require.NoError(t, err)
	return p
}
