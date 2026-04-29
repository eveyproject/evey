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
var AllowedSlots = map[string]SlotMeta{
	"home.section":             {Auth: true, Critical: false},
	"home.hero":                {Auth: true, Critical: false},
	"library.sidebar":          {Auth: true, Critical: false},
	"media.detail.sidebar":     {Auth: true, Critical: false},
	"player.overlay":           {Auth: true, Critical: false},
	"settings.user.appearance": {Auth: true, Critical: false},
}

// IsSlotAllowed returns true when a plugin is permitted to mount in this slot.
// Critical paths (admin/*, login/*, setup/*, profile/security/*) are never allowed.
func IsSlotAllowed(slot string) bool {
	_, ok := AllowedSlots[slot]
	return ok
}
