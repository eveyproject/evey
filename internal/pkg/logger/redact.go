package logger

import (
	"log/slog"
	"regexp"
	"strings"
)

// Credentials must not reach the log file: the file is what the admin log
// routes serve, what gets pasted in bug reports and what a backup carries.
// Two nets, applied at the write: an attribute whose KEY names a secret is
// replaced whole, and any string (message included) that carries a bearer,
// a JWT or a secret=value pair is scrubbed.

const redacted = "[redacted]"

// secretKeys are attribute keys whose value is a credential by definition.
var secretKeys = map[string]bool{
	"authorization": true, "token": true, "access_token": true, "refresh_token": true,
	"api_key": true, "apikey": true, "password": true, "pin": true, "secret": true,
	"pin_hash": true, "password_hash": true,
}

var lineRedactions = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer " + redacted},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}`), "[jwt]"},
	{regexp.MustCompile(`(?i)\b(token|access_token|refresh_token|api_key|apikey|password|pin|secret)=[^&\s"'\\]+`), "$1=" + redacted},
	{regexp.MustCompile(`(?i)"(token|access_token|refresh_token|api_key|apikey|password|pin|secret)"\s*:\s*"[^"]*"`), `"$1":"` + redacted + `"`},
}

// RedactLine scrubs credentials from free text (a log line, a message).
func RedactLine(s string) string {
	// Cheap gate: most lines carry none of the markers and skip the regexes.
	if !strings.Contains(s, "=") && !strings.Contains(s, "eyJ") && !strings.Contains(s, "earer ") && !strings.Contains(s, `":`) {
		return s
	}
	for _, r := range lineRedactions {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// redactAttr is the slog ReplaceAttr hook behind the file handler.
func redactAttr(_ []string, a slog.Attr) slog.Attr {
	if secretKeys[strings.ToLower(a.Key)] {
		if a.Value.Kind() == slog.KindGroup {
			return a
		}
		return slog.String(a.Key, redacted)
	}
	if a.Value.Kind() == slog.KindString {
		if v := a.Value.String(); v != "" {
			return slog.String(a.Key, RedactLine(v))
		}
	}
	return a
}
