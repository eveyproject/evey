package httpapi

import "testing"

// Log lines are read back off disk and parsed by hand-written helpers before
// the admin log explorer renders them. The server writes most of them, but a
// plugin can put arbitrary text into a log message, and a truncated write
// leaves a half line behind - so the bytes reaching these functions are not
// guaranteed to be well-formed JSON, or well-formed anything.
//
// Reading the code, neither helper does manual index arithmetic (strings.Cut
// and encoding/json only), so this is a confirmation rather than a hunt. It
// stays as a permanent harness: the day someone optimises logLineTime with a
// byte offset, this is what says so.
func FuzzParseLogLine(f *testing.F) {
	f.Add(`{"time":"2026-08-08T05:00:00.123456789Z","level":"INFO","msg":"some message","component":"library"}`)
	f.Add(`{"time":"2026-08-08T05:00:00Z","level":"ERROR","msg":"boom","media_id":"mi-1","user_id":"usr-1"}`)
	f.Add("")
	f.Add("{")
	f.Add(`{"time":"`)
	f.Add(`{"time":"not-a-date","level":42,"msg":null}`)
	f.Add(`{"time":123,"level":{"nested":true},"msg":["a"]}`)
	// A message that contains the key the scanner looks for, so the cheap
	// timestamp extraction meets a decoy before the real field.
	f.Add(`{"msg":"user typed \"time\":\"2020-01-01T00:00:00Z\"","time":"2026-08-08T05:00:00Z"}`)

	f.Fuzz(func(t *testing.T, line string) {
		// Returning at all is the assertion: on malformed input there is no
		// right answer, only "does not crash".
		_, _ = parseLogLine(line)
		_, _ = logLineTime(line)
	})
}
