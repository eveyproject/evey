package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/pkg/tracing"
)

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

	logPath := h.logDir + "/evey.log"
	data, err := tailFile(logPath, lines)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "log file not found")
		http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"lines": data}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
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
	logPath := h.logDir + "/evey.log"

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
					if err := sseWriteLine(w, l); err != nil {
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
	return rawLines, nil
}
