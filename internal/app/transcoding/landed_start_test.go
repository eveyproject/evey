package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

// landedProcess is fakeProcess whose sessions start one segment before the
// one asked, the way a transcode seek past the last video frame backs off
// (backOffToVideo in the ffmpeg adapter).
type landedProcess struct{ *fakeProcess }

type landedHandle struct {
	*fakeHandle
	landed int
}

func (h landedHandle) LandedStartSegment() int { return h.landed }

func (p landedProcess) Start(ctx context.Context, opts transcoding.HLSProcessOpts) (transcoding.HLSProcessHandle, error) {
	h, err := p.fakeProcess.Start(ctx, opts)
	if err != nil {
		return nil, err
	}
	return landedHandle{fakeHandle: h.(*fakeHandle), landed: opts.StartNumber - 1}, nil
}

// The session writes the segment before the requested one: a request for it
// is covered, not a second encoder over the same names.
func TestSoftwareProducer_ASessionThatStartedEarlierCoversTheSegmentsBefore(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := landedProcess{newFakeProcess()}
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 40))
	require.Equal(t, 1, proc.startCount())
	require.Equal(t, 40, proc.lastOpts().StartNumber)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 39))
	assert.Equal(t, 1, proc.startCount(), "segment 39 is the session's own first segment")
}

func TestSegmentOrchestrator_ASessionThatStartedEarlierCoversTheSegmentsBefore(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := landedProcess{newFakeProcess()}
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 40))
	require.Equal(t, 1, proc.startCount())
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 39))
	assert.Equal(t, 1, proc.startCount(), "segment 39 is the session's own first segment")
}
