package metadata

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/pkg/logger"
)

// A provider failing every call warns once a minute per message, the line
// that goes through counting the ones held back.
func TestWarnSampled_OneLinePerMinuteWithTheCountHeldBack(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	defer slog.SetDefault(prev)

	now := time.Unix(1000, 0)
	sampler := logger.NewSamplerForTest(time.Minute, func() time.Time { return now })
	ctx := context.Background()
	for range 5 {
		warnSampled(ctx, sampler, "fetch translations", slog.String("item", "m1"))
	}
	if n := strings.Count(buf.String(), "fetch translations"); n != 1 {
		t.Fatalf("lines inside the window = %d, want 1:\n%s", n, buf.String())
	}

	now = now.Add(time.Minute)
	warnSampled(ctx, sampler, "fetch translations", slog.String("item", "m6"))
	if n := strings.Count(buf.String(), "fetch translations"); n != 2 {
		t.Fatalf("lines after the window = %d, want 2:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), "held_back=4") {
		t.Fatalf("the line after the window must count the 4 held back:\n%s", buf.String())
	}
}
