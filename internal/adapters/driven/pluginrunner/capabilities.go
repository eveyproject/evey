package pluginrunner

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
	"api.metadata":         true,
	"notification.sender":  true,
}
