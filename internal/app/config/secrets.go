package config

import "strings"

// secrets.go - which persisted config keys hold a credential, and what the
// admin panel is allowed to see of them.
//
// The store is a generic key/value table (`string value` in the proto), so it
// has no notion of a secret: ListServerConfigEntries handed every row back
// verbatim, which meant the panel - and anything that could read one admin
// response - received the TMDB / OMDb / OpenSubtitles / IntroDB keys and the
// SMTP password in clear. The panel masked them with bullets in the DOM while
// the full value sat in the JSON right behind it.
//
// The predicate lives here, in the app layer, rather than in the handler: two
// entry points already write the same keys, and a masking rule that only one of
// them knows about is a rule that leaks through the other.

// SecretMask is what a secret's value is replaced with on the way out, and the
// sentinel the write path recognises on the way back in. It is deliberately a
// fixed-width string that carries no information about the real value - not
// even its length, which for an API key narrows the provider.
//
// A caller that sends SecretMask back is saying "leave it alone" (see
// ShouldKeepStoredSecret). An operator whose credential is literally eight
// bullet characters cannot change it through the panel; that is the whole cost
// of this design, and it is worth it.
const SecretMask = "••••••••"

// secretKeySuffixes marks a key as holding a credential. Suffix matching rather
// than an explicit list because the list is exactly what goes stale: a provider
// added next month gets its key named `api.<name>_key` like every other one,
// and inherits the masking without anybody remembering this file exists.
var secretKeySuffixes = []string{
	"_key",
	".key",
	"_secret",
	".secret",
	"_password",
	".password",
	"_token",
	".token",
}

// IsSecretKey reports whether the value stored under key is a credential that
// must never be read back out of the server.
//
// A key that says "public" is exempt: a VAPID or OAuth public key ends in
// "_key" like its private twin, and masking it would break the clients that are
// supposed to receive it.
func IsSecretKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" || strings.Contains(k, "public") {
		return false
	}
	for _, suffix := range secretKeySuffixes {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// MaskSecretValue returns what an admin API may report for key.
//
// An empty value is returned as-is: a key with no value set is not a secret to
// hide, and masking it would tell the panel a credential exists where none
// does - which is how an operator ends up debugging an integration that was
// never configured.
func MaskSecretValue(key, value string) string {
	if value == "" || !IsSecretKey(key) {
		return value
	}
	return SecretMask
}

// ShouldKeepStoredSecret reports whether a write must be ignored because the
// caller is echoing back the mask it was served rather than supplying a new
// value.
//
// Without this, every save from a form that carries all its fields at once -
// the SMTP card sends host, port, username, password, from and tls together -
// would overwrite the stored password with bullets the moment the operator
// changed the port. An empty value is NOT the mask: clearing a field is a
// legitimate way to remove a credential, and it must keep working.
func ShouldKeepStoredSecret(key, value string) bool {
	return IsSecretKey(key) && value == SecretMask
}
