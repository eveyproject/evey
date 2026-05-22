//go:build !linux

package pluginrunner

// landlockAvailable is always false off Linux; the server only runs Landlock
// inside its Linux image. This stub keeps the package building on dev machines.
func landlockAvailable() bool { return false }
