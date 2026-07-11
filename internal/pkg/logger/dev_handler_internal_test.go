package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestDevHandler_GroupQualifiesKeys locks in the fix for the previously-ignored
// WithGroup: the dev handler stored the group name but never read it, so grouped
// attributes rendered without any qualifier. The handler writes to an unexported
// io.Writer, so this must be an internal (package logger) test.
func TestDevHandler_GroupQualifiesKeys(t *testing.T) {
	var buf bytes.Buffer
	h := (&devHandler{w: &buf}).
		WithGroup("http").
		WithAttrs([]slog.Attr{slog.String("method", "GET")})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "req", 0)
	rec.AddAttrs(slog.Int("status", 200))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	out := buf.String()
	// Attr added after WithGroup is qualified at add time; record attr is qualified
	// at render time. Both must carry the "http." prefix.
	for _, want := range []string{"http.method=GET", "http.status=200"} {
		if !strings.Contains(out, want) {
			t.Errorf("dev output missing %q; got: %q", want, out)
		}
	}
	// The bare (unqualified) keys must NOT appear - that was the bug.
	if strings.Contains(out, " method=GET") || strings.Contains(out, " status=200") {
		t.Errorf("dev output has unqualified group keys (WithGroup ignored); got: %q", out)
	}
}

// TestDevHandler_TopLevelAttrsUnprefixed verifies attrs added before any group
// stay unqualified (prefix is empty at the top level).
func TestDevHandler_TopLevelAttrsUnprefixed(t *testing.T) {
	var buf bytes.Buffer
	h := (&devHandler{w: &buf}).WithAttrs([]slog.Attr{slog.String("component", "scan")})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !strings.Contains(buf.String(), "component=scan") {
		t.Errorf("top-level attr missing; got: %q", buf.String())
	}
}
