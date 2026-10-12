package httpapi

import eveylogger "github.com/eveyproject/evey/internal/pkg/logger"

// redactLogLine scrubs credentials from one log line before it leaves the
// server through the admin log routes. The file is scrubbed at the write too
// (logger.redactAttr); this is the belt for lines written before that, and
// for anything a stray non-slog writer put in the file.
func redactLogLine(line string) string {
	return eveylogger.RedactLine(line)
}
