package pluginrunner

import "slices"

import "github.com/gravity-zero/evey/internal/domain/plugin"

var KnownCapabilities = map[string]bool{
	"metadata.provider":   true,
	"media.read":          true,
	"media.write":         true,
	"webhook.receiver":    true,
	"notification.sender": true,
	"ui.extension":        true,
}

var KnownPermissions = map[string]bool{
	"network":      true,
	"filesystem":   true,
	"media.read":   true,
	"media.write":  true,
	"api.library":  true,
	"api.user":     true,
	"api.metadata": true,
}

func CheckCapability(p *plugin.Plugin, requiredCap string) bool {
	return slices.Contains(p.Capabilities, requiredCap)
}

func CheckPermission(p *plugin.Plugin, requiredPerm string) bool {
	return slices.Contains(p.Permissions, requiredPerm)
}
