package config

import "testing"

// The real key names in the store, straight from the admin panel's cards, plus
// the ones that must NOT be caught. A rule that masks a transcoding knob is a
// rule that gets reverted the first time an operator cannot read a setting.
func TestIsSecretKey(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"api.tmdb_key", true},
		{"api.omdb_key", true},
		{"api.opensubtitles_key", true},
		{"api.tidb_key", true},
		{"smtp.password", true},
		{"some.api_token", true},
		{"provider.client_secret", true},
		{"API.TMDB_KEY", true},
		{"  api.tmdb_key  ", true},
		// A public key ends the same way and is meant to be served.
		{"push.vapid_public_key", false},
		{"oauth.public_key", false},
		// Ordinary settings.
		{"smtp.host", false},
		{"smtp.username", false},
		{"server.name", false},
		{"transcoding.max_concurrent", false},
		{"intro_providers.enabled", false},
		{"playback.skip_forward_seconds", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsSecretKey(c.key); got != c.want {
			t.Errorf("IsSecretKey(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestMaskSecretValue(t *testing.T) {
	if got := MaskSecretValue("api.tmdb_key", "abcdef123456"); got != SecretMask {
		t.Errorf("a set secret must be masked, got %q", got)
	}
	// An unset key must not be reported as configured.
	if got := MaskSecretValue("api.tmdb_key", ""); got != "" {
		t.Errorf("an empty secret must stay empty, got %q", got)
	}
	if got := MaskSecretValue("smtp.host", "smtp.example.com"); got != "smtp.example.com" {
		t.Errorf("a non-secret must pass through, got %q", got)
	}
}

func TestShouldKeepStoredSecret(t *testing.T) {
	if !ShouldKeepStoredSecret("smtp.password", SecretMask) {
		t.Error("echoing the mask back must leave the stored secret alone")
	}
	if ShouldKeepStoredSecret("smtp.password", "hunter2") {
		t.Error("a real new value must be written")
	}
	// Clearing a credential has to keep working.
	if ShouldKeepStoredSecret("smtp.password", "") {
		t.Error("an empty value must clear the secret, not be ignored")
	}
	// The mask is only special on a secret key.
	if ShouldKeepStoredSecret("smtp.host", SecretMask) {
		t.Error("the mask has no meaning on a non-secret key")
	}
}
