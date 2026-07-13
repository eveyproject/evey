package transcoding

import "testing"

// The admin toggle persists ONLY transcoding.hw_mode (settings_handler sets
// HWAccelEnabled in memory and returns the mode string). So on the next boot
// the legacy master gate MUST be derived from the persisted mode - otherwise a
// GPU-enabled server silently drops back to software after any restart while
// the panel still shows the toggle on, which is what cut playback in prod on
// 2026-07-13 (deploy at 02:25, software transcode at 03:41).
func TestHWGateEnabled_FollowsPersistedMode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string // persisted transcoding.hw_mode
		envAccel   bool   // EVEY_TRANSCODING_HW_ACCEL_ENABLED (bootstrap default: false)
		envBackend string
		want       bool
	}{
		{"panel enabled (auto), env default off", "auto", false, "", true},
		{"panel pinned a backend, env default off", "vaapi", false, "", true},
		{"panel disabled", "off", false, "", false},
		{"panel disabled wins over env on", "off", true, "", false},
		{"no persisted mode, env off", "", false, "", false},
		{"no persisted mode, env on", "", true, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := HWGateEnabled(tc.mode, tc.envAccel, tc.envBackend); got != tc.want {
				t.Errorf("HWGateEnabled(%q, %v, %q) = %v, want %v", tc.mode, tc.envAccel, tc.envBackend, got, tc.want)
			}
		})
	}
}
