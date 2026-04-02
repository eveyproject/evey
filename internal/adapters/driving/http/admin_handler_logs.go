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
)

func (h *adminHandler) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if !h.adminCheckWithQueryToken(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}

	logPath := h.logDir + "/evey.log"
	data, err := tailFile(logPath, lines)
	if err != nil {
		http.Error(w, `{"message":"log file not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"lines": data}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func (h *adminHandler) handleStreamLogs(w http.ResponseWriter, r *http.Request) {
	if !h.adminCheckWithQueryToken(r) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"message":"streaming unsupported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")

	lines := 200
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n >= 0 && n <= 2000 {
			lines = n
		}
	}

	logPath := h.logDir + "/evey.log"

	var lastSize int64
	if info, err := os.Stat(logPath); err == nil {
		lastSize = info.Size()
	}

	if lines > 0 {
		initial, tailErr := tailFile(logPath, lines)
		if tailErr != nil {
			slog.Warn("tail log file", slog.Any("err", tailErr))
		}
		for _, l := range initial {
			if l != "" {
				fmt.Fprintf(w, "data: %s\n\n", l)
			}
		}
		flusher.Flush()
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	const maxBytesPerTick = 64 * 1024

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			info, err := os.Stat(logPath)
			if err != nil {
				continue
			}
			newSize := info.Size()
			if newSize < lastSize {
				lastSize = 0
			}
			if newSize == lastSize {
				if _, writeErr := fmt.Fprintf(w, ": ping\n\n"); writeErr != nil {
					slog.Debug("sse client gone", slog.Any("err", writeErr))
					return
				}
				flusher.Flush()
				continue
			}

			f, err := os.Open(logPath)
			if err != nil {
				continue
			}
			if _, seekErr := f.Seek(lastSize, 0); seekErr != nil {
				f.Close()
				continue
			}
			buf := make([]byte, maxBytesPerTick)
			n, readErr := f.Read(buf)
			f.Close()
			if readErr != nil && n == 0 {
				continue
			}
			lastSize += int64(n)

			newLines := strings.Split(string(buf[:n]), "\n")
			for _, l := range newLines {
				if l != "" {
					if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", l); writeErr != nil {
						slog.Debug("sse client gone", slog.Any("err", writeErr))
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
