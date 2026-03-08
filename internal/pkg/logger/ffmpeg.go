package logger

import (
	"bufio"
	"io"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reProgress = regexp.MustCompile(`frame=\s*(\d+).*fps=\s*([\d.]+).*time=([\d:.]+).*bitrate=\s*([\d.]+\S*)`)
	reError    = regexp.MustCompile(`(?i)error|invalid|failed|cannot|no such`)
	reWarning  = regexp.MustCompile(`(?i)warning|deprecated|overread`)
)

// NewFFmpegLogger returns a logger for FFmpeg output, writing to logs/ffmpeg.log.
func NewFFmpegLogger(cfg Config) *slog.Logger {
	path := filepath.Join(cfg.Dir, "ffmpeg.log")
	return NewFileLogger(path, cfg).With(slog.String("component", "ffmpeg"))
}

// StreamFFmpegOutput reads stderr line-by-line from r and logs each line
// structured via the provided logger. Returns when r is closed.
func StreamFFmpegOutput(r io.Reader, log *slog.Logger) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		parseFFmpegLine(log, line)
	}
}

func parseFFmpegLine(log *slog.Logger, line string) {
	if m := reProgress.FindStringSubmatch(line); m != nil {
		log.Debug("progress",
			slog.String("frame", m[1]),
			slog.String("fps", m[2]),
			slog.String("time", m[3]),
			slog.String("bitrate", m[4]),
		)
		return
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	switch {
	case reError.MatchString(trimmed):
		log.Error(trimmed)
	case reWarning.MatchString(trimmed):
		log.Warn(trimmed)
	default:
		log.Debug(trimmed)
	}
}
