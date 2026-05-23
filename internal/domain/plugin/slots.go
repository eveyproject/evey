package plugin

// SlotMeta carries mount-permission metadata for a UI slot.
type SlotMeta struct {
	// Auth indicates this slot is only reachable by authenticated users.
	Auth bool
	// Critical marks slots that would appear on security-sensitive paths;
	// reserved for future enforcement — currently no slot is critical.
	Critical bool
}

// AllowedSlots enumerates every slot a plugin is permitted to mount in.
// Critical paths (admin/*, login/*, setup/*, profile/security/*) are
// intentionally absent — plugins must never inject UI there.
//
// Slots NOT included and why:
//   - admin.*      : admin area is privileged; plugin injection there is a privilege-escalation vector.
//   - login.*      : login path is authentication-critical; even cosmetic injection can be used for phishing.
//   - setup.*      : first-run setup handles credentials; same risk as login.
//   - profile.security.* : security settings (PIN, password); injection would undermine access controls.
//   - sidebar      : too generic — mount point is ambiguous without an explicit sub-path.
var AllowedSlots = map[string]SlotMeta{
	"home.section":             {Auth: true, Critical: false},
	"home.hero":                {Auth: true, Critical: false},
	"library.sidebar":          {Auth: true, Critical: false},
	"media.detail.sidebar":     {Auth: true, Critical: false},
	"player.overlay":           {Auth: true, Critical: false},
	"settings.user.appearance": {Auth: true, Critical: false},
	// sidebar.bottom is the navigation tray bottom area — non-critical, cosmetic.
	"sidebar.bottom": {Auth: true, Critical: false},
	// admin.nav is the ONE controlled admin-area slot: it adds a single nav entry
	// to the admin panel opening the plugin's OWN sandboxed page. It is NOT an
	// injection point into existing admin screens (those stay off-limits, see
	// below) — the frontend renders it as a dedicated, admin-gated route hosting
	// the plugin iframe. Requesting it surfaces in the install consent diff.
	"admin.nav": {Auth: true, Critical: false},
}

// IsSlotAllowed returns true when a plugin is permitted to mount in this slot.
// Critical paths (admin/*, login/*, setup/*, profile/security/*) are never allowed.
func IsSlotAllowed(slot string) bool {
	_, ok := AllowedSlots[slot]
	return ok
}
