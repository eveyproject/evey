//go:build linux

package pluginrunner

// Backend red-team regression test. The sandbox-escape fixtures in
// tests/plugins/sandbox-escape/ pass the static analyzer and then attempt a
// real escape at runtime, logging "[ESCAPE] <probe>: BLOCKED|BYPASSED". This
// test runs the filesystem/network/process/exec fixtures through the REAL
// launcher argv (buildLauncherArgv) + LD_PRELOAD wiring the runner uses, parses
// the verdicts, and asserts they are BLOCKED - so a regression that re-opens an
// escape fails CI rather than only showing up in prod logs.
//
// What is asserted where:
//   - seccomp layers (network -deny-inet, kill(2)) and the LD_PRELOAD execve
//     denial are portable, so those verdicts are asserted on any Linux kernel.
//   - Landlock-enforced verdicts (filesystem reads, /proc) are gated behind
//     landlockAvailable(): asserted on a Landlock-capable kernel (CI/prod),
//     skipped (logged) on a kernel without Landlock so the test never
//     false-fails on, e.g., a WSL2 dev kernel.
//
// Prerequisites: a `node` runtime the sandbox lets run (the fixtures are
// Node.js, builtins only - no node_modules), i.e. installed under the
// read-only paths the launcher allowlists (/usr, /bin, ...): a node under a
// home directory is refused exec by Landlock and every fixture would print
// nothing. And `cc` for the LD_PRELOAD shim the execve probes need. Locally,
// a missing prerequisite skips what it would leave unasserted, with the
// reason; in CI (CI set) it fails instead, so the suite cannot go green on a
// runner where the sandbox was never exercised. Run it where both exist, e.g.
// the evey-server container:
//
//	docker exec evey-server go test ./internal/adapters/driven/pluginrunner/ -run TestRedTeamSandboxEscape -v
//
// The resource-monitor probes (probe-miner mem balloon) and the data-store
// isolation probes (escape-datastore) are NOT covered here - they need the
// server's userspace monitor and the gRPC HostService respectively, and are
// covered by runner_resmon_test.go and the plugin-data tests.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

// repoRoot resolves the module root from this test file's location
// (internal/adapters/driven/pluginrunner ⇒ four levels up), independent of the
// caller's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}

// Fixtures tag verdicts [ESCAPE] (confinement boundary) or [MINER] (a
// cryptominer-flavoured fixture, e.g. cryptominer-gpu); both are parsed here.
var escapeVerdictRe = regexp.MustCompile(`\[(?:ESCAPE|MINER)\] (\S+): (BLOCKED|BYPASSED)`)

func parseEscapeVerdicts(out string) map[string]string {
	m := map[string]string{}
	for line := range strings.SplitSeq(out, "\n") {
		if mt := escapeVerdictRe.FindStringSubmatch(line); mt != nil {
			m[mt[1]] = mt[2]
		}
	}
	return m
}

// gate selects whether a probe assertion runs in the current environment.
type gate int

const (
	gateAlways   gate = iota // seccomp-enforced: portable
	gateLandlock             // Landlock-enforced: only on a Landlock-capable kernel
	gatePreload              // LD_PRELOAD execve denial: only when the shim built
)

type probeExpect struct {
	probe string
	want  string // "BLOCKED" or "BYPASSED"
	gate  gate
}

// runEscapeFixture installs the named fixture into a throwaway
// <tmp>/data/plugins/<id> tree (so its cwd-derived "/data" root points at a dir
// holding a fake jwt.key and a sibling plugin, none of which are on the
// allowlist), then runs it through the launcher exactly as the runner would in
// landlock mode. Returns the parsed [ESCAPE] verdicts.
func runEscapeFixture(t *testing.T, nodePath, launcher, soPath, root, name string, perms []string) map[string]string {
	t.Helper()

	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	installDir := filepath.Join(dataDir, "plugins", name)
	sibling := filepath.Join(dataDir, "plugins", "victim-plugin")
	for _, d := range []string{installDir, sibling} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	// A fake server secret outside the allowlist: a successful read = BYPASSED.
	if err := os.WriteFile(filepath.Join(dataDir, "jwt.key"), []byte("SUPER-SECRET-SIGNING-KEY"), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "tests", "plugins", "sandbox-escape", name, "main.js"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "main.js"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	// The launcher allowlists <sockDir>/<id> read-write; create it so a Landlock
	// rule for it resolves on a Landlock kernel (harmless no-op elsewhere).
	sock := filepath.Join(sockDir, name)
	_ = os.MkdirAll(sock, 0o755)
	t.Cleanup(func() { _ = os.RemoveAll(sock) })

	// The runner layers the LD_PRELOAD seccomp shim for plugins without the
	// subprocess permission. node must be able to open() the lib, and under
	// Landlock only allowlisted paths are openable - so place it in the plugin's
	// own (read-write, allowlisted) install dir, mirroring prod where the shim
	// ships under /app (on the read-only allowlist). A copy outside the allowlist
	// would be silently un-loadable and execve would not be denied.
	preload := ""
	if soPath != "" && !slices.Contains(perms, subprocessPerm) {
		preload = filepath.Join(installDir, "libevey-sandbox.so")
		so, rerr := os.ReadFile(soPath)
		if rerr != nil {
			t.Fatalf("read sandbox lib: %v", rerr)
		}
		if werr := os.WriteFile(preload, so, 0o755); werr != nil {
			t.Fatalf("install sandbox lib: %v", werr)
		}
	}

	p := &plugin.Plugin{ID: name, InstallDir: installDir, Permissions: perms}
	base := exec.Command(nodePath, filepath.Join(installDir, "main.js"))
	argv := buildLauncherArgv(launcher, p, base, landlockAvailable(), 0, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = installDir
	cmd.Env = append(os.Environ(), "HOME="+installDir, "TMPDIR="+installDir)
	if preload != "" {
		cmd.Env = append(cmd.Env, "LD_PRELOAD="+preload)
	}

	// Fixtures never bind the plugin socket and exit 0; we only care about the
	// logged verdicts (combined stdout+stderr). A non-zero exit is not a failure.
	out, _ := cmd.CombinedOutput()
	if testing.Verbose() {
		t.Logf("%s output:\n%s", name, out)
	}
	return parseEscapeVerdicts(string(out))
}

// skipUnlessCI skips with the reason on a developer machine and fails in CI,
// where a skipped sandbox test would be a green suite that asserted nothing.
func skipUnlessCI(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("CI") != "" {
		t.Fatalf("%s - the red-team sandbox test must run in CI", reason)
	}
	t.Skip(reason)
}

// nodeRunsSandboxed reports whether the launcher lets node start under the
// sandbox at all, and what it said when it did not.
func nodeRunsSandboxed(t *testing.T, nodePath, launcher string) (bool, string) {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(sockDir, "redteam-preflight")
	_ = os.MkdirAll(sock, 0o755)
	t.Cleanup(func() { _ = os.RemoveAll(sock) })
	p := &plugin.Plugin{ID: "redteam-preflight", InstallDir: dir, Permissions: []string{"api.library"}}
	argv := buildLauncherArgv(launcher, p, exec.Command(nodePath, "-e", "console.log('preflight-ok')"), landlockAvailable(), 0, 0)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	out, _ := cmd.CombinedOutput()
	return strings.Contains(string(out), "preflight-ok"), strings.TrimSpace(string(out))
}

func TestRedTeamSandboxEscape(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil {
		skipUnlessCI(t, "node not in PATH; the backend red-team fixtures are Node.js scripts")
	}
	root := repoRoot(t)
	bin := t.TempDir()

	launcher := filepath.Join(bin, launcherBinaryName)
	build := exec.Command("go", "build", "-o", launcher, "./cmd/evey-plugin-launcher")
	build.Dir = root
	if out, berr := build.CombinedOutput(); berr != nil {
		t.Fatalf("build launcher: %v\n%s", berr, out)
	}
	if ok, said := nodeRunsSandboxed(t, nodePath, launcher); !ok {
		skipUnlessCI(t, "node at "+nodePath+" cannot start inside the sandbox (outside its read-only paths?): every fixture would print nothing; launcher said: "+said)
	}

	// Best-effort: build the LD_PRELOAD seccomp shim. Without cc, the execve
	// (spawn) assertions are skipped rather than failing the whole test.
	soPath := ""
	if cc, lerr := exec.LookPath("cc"); lerr == nil {
		candidate := filepath.Join(bin, "libevey-sandbox.so")
		c := exec.Command(cc, "-shared", "-fPIC", "-nostartfiles", "-o", candidate, filepath.Join(root, "sandbox", "sandbox.c"))
		if out, cerr := c.CombinedOutput(); cerr == nil {
			soPath = candidate
		} else {
			t.Logf("cc build of libevey-sandbox.so failed; skipping LD_PRELOAD (execve) assertions: %v\n%s", cerr, out)
		}
	} else {
		t.Log("cc not in PATH; skipping LD_PRELOAD (execve) assertions")
	}

	landlockOn := landlockAvailable()
	soBuilt := soPath != ""
	t.Logf("environment: node=%s landlock=%v ld_preload=%v", nodePath, landlockOn, soBuilt)

	assert := func(t *testing.T, verdicts map[string]string, exps []probeExpect) {
		t.Helper()
		for _, e := range exps {
			switch e.gate {
			case gateLandlock:
				if !landlockOn {
					t.Logf("skip %q: Landlock not enforced on this kernel (asserted on CI/prod)", e.probe)
					continue
				}
			case gatePreload:
				if !soBuilt {
					t.Logf("skip %q: LD_PRELOAD shim unavailable", e.probe)
					continue
				}
			}
			got, ok := verdicts[e.probe]
			if !ok {
				t.Errorf("probe %q logged no verdict (fixture output missing it)", e.probe)
				continue
			}
			if got != e.want {
				t.Errorf("probe %q: got %s, want %s", e.probe, got, e.want)
			}
		}
	}

	// All fixtures declare api.library (a backend RPC perm) and neither network
	// nor subprocess - so the launcher gets -deny-inet and the LD_PRELOAD shim.
	const noNetNoSub = "api.library"

	t.Run("escape-fs", func(t *testing.T) {
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "escape-fs", []string{noNetNoSub})
		assert(t, v, []probeExpect{
			{"own_dir_CONTROL", "BYPASSED", gateAlways}, // control: own dir is allowlisted
			{"read_server_secret", "BLOCKED", gateLandlock},
			{"list_data_root", "BLOCKED", gateLandlock},
			{"list_all_plugins", "BLOCKED", gateLandlock},
			{"write_into_data", "BLOCKED", gateLandlock},
		})
	})

	t.Run("escape-net", func(t *testing.T) {
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "escape-net", []string{noNetNoSub})
		assert(t, v, []probeExpect{
			{"connect_postgres_5432", "BLOCKED", gateAlways}, // seccomp -deny-inet
			{"connect_meilisearch_7700", "BLOCKED", gateAlways},
			{"connect_server_http_9091", "BLOCKED", gateAlways},
			{"connect_external_https_443", "BLOCKED", gateAlways},
			{"connect_external_dns_tcp_53", "BLOCKED", gateAlways},
		})
	})

	t.Run("escape-proc", func(t *testing.T) {
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "escape-proc", []string{noNetNoSub})
		assert(t, v, []probeExpect{
			{"signal_pid1", "BLOCKED", gateAlways}, // seccomp denies kill(2)
			{"proc_enumeration", "BLOCKED", gateLandlock},
			{"steal_server_env", "BLOCKED", gateLandlock},
		})
	})

	t.Run("escape-exec", func(t *testing.T) {
		if !soBuilt {
			skipUnlessCI(t, "no C compiler (cc) built the LD_PRELOAD shim: the execve denial cannot be asserted")
		}
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "escape-exec", []string{noNetNoSub})
		assert(t, v, []probeExpect{
			{"spawn_subprocess", "BLOCKED", gatePreload},   // LD_PRELOAD denies execve
			{"child_reads_secret", "BLOCKED", gatePreload}, // spawn denied ⇒ reported blocked
		})
	})

	// Cryptominer on a hardware-transcode box: a plugin tries to reach the GPU
	// (DRM render node / WSL dxg / NVIDIA nodes) to mine, or to spawn a vendor
	// helper as a loader. The render node IS /dev/dri/renderD128 for both the
	// AMD/Intel iGPU (VAAPI) and discrete cards, so this one fixture covers every
	// vendor. The sandbox must deny the device open (Landlock now allowlists only
	// the safe pseudo-devices, not the whole /dev) and the helper spawn (LD_PRELOAD
	// execve shim). To make this assertion meaningful, map a real render node into
	// the test container (docker-compose.override.yml dev `evey` mounts /dev/dri).
	t.Run("cryptominer-gpu", func(t *testing.T) {
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "cryptominer-gpu", []string{noNetNoSub})
		assert(t, v, []probeExpect{
			{"list_dri", "BLOCKED", gateLandlock},
			{"open_drm_render", "BLOCKED", gateLandlock},
			{"open_drm_card", "BLOCKED", gateLandlock},
			{"open_dxg", "BLOCKED", gateLandlock},
			{"open_nvidia0", "BLOCKED", gateLandlock},
			{"open_nvidiactl", "BLOCKED", gateLandlock},
			{"exec_nvidia_smi", "BLOCKED", gatePreload},
			{"exec_ffmpeg_nvenc", "BLOCKED", gatePreload},
			// read_nvenc_lib / read_cuda_lib are NOT asserted: the vendor .so
			// files live under /usr (readable for the interpreter) and a plain
			// read is inert without a device node - the render-node boundary
			// asserted above is what blocks GPU use. They are logged for context.
		})
	})

	// Same fixture, but WITH the hardware.gpu permission granted. The grant is
	// the render node ALONE: open_drm_render must now succeed (legit GPU use),
	// while the privileged card node and every other vendor's node stay denied -
	// proving the grant is minimal and cannot be widened to card0/dxg/nvidia*.
	// Requires a real render node mapped into the test container (the dev `evey`
	// service maps /dev/dri); on a host without one, resolveRenderNode returns ""
	// so no -render-dev is emitted and open_drm_render reports BLOCKED - which is
	// why open_drm_render is gated on the node existing.
	t.Run("cryptominer-gpu-granted", func(t *testing.T) {
		if resolveRenderNode() == "" {
			t.Skip("no DRM render node on this host; granted-case asserts nothing")
		}
		v := runEscapeFixture(t, nodePath, launcher, soPath, root, "cryptominer-gpu", []string{noNetNoSub, gpuPerm})
		assert(t, v, []probeExpect{
			{"open_drm_render", "BYPASSED", gateLandlock}, // granted: render node reachable
			{"open_drm_card", "BLOCKED", gateLandlock},    // card/primary node NEVER granted
			{"open_dxg", "BLOCKED", gateLandlock},         // WSL GPU node never granted
			{"open_nvidia0", "BLOCKED", gateLandlock},     // NVIDIA nodes are a separate tier
			{"open_nvidiactl", "BLOCKED", gateLandlock},
			{"exec_nvidia_smi", "BLOCKED", gatePreload}, // exec still denied (no subprocess perm)
		})
	})
}
