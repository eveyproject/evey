package ffmpeg

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/logger"
)

// ctxRecorder keeps, per message, the request id the record's context carried.
type ctxRecorder struct {
	mu   sync.Mutex
	seen map[string]string
}

func (r *ctxRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (r *ctxRecorder) Handle(ctx context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen[rec.Message] = logger.RequestIDFromCtx(ctx)
	return nil
}
func (r *ctxRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *ctxRecorder) WithGroup(string) slog.Handler      { return r }

// The spawn line goes out through the request's context, so the ids the
// handler injects tie the ffmpeg start to the segment request that asked.
func TestStartOne_SpawnLogCarriesTheRequestContext(t *testing.T) {
	rec := &ctxRecorder{seen: map[string]string{}}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	defer slog.SetDefault(prev)

	ctx, cancel := context.WithCancel(logger.WithRequestID(context.Background(), "req-spawn-1"))
	defer cancel()
	h, err := startOne(ctx, transcoding.HLSProcessOpts{
		FilePath: "/nonexistent/source.mkv", OutputDir: t.TempDir(),
		VideoDecision: "transcode", AudioDecision: "copy", AudioCodec: "aac",
		SubtitleTrackIndex: -1, UseFMP4: true,
		Profiles: []transcoding.HLSProfile{{Name: "180p", Width: 320, Height: 180, VideoBitrate: 600, AudioBitrate: 96}},
	}, true)
	if err == nil && h != nil {
		_ = h.Stop()
	}

	rec.mu.Lock()
	got, logged := rec.seen["hls: ffmpeg start"]
	rec.mu.Unlock()
	if !logged {
		t.Fatal("the spawn must be logged")
	}
	if got != "req-spawn-1" {
		t.Fatalf("spawn log request id = %q, want the request's", got)
	}
}
