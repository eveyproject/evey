package plugin

// SlotMeta carries mount-permission metadata for a UI slot.
type SlotMeta struct {
	// Auth indicates this slot is only reachable by authenticated users.
	Auth bool
	// Critical marks slots that would appear on security-sensitive paths;
	// reserved for future enforcement - currently no slot is critical.
	Critical bool
}

// AllowedSlots enumerates every slot a plugin is permitted to mount in.
// Critical paths (existing admin screens, login/*, setup/*, profile/security/*)
// are intentionally absent - plugins must never inject UI there. The one
// exceptions are admin.nav (see below), a single controlled nav entry that
// opens the plugin's own sandboxed page rather than injecting into an
// existing admin screen, and admin.plugin.config, the plugin's own config
// panel inside its own card.
//
// Slots NOT included and why:
//   - admin.* (beyond admin.nav and admin.plugin.config) : admin area is privileged; injecting into an existing admin screen is a privilege-escalation vector.
//   - login.*      : login path is authentication-critical; even cosmetic injection can be used for phishing.
//   - setup.*      : first-run setup handles credentials; same risk as login.
//   - profile.security.* : security settings (PIN, password); injection would undermine access controls.
//   - sidebar      : too generic - mount point is ambiguous without an explicit sub-path.
var AllowedSlots = map[string]SlotMeta{
	"home.section":             {Auth: true, Critical: false},
	"home.hero":                {Auth: true, Critical: false},
	"library.sidebar":          {Auth: true, Critical: false},
	"media.detail.sidebar":     {Auth: true, Critical: false},
	"player.overlay":           {Auth: true, Critical: false},
	"settings.user.appearance": {Auth: true, Critical: false},
	// sidebar.bottom is the navigation tray bottom area - non-critical, cosmetic.
	"sidebar.bottom": {Auth: true, Critical: false},
	// admin.nav is the ONE controlled admin-area slot: it adds a single nav entry
	// to the admin panel opening the plugin's OWN sandboxed page. It is NOT an
	// injection point into existing admin screens (those stay off-limits, see
	// below) - the frontend renders it as a dedicated, admin-gated route hosting
	// the plugin iframe. Requesting it surfaces in the install consent diff.
	"admin.nav": {Auth: true, Critical: false},
	// admin.plugin.config replaces the generated config form inside the
	// plugin's OWN card on the admin plugins screen with the plugin's sandboxed
	// panel, for a value a plain field cannot pick well (a city, a list). It
	// injects nothing elsewhere; its one extra bridge call, evey.config.save,
	// writes only this plugin's config, on the admin's session, validated
	// against the schema like the form.
	"admin.plugin.config": {Auth: true, Critical: false},
}

// IsSlotAllowed returns true when a plugin is permitted to mount in this slot.
// Critical paths (existing admin screens, login/*, setup/*, profile/security/*)
// are never allowed; admin.nav and admin.plugin.config are the controlled
// exceptions (see AllowedSlots).
func IsSlotAllowed(slot string) bool {
	_, ok := AllowedSlots[slot]
	return ok
}
