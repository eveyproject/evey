package plugin

// HostPermissions are the permissions the host page enforces itself, for a
// bridge call it answers on the viewer's own session (media.trash: the host
// shows its own confirmation, then calls an admin endpoint). The public
// plugin list names the ones each plugin holds, and nothing else of its
// permissions.
var HostPermissions = map[string]bool{
	"media.trash": true,
}

// GrantedHostPermissions returns the host permissions among perms, in order.
func GrantedHostPermissions(perms []string) []string {
	out := []string{}
	for _, p := range perms {
		if HostPermissions[p] {
			out = append(out, p)
		}
	}
	return out
}
