package fakes

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

var _ transcoding.HLSProcess = (*HLSProcess)(nil)

type HLSProcess struct {
	mu      sync.Mutex
	Started []transcoding.HLSProcessOpts
	Err     error // if set, Start returns this error
	// HandleFailure, when set, is reported by every spawned handle's Failure()
	// - lets a test drive the level-2 safety net (e.g. a HW-capability class).
	HandleFailure transcoding.FailureClass
}

func (f *HLSProcess) Start(_ context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	f.mu.Lock()
	f.Started = append(f.Started, opts)
	f.mu.Unlock()

	if f.Err != nil {
		return nil, f.Err
	}

	// Create fake m3u8 files so isTranscodeComplete/WaitReady works
	isSingle := opts.VideoDecision == "copy" || opts.VideoDecision == "transcode"
	if isSingle {
		_ = os.WriteFile(filepath.Join(opts.OutputDir, "stream_0.m3u8"), []byte("#EXTM3U\n#EXT-X-ENDLIST\n"), 0644)
		_ = os.WriteFile(filepath.Join(opts.OutputDir, "master.m3u8"), []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=5000000\nstream_0.m3u8\n"), 0644)
	} else {
		for i := range opts.Profiles {
			content := "#EXTM3U\n#EXT-X-ENDLIST\n"
			_ = os.WriteFile(filepath.Join(opts.OutputDir, fmtStream(i)), []byte(content), 0644)
		}
		_ = os.WriteFile(filepath.Join(opts.OutputDir, "master.m3u8"), []byte("#EXTM3U\n"), 0644)
	}

	h := &fakeHandle{
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
		failure: f.HandleFailure,
	}
	close(h.ready)
	close(h.done)
	return h, nil
}

func fmtStream(i int) string {
	return "stream_" + string(rune('0'+i)) + ".m3u8"
}

type fakeHandle struct {
	ready   chan struct{}
	done    chan struct{}
	failure transcoding.FailureClass
}

func (h *fakeHandle) WaitReady(_ context.Context) error { <-h.ready; return nil }
func (h *fakeHandle) Stop() error                       { return nil }
func (h *fakeHandle) Done() <-chan struct{}             { return h.done }
func (h *fakeHandle) Err() error                        { return nil }
func (h *fakeHandle) Pause() error                      { return nil }
func (h *fakeHandle) Resume() error                     { return nil }
func (h *fakeHandle) Failure() transcoding.FailureClass { return h.failure }
