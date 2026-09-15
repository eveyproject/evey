package httpapi

import "regexp"

// The log routes hand an admin whatever the server wrote. A credential that
// reached a line (a proxy dump, a stream URL with its ?token=, a login body)
// must not come back out in clear: an admin response can be proxied, pasted in
// a bug report or read over a shoulder.
var logRedactions = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Bearer credentials, wherever they were echoed.
	{regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer [redacted]"},
	// A JWT stands out on its own: three base64url segments starting "eyJ".
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}\.[A-Za-z0-9_-]{4,}`), "[jwt]"},
	// Query/form/slog key=value pairs carrying a secret.
	{regexp.MustCompile(`(?i)\b(token|access_token|refresh_token|api_key|apikey|password|pin|secret)=[^&\s"'\\]+`), "$1=[redacted]"},
	// JSON fields carrying a secret.
	{regexp.MustCompile(`(?i)"(token|access_token|refresh_token|api_key|apikey|password|pin|secret)"\s*:\s*"[^"]*"`), `"$1":"[redacted]"`},
}

// redactLogLine scrubs credentials from one log line before it leaves the
// server through the admin log routes.
func redactLogLine(line string) string {
	for _, r := range logRedactions {
		line = r.re.ReplaceAllString(line, r.repl)
	}
	return line
}
