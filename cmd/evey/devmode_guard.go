package main

import (
	"fmt"
	"strings"
)

// refuseDevModeInProd fails closed when DevMode is enabled and the operator
// declared a production environment via EVEY_ENV. DevMode disables the plugin
// sandbox; combining it with a prod label is almost certainly a misconfiguration
// that would ship an unsandboxed plugin runtime. Returns nil otherwise.
//
// Accepted prod values (case + whitespace insensitive): "production", "prod".
func refuseDevModeInProd(eveyEnv string) error {
	env := strings.ToLower(strings.TrimSpace(eveyEnv))
	if env == "production" || env == "prod" {
		return fmt.Errorf("DevMode=true with EVEY_ENV=%s - refusing to start (DevMode disables the plugin sandbox)", env)
	}
	return nil
}
