package pluginrunner

// networkPerm is the permission that grants a plugin outbound network access.
const networkPerm = "network"

// buildBwrapCmd returns the bwrap argv prefix to wrap a plugin command.
// pluginDir is the writable bind mount for the plugin's own directory.
// pluginSockDir is the per-plugin sock dir on the host; it is bind-mounted
// (read-write) into the sandbox at the same path so the plugin can bind
// EVEY_PLUGIN_SOCK and the host (outside the sandbox) sees the socket.
// hasNetworkPerm controls whether --unshare-net is omitted (true = plugin
// may reach the network; false = network namespace is isolated).
// hostsFile, when non-empty and hasNetworkPerm is true, is bind-mounted
// read-only as /etc/hosts so only whitelisted hosts are resolvable.
// Returns nil when mode == "basic" so callers can short-circuit.
func buildBwrapCmd(pluginDir, pluginSockDir string, hasNetworkPerm bool, mode string, hostsFile string) []string {
	if mode == sandboxModeBasic {
		return nil
	}

	args := []string{
		"bwrap",
		"--unshare-user",
		"--unshare-pid",
		"--unshare-uts",
		"--unshare-ipc",
	}

	if !hasNetworkPerm {
		args = append(args, "--unshare-net")
	}

	args = append(args,
		"--ro-bind", "/", "/",
		"--bind", pluginDir, pluginDir,
		"--proc", "/proc",
		"--dev-bind", "/dev", "/dev",
		"--tmpfs", "/tmp",
	)

	// The sock dir lives under /tmp on the host (sockDir = "/tmp/evey/plugins").
	// --tmpfs /tmp above wipes any host-prepared directory inside the sandbox,
	// so we re-bind the sock dir RW AFTER the tmpfs to make the host-side sock
	// path visible to the plugin process.
	if pluginSockDir != "" {
		args = append(args, "--bind", pluginSockDir, pluginSockDir)
	}

	if hostsFile != "" && hasNetworkPerm {
		args = append(args, "--ro-bind", hostsFile, "/etc/hosts")
	}

	args = append(args,
		"--die-with-parent",
		"--",
	)

	return args
}
