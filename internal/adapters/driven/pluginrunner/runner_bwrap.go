package pluginrunner

// networkPerm is the permission that grants a plugin outbound network access.
const networkPerm = "network"

// buildBwrapCmd returns the bwrap argv prefix to wrap a plugin command.
// pluginDir is the writable bind mount for the plugin's own directory.
// hasNetworkPerm controls whether --unshare-net is omitted (true = plugin
// may reach the network; false = network namespace is isolated).
// hostsFile, when non-empty and hasNetworkPerm is true, is bind-mounted
// read-only as /etc/hosts so only whitelisted hosts are resolvable.
// Returns nil when mode == "basic" so callers can short-circuit.
func buildBwrapCmd(pluginDir string, hasNetworkPerm bool, mode string, hostsFile string) []string {
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
	)

	if hostsFile != "" && hasNetworkPerm {
		args = append(args, "--ro-bind", hostsFile, "/etc/hosts")
	}

	args = append(args,
		"--proc", "/proc",
		"--dev-bind", "/dev", "/dev",
		"--tmpfs", "/tmp",
		"--die-with-parent",
		"--",
	)

	return args
}
