package ffmpeg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// An ffmpeg that exits on its own must still cancel its process context:
// otherwise the context stays registered on its long-lived parent forever.
func TestHLSHandleRun_NaturalExitReleasesItsContext(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"stream_0.m3u8", "stream_0_000.m4s"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &hlsHandle{
		opts:       transcoding.HLSProcessOpts{OutputDir: dir, VideoDecision: "copy"},
		cancel:     cancel,
		ready:      make(chan struct{}),
		done:       make(chan struct{}),
		procExited: make(chan struct{}),
		stderr:     newRingBuffer(1024),
		noMaster:   true,
	}
	close(h.procExited)
	go h.run(ctx)
	select {
	case <-h.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("run never returned")
	}
	if ctx.Err() == nil {
		t.Fatal("the process context is still live after a natural exit")
	}
}

func TestCompositeHandleReap_ReleasesItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := &hlsHandle{done: make(chan struct{})}
	close(sub.done)
	c := &compositeHandle{ctx: ctx, cancel: cancel, subs: []*hlsHandle{sub}, done: make(chan struct{})}
	c.reap()
	if ctx.Err() == nil {
		t.Fatal("the composite context is still live after every rung exited")
	}
}
