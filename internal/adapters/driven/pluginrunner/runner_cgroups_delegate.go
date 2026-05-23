package pluginrunner

import (
	"bufio"
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroupDelegateEnv opts into per-plugin cgroup delegation. It is off by default
// because it relocates the container's own processes into a leaf cgroup and
// requires /sys/fs/cgroup to be mounted read-write inside the container.
const cgroupDelegateEnv = "EVEY_PLUGINS_CGROUP_DELEGATE"

// cgroupDelegationEnabled reports whether the operator opted into delegation.
func cgroupDelegationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(cgroupDelegateEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// setupCgroupDelegation prepares the cgroup v2 hierarchy so prepareCgroup can
// create per-plugin leaves with working memory/cpu/pids controllers.
//
// cgroup v2's "no internal processes" rule forbids enabling controllers
// (cgroup.subtree_control) on a cgroup that holds processes directly, and Docker
// places the container's processes (the evey server) in the container cgroup
// root. So we: move the root's processes into an evey-init leaf, enable the
// controllers in the root's subtree_control, then create an evey-plugins parent
// with the controllers delegated. Per-plugin leaves under evey-plugins then get
// memory.max / cpu.max / pids.max — so a plugin breaching its own limit is
// OOM-killed in its own cgroup, never touching the host, the server or siblings.
//
// Every step is best-effort: on ANY error we log a precise reason and return
// false, leaving the server exactly where it was (no limits, never a crash).
func setupCgroupDelegation() bool {
	root := cgroupRoot

	avail, err := os.ReadFile(filepath.Join(root, "cgroup.controllers"))
	if err != nil {
		slog.Warn("cgroup delegation: cgroup v2 unavailable (cannot read cgroup.controllers) — per-plugin limits disabled",
			slog.Any("err", err))
		return false
	}
	want := availableControllers(string(avail))
	// memory/cpu/pids are the caps that matter; io is a best-effort bonus. If
	// the three core controllers are not all delegable, per-plugin limits can't
	// be enforced — fail-soft.
	if !containsAll(want, "memory", "cpu", "pids") {
		slog.Warn("cgroup delegation: memory/cpu/pids not all available at the cgroup root — per-plugin limits disabled",
			slog.String("available", strings.TrimSpace(string(avail))))
		return false
	}

	pluginsParent := filepath.Join(root, "evey-plugins")
	if err := os.MkdirAll(pluginsParent, 0o755); err != nil {
		slog.Warn("cgroup delegation: cannot create evey-plugins (is /sys/fs/cgroup mounted read-write in the container?) — per-plugin limits disabled",
			slog.Any("err", err))
		return false
	}

	// "no internal processes": relocate the container's own processes to a leaf
	// so the root can enable subtree_control. Hierarchical limits still apply, so
	// the server in evey-init stays bounded by any container-level limit.
	if pids := readCgroupProcs(filepath.Join(root, "cgroup.procs")); len(pids) > 0 {
		initLeaf := filepath.Join(root, "evey-init")
		if err := os.MkdirAll(initLeaf, 0o755); err != nil {
			slog.Warn("cgroup delegation: cannot create evey-init leaf — per-plugin limits disabled", slog.Any("err", err))
			return false
		}
		moved, failed := moveCgroupProcs(pids, filepath.Join(initLeaf, "cgroup.procs"))
		slog.Info("cgroup delegation: relocated container processes to evey-init leaf",
			slog.Int("moved", moved), slog.Int("failed", failed))
	}

	if err := enableSubtreeControllers(filepath.Join(root, "cgroup.subtree_control"), want); err != nil {
		slog.Warn("cgroup delegation: cannot enable controllers in the root subtree_control "+
			"(processes may still live in the root cgroup, or the mount is read-only) — per-plugin limits disabled",
			slog.Any("err", err))
		return false
	}
	if err := enableSubtreeControllers(filepath.Join(pluginsParent, "cgroup.subtree_control"), want); err != nil {
		slog.Warn("cgroup delegation: cannot enable controllers on evey-plugins — per-plugin limits disabled",
			slog.Any("err", err))
		return false
	}

	slog.Info("cgroup delegation ready — per-plugin resource limits active",
		slog.String("controllers", strings.Join(want, ",")))
	return true
}

// availableControllers returns the subset of {memory,cpu,pids} present in a
// cgroup.controllers file, preserving that fixed order.
func availableControllers(controllers string) []string {
	have := make(map[string]bool)
	for _, f := range strings.Fields(controllers) {
		have[f] = true
	}
	var out []string
	for _, c := range []string{"memory", "cpu", "pids", "io"} {
		if have[c] {
			out = append(out, c)
		}
	}
	return out
}

// containsAll reports whether every want item is present in have.
func containsAll(have []string, want ...string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// readCgroupProcs reads PIDs (one per line) from a cgroup.procs file.
func readCgroupProcs(path string) []int {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pids []int
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		if n, err := strconv.Atoi(strings.TrimSpace(sc.Text())); err == nil && n > 0 {
			pids = append(pids, n)
		}
	}
	return pids
}

// moveCgroupProcs writes each PID to dst (a cgroup.procs file) one at a time, as
// cgroup v2 requires. Failures are non-fatal (e.g. a process that exited between
// the read and the write); returns the moved/failed counts.
func moveCgroupProcs(pids []int, dst string) (moved, failed int) {
	for _, pid := range pids {
		if err := os.WriteFile(dst, []byte(strconv.Itoa(pid)), 0o644); err != nil {
			failed++
			continue
		}
		moved++
	}
	return moved, failed
}

// enableSubtreeControllers writes "+memory +cpu +pids" to a subtree_control file.
func enableSubtreeControllers(path string, controllers []string) error {
	parts := make([]string, len(controllers))
	for i, c := range controllers {
		parts[i] = "+" + c
	}
	return os.WriteFile(path, []byte(strings.Join(parts, " ")), 0o644)
}
