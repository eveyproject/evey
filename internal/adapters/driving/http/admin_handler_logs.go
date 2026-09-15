package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

// currentLogPath resolves the active log file produced by the rotating writer
// in internal/pkg/logger (files are named evey-YYYY-MM-DD[-HH].log). Returns
// the most recently modified evey-*.log so admin tail/stream picks up the
// hourly rotation too.
func currentLogPath(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var newestPath string
	var newestMtime time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "evey-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if newestPath == "" || info.ModTime().After(newestMtime) {
			newestPath = filepath.Join(dir, name)
			newestMtime = info.ModTime()
		}
	}
	if newestPath == "" {
		return "", os.ErrNotExist
	}
	return newestPath, nil
}

func parseLineCount(r *http.Request, defaultN, maxN int) int {
	v := r.URL.Query().Get("lines")
	if v == "" {
		return defaultN
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > maxN {
		return defaultN
	}
	return n
}

func sseWriteLine(w http.ResponseWriter, line string) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", line)
	return err
}

func sseWritePing(w http.ResponseWriter) error {
	_, err := fmt.Fprintf(w, ": ping\n\n")
	return err
}

func readNewLogBytes(path string, lastSize int64, maxBytes int) ([]byte, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, lastSize, err
	}
	newSize := info.Size()
	if newSize < lastSize {
		lastSize = 0
	}
	if newSize == lastSize {
		return nil, lastSize, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, lastSize, err
	}
	defer f.Close()

	if _, err := f.Seek(lastSize, 0); err != nil {
		return nil, lastSize, err
	}
	buf := make([]byte, maxBytes)
	n, readErr := f.Read(buf)
	if readErr != nil && n == 0 {
		return nil, lastSize, readErr
	}
	return buf[:n], lastSize + int64(n), nil
}

func (h *adminHandler) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.StartSpan(r.Context(), "http.admin.GetLogs")
	defer span.End()
	lines := parseLineCount(r, 200, 2000)
	if lines == 0 {
		lines = 200
	}

	logPath, err := currentLogPath(h.logDir)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "log file not found")
		http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
		return
	}
	data, err := tailFile(logPath, lines)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "log file not found")
		http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"lines": data}); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

// handleSearchLogs powers the admin log-debug UI: it scans the rotating
// evey-*.log files, applies the query filters (level/where/source/ids/text/
// time) and returns classified, newest-first entries with pagination.
func (h *adminHandler) handleSearchLogs(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.StartSpan(r.Context(), "http.admin.SearchLogs")
	defer span.End()

	f := parseLogSearchFilter(r)
	res, err := searchLogs(h.logDir, f)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "log dir unreadable")
		http.Error(w, `{"message":"log directory not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
	}
}

func parseLogSearchFilter(r *http.Request) logSearchFilter {
	q := r.URL.Query()
	f := logSearchFilter{
		Q:         strings.TrimSpace(q.Get("q")),
		Where:     q.Get("where"),
		Source:    q.Get("source"),
		SessionID: q.Get("session_id"),
		UserID:    q.Get("user_id"),
		MediaID:   q.Get("media_id"),
		RequestID: q.Get("request_id"),
		Levels:    parseLevels(q.Get("level")),
		Since:     parseLogTime(q.Get("since")),
		Until:     parseLogTime(q.Get("until")),
		Limit:     parseLogSearchInt(q.Get("limit"), 200, 2000),
		// Clamped to what a single request can actually retain, so the API
		// never accepts an offset it would answer with a silent empty page.
		Offset: parseLogSearchInt(q.Get("offset"), 0, logRetainCapMax),
	}
	switch strings.ToLower(q.Get("hide_access")) {
	case "1", "true", "yes":
		f.HideAccess = true
	}
	return f
}

// parseLevels turns a comma-separated level list into a normalized set. An
// empty or all-invalid input yields nil (matches any level).
func parseLevels(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	out := map[string]bool{}
	for p := range strings.SplitSeq(raw, ",") {
		p = strings.ToUpper(strings.TrimSpace(p))
		switch p {
		case "ERROR", "WARN", "INFO", "DEBUG":
			out[p] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseLogTime accepts either RFC3339(Nano) or unix milliseconds. Empty or
// unparseable input returns the zero time (unbounded).
func parseLogTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.UnixMilli(ms).UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	return time.Time{}
}

func parseLogSearchInt(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func (h *adminHandler) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	// Span only covers the SSE setup, not per-line streaming.
	_, span := tracing.StartSpan(r.Context(), "http.admin.StreamLogsOpen")
	flusher, ok := w.(http.Flusher)
	if !ok {
		span.SetStatus(otelcodes.Error, "streaming unsupported")
		span.End()
		http.Error(w, `{"message":"streaming unsupported"}`, http.StatusInternalServerError)
		return
	}
	span.End()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	lines := parseLineCount(r, 200, 2000)
	logPath, err := currentLogPath(h.logDir)
	if err != nil {
		// No log file yet - degrade to ping-only follow so the client stays
		// connected and we'll start sending as soon as one appears.
		h.followLog(w, flusher, r, "", 0)
		return
	}

	lastSize := initialFileSize(logPath)

	if lines > 0 {
		h.sendInitialLines(w, flusher, logPath, lines)
	}

	h.followLog(w, flusher, r, logPath, lastSize)
}

func initialFileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func (h *adminHandler) sendInitialLines(w http.ResponseWriter, flusher http.Flusher, logPath string, lines int) {
	initial, err := tailFile(logPath, lines)
	if err != nil {
		slog.Warn("tail log file", slog.Any("err", err))
	}
	for _, l := range initial {
		if l != "" {
			if err := sseWriteLine(w, l); err != nil {
				slog.Warn("sse write initial log line failed", slog.Any("err", err))
			}
		}
	}
	flusher.Flush()
}

func (h *adminHandler) followLog(w http.ResponseWriter, flusher http.Flusher, r *http.Request, logPath string, lastSize int64) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	const maxBytesPerTick = 64 * 1024

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// Re-resolve the active log file each tick so we follow hourly/daily
			// rotation without dropping the SSE connection.
			if active, err := currentLogPath(h.logDir); err == nil && active != logPath {
				logPath = active
				lastSize = 0
			}
			buf, newLastSize, err := readNewLogBytes(logPath, lastSize, maxBytesPerTick)
			lastSize = newLastSize
			if err != nil {
				continue
			}

			if buf == nil {
				if err := sseWritePing(w); err != nil {
					slog.Debug("sse client gone", slog.Any("err", err))
					return
				}
				flusher.Flush()
				continue
			}

			newLines := strings.Split(string(buf), "\n")
			for _, l := range newLines {
				if l != "" {
					if err := sseWriteLine(w, redactLogLine(l)); err != nil {
						slog.Debug("sse client gone", slog.Any("err", err))
						return
					}
				}
			}
			flusher.Flush()
		}
	}
}

func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tailFile: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("tailFile: %w", err)
	}

	size := stat.Size()
	if size == 0 {
		return nil, nil
	}

	readSize := min(int64(512*1024), size)
	offset := size - readSize
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil {
		return nil, fmt.Errorf("tailFile: %w", err)
	}

	rawLines := strings.Split(string(buf), "\n")
	for len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) > n {
		rawLines = rawLines[len(rawLines)-n:]
	}
	for i, l := range rawLines {
		rawLines[i] = redactLogLine(l)
	}
	return rawLines, nil
}
