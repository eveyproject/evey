//go:build linux

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fsBaseV1 is the set of filesystem access rights known to Landlock ABI v1.
// Later ABIs add a few more (handled in supportedFSAccess); we must never pass
// a right the running kernel doesn't know or landlock_create_ruleset returns
// EINVAL - hence the per-ABI masking.
const fsBaseV1 = uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
	unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_FILE |
	unix.LANDLOCK_ACCESS_FS_READ_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_DIR |
	unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
	unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
	unix.LANDLOCK_ACCESS_FS_MAKE_REG |
	unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_SYM)

// supportedFSAccess returns the handled_access_fs mask for a given ABI version.
// This is the universe of rights the ruleset governs; anything not granted by a
// per-path rule below is therefore denied.
func supportedFSAccess(abi int) uint64 {
	a := fsBaseV1
	if abi >= 2 {
		a |= unix.LANDLOCK_ACCESS_FS_REFER // rename/link across directories
	}
	if abi >= 3 {
		a |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 5 {
		a |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return a
}

// readExecAccess is what we grant on read-only directories (system libs, certs):
// read files, list directories, execute binaries. No write/create/remove.
func readExecAccess(abi int) uint64 {
	return (uint64(unix.LANDLOCK_ACCESS_FS_EXECUTE |
		unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR)) & supportedFSAccess(abi)
}

// readWriteAccess is what we grant on the plugin's own directory and its socket
// directory: the supported set minus symlink/FIFO/device-node creation, which stay
// handled and thus denied (a symlink under web_root would be served by the server).
func readWriteAccess(abi int) uint64 {
	return supportedFSAccess(abi) &^ uint64(unix.LANDLOCK_ACCESS_FS_MAKE_SYM|
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO|
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR|
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK)
}

// deviceAccess is what we grant on an individual allowed device node (/dev/null,
// /dev/urandom…): read + write the file, nothing else. Deliberately WITHOUT
// IOCTL_DEV (ABI v5) - a sandboxed plugin has no business issuing device ioctls,
// which is how GPU/DRM command submission works. Granting per-node like this
// (instead of the whole /dev directory) keeps GPU render nodes, /dev/dxg and
// /dev/nvidia* outside the allowlist, so a cryptominer plugin can't open them.
func deviceAccess(abi int) uint64 {
	return (uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_WRITE_FILE)) & supportedFSAccess(abi)
}

// renderDeviceAccess is the access mask for the ONE granted GPU render node
// (the hardware.gpu permission). It adds IOCTL_DEV (ABI v5+) because DRM command
// submission IS ioctl-based - a render node without it is useless - but it is
// applied ONLY to this node, never to the safe pseudo-devices (deviceAccess),
// so the GPU ioctl surface is opened for exactly one file. On ABI <5 the kernel
// does not govern device ioctls, so read+write already permits submission.
// Note: IOCTL_DEV (like all Landlock) governs only newly-open()ed files - an
// inherited render fd bypasses it, so the runner must never pre-open one.
func renderDeviceAccess(abi int) uint64 {
	a := uint64(unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE)
	if abi >= 5 {
		a |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	return a & supportedFSAccess(abi)
}

// isRenderNode reports whether p is a DRM render node path (/dev/dri/renderD<n>).
// The -render-dev grant carries the GPU ioctl surface (IOCTL_DEV), so it is
// refused for anything else - in particular a card/primary node
// (/dev/dri/card*), which controls modeset/DRM-Master and is privileged.
// Defence in depth: the runner already resolves only renderD*, but the launcher
// must never grant the GPU command-submission surface to an arbitrary path.
func isRenderNode(p string) bool {
	const prefix = "/dev/dri/renderD"
	if !strings.HasPrefix(p, prefix) {
		return false
	}
	digits := p[len(prefix):]
	if digits == "" {
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LANDLOCK_RULE_NET_PORT and the net-port rule attribute are part of Landlock
// ABI v4 but are not yet exported by x/sys/unix v0.45.0, so we declare them
// here. The struct layout mirrors the kernel's `struct landlock_net_port_attr`.
const ruleTypeNetPort = 0x2

type landlockNetPortAttr struct {
	AllowedAccess uint64
	Port          uint64
}

// supportedNetAccess returns the handled_access_net mask for the given ABI.
// Network access control landed in ABI v4 (kernel ≥6.7); earlier kernels return
// 0, so callers must skip net restriction (best-effort degrade).
func supportedNetAccess(abi int) uint64 {
	if abi >= 4 {
		return uint64(unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP)
	}
	return 0
}

// landlockABI queries the kernel's supported Landlock ABI version. Returns an
// error when Landlock is absent (ENOSYS) or disabled in the active LSM list
// (EOPNOTSUPP) - the caller decides whether that is fatal.
func landlockABI() (int, error) {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, uintptr(unix.LANDLOCK_CREATE_RULESET_VERSION))
	if errno != 0 {
		return 0, fmt.Errorf("landlock_create_ruleset(version): %w", errno)
	}
	return int(r), nil
}

func applyLandlock(cfg config) error {
	if len(cfg.roPaths) == 0 && len(cfg.rwPaths) == 0 && len(cfg.devPaths) == 0 && cfg.renderDevPath == "" && !cfg.restrictNet {
		return nil // nothing requested; caller asked for no confinement
	}

	abi, err := landlockABI()
	if err != nil {
		return err
	}
	if abi < 1 {
		return fmt.Errorf("landlock reported abi %d (unsupported)", abi)
	}

	handled := supportedFSAccess(abi)
	// Govern TCP access too when asked AND the kernel supports it (ABI v4+).
	// On older kernels handledNet stays 0: filesystem confinement still applies,
	// network filtering silently degrades to a no-op (best effort).
	var handledNet uint64
	if cfg.restrictNet {
		handledNet = supportedNetAccess(abi)
	}
	attr := unix.LandlockRulesetAttr{Access_fs: handled, Access_net: handledNet}
	rulesetFD, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	defer unix.Close(int(rulesetFD)) //nolint:errcheck // best-effort fd cleanup

	addRule := func(path string, access uint64) error {
		fd, openErr := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if openErr != nil {
			return fmt.Errorf("open %s: %w", path, openErr)
		}
		defer unix.Close(fd) //nolint:errcheck // best-effort fd cleanup
		beneath := unix.LandlockPathBeneathAttr{
			Allowed_access: access & handled,
			Parent_fd:      int32(fd),
		}
		_, _, e := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE,
			rulesetFD, uintptr(unix.LANDLOCK_RULE_PATH_BENEATH),
			uintptr(unsafe.Pointer(&beneath)), 0, 0, 0)
		if e != 0 {
			return fmt.Errorf("landlock_add_rule %s: %w", path, e)
		}
		return nil
	}

	// Read-only system paths: skip ones that don't exist on this image (e.g. a
	// distro without /lib64 or /sbin) rather than failing the whole ruleset.
	for _, p := range cfg.roPaths {
		if !pathExists(p) {
			continue
		}
		if err := addRule(p, readExecAccess(abi)); err != nil {
			return err
		}
	}
	// Read-write paths (plugin dir, socket dir) must exist; a missing one is a
	// real misconfiguration, so let the error surface.
	for _, p := range cfg.rwPaths {
		if err := addRule(p, readWriteAccess(abi)); err != nil {
			return err
		}
	}
	// Individual device nodes (/dev/null, /dev/urandom…): allow read+write but
	// skip ones absent on this image (a minimal container may lack /dev/full or
	// /dev/tty) rather than failing the whole ruleset. Everything else under
	// /dev - GPU render nodes, /dev/dxg, /dev/nvidia* - stays denied.
	for _, p := range cfg.devPaths {
		if !pathExists(p) {
			continue
		}
		if err := addRule(p, deviceAccess(abi)); err != nil {
			return err
		}
	}
	// The single GPU render node (hardware.gpu grant). Validated here too -
	// never grant the IOCTL_DEV-bearing render rule to anything but a renderD*
	// node - and fail closed if it doesn't exist (the runner only ever passes a
	// resolved node, so a missing one is a real misconfiguration).
	if cfg.renderDevPath != "" {
		if !isRenderNode(cfg.renderDevPath) {
			return fmt.Errorf("render-dev %q is not a DRM render node (/dev/dri/renderD<n>)", cfg.renderDevPath)
		}
		if err := addRule(cfg.renderDevPath, renderDeviceAccess(abi)); err != nil {
			return err
		}
	}

	// Network: with handledNet set, every TCP bind/connect is denied unless a
	// rule below allows it. We only ever allow connect() to the listed ports
	// (never bind), so a plugin can reach its declared egress ports but cannot
	// listen, and a plugin with no allowed ports is fully cut off from TCP.
	if handledNet != 0 {
		connectAccess := uint64(unix.LANDLOCK_ACCESS_NET_CONNECT_TCP) & handledNet
		for _, port := range cfg.netConnectPorts {
			np := landlockNetPortAttr{AllowedAccess: connectAccess, Port: port}
			if _, _, e := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE,
				rulesetFD, ruleTypeNetPort,
				uintptr(unsafe.Pointer(&np)), 0, 0, 0); e != 0 {
				return fmt.Errorf("landlock_add_rule net connect port %d: %w", port, e)
			}
		}
	}

	// Landlock restriction binds to the calling thread and survives execve. Pin
	// to this OS thread so the subsequent syscall.Exec runs on the same thread
	// that we just restricted.
	runtime.LockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if _, _, e := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, rulesetFD, 0, 0); e != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", e)
	}
	return nil
}

func applyRlimits(cfg config) {
	set := func(resource int, val uint64, name string) {
		if val == 0 {
			return
		}
		lim := unix.Rlimit{Cur: val, Max: val}
		if err := unix.Setrlimit(resource, &lim); err != nil {
			fmt.Fprintf(os.Stderr, "evey-plugin-launcher: setrlimit %s: %v\n", name, err)
		}
	}
	set(unix.RLIMIT_AS, cfg.rlimitAS, "AS")
	set(unix.RLIMIT_CPU, cfg.rlimitCPU, "CPU")
	set(unix.RLIMIT_NOFILE, cfg.rlimitNOFILE, "NOFILE")
	set(unix.RLIMIT_NPROC, cfg.rlimitNPROC, "NPROC")
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
