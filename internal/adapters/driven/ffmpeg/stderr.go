package ffmpeg

// firstLine trims ffmpeg's stderr to its first line for error messages.
func firstLine(out []byte) string {
	for i, b := range out {
		if b == '\n' {
			return string(out[:i])
		}
	}
	return string(out)
}
