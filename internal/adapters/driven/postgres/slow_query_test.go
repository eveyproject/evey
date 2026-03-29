package postgres

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLogSlowQuery_BelowThreshold(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(slog.Default())

	LogSlowQuery(context.Background(), time.Now(), "SELECT 1")()

	if buf.Len() != 0 {
		t.Fatalf("expected no log, got: %s", buf.String())
	}
}

func TestLogSlowQuery_AboveThreshold(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(slog.Default())

	start := time.Now().Add(-200 * time.Millisecond)
	LogSlowQuery(context.Background(), start, "SELECT * FROM users WHERE id = $1")()

	out := buf.String()
	if !strings.Contains(out, "slow query") {
		t.Fatalf("expected 'slow query' in log, got: %s", out)
	}
	if !strings.Contains(out, "SELECT * FROM users") {
		t.Fatalf("expected query in log, got: %s", out)
	}
}

func TestLogSlowQuery_TruncatesLongQuery(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(slog.Default())

	longQuery := strings.Repeat("x", 300)
	start := time.Now().Add(-200 * time.Millisecond)
	LogSlowQuery(context.Background(), start, longQuery)()

	out := buf.String()
	if strings.Contains(out, strings.Repeat("x", 300)) {
		t.Fatalf("query should be truncated")
	}
}

func TestLogSlowQuery_IncludesError(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(slog.Default())

	start := time.Now().Add(-200 * time.Millisecond)
	LogSlowQuery(context.Background(), start, "SELECT 1")(errors.New("connection refused"))

	out := buf.String()
	if !strings.Contains(out, "connection refused") {
		t.Fatalf("expected error in log, got: %s", out)
	}
}
