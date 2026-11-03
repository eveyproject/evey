package transcoding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The ffmpeg adapter logs its spawn, its exit error and its stderr tail; every
// other line of a playback carries the media and the decision hash that names
// the session. Both producers hand them to the process so an ffmpeg failure in
// the logs ties back to the session and the title that spawned it.

func TestSoftwareProducer_SpawnCarriesMediaAndSession(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	p := NewSoftwareProducer(cache, proc, stubSource{path: "/media/test.mkv"}, swTestOpts(), discardLogger())
	defer p.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, p.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, fp.MediaID, proc.lastOpts().MediaID)
	assert.Equal(t, fp.Hash(), proc.lastOpts().SessionID)
}

func TestOrchestrator_SpawnCarriesMediaAndSession(t *testing.T) {
	cache, _ := newTestCache(t)
	proc := newFakeProcess()
	o := NewSegmentOrchestrator(cache, proc, stubSource{path: "/media/test.mkv"}, testOpts(), discardLogger())
	defer o.Close()

	fp := minimalFingerprint(t)
	require.NoError(t, o.EnsureSegmentRange(context.Background(), fp, 0, 0))
	assert.Equal(t, fp.MediaID, proc.lastOpts().MediaID)
	assert.Equal(t, fp.Hash(), proc.lastOpts().SessionID)
}
