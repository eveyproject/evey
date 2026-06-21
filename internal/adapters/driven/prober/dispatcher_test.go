package prober

import (
	"context"
	"errors"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
)

type fakeFallback struct {
	calls int
}

func (f *fakeFallback) ProbeStreams(_ context.Context, _ string) (*library.ProbeResult, error) {
	f.calls++
	return &library.ProbeResult{Streams: []*library.MediaStream{{Codec: "FFPROBE_SENTINEL"}}}, nil
}

type fakeMeta struct {
	isEBML bool
	isMP4  bool
	c      *extmkv.Container
	err    error
	reads  int
}

func (m *fakeMeta) IsEBML(string) bool { return m.isEBML }
func (m *fakeMeta) IsMP4(string) bool  { return m.isMP4 }
func (m *fakeMeta) ReadMeta(context.Context, string) (*extmkv.Container, error) {
	m.reads++
	return m.c, m.err
}

func mappable() *extmkv.Container {
	return &extmkv.Container{Tracks: []extmkv.Track{
		{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080)), ColorSpace: new(cicpBT709)},
	}}
}

// --- unrecognized container -> ffprobe ---

func TestDispatcher_Unrecognized_DelegatesToFFprobe(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isEBML: false, isMP4: false}
	d := &Dispatcher{fallback: fb, meta: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.avi")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls)
	assert.Equal(t, 0, m.reads, "must not open an unrecognized file with mkvgo")
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

// --- EBML (mkv) path ---

func TestDispatcher_EBML_Mappable_FastPath(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isEBML: true, c: mappable()}
	d := &Dispatcher{fallback: fb, meta: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 0, fb.calls, "mappable .mkv must NOT hit ffprobe")
	assert.Equal(t, 1, m.reads)
	assert.Equal(t, "h264", res.Streams[0].Codec)
}

func TestDispatcher_EBML_ReadMetaError_Fallback(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isEBML: true, err: errors.New("corrupt head")}
	d := &Dispatcher{fallback: fb, meta: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls, "ReadMeta error -> ffprobe is the authority")
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

func TestDispatcher_EBML_Unmappable_Fallback(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isEBML: true, c: &extmkv.Container{}} // zero tracks -> unmappable
	d := &Dispatcher{fallback: fb, meta: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls)
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

func TestDispatcher_MKVFlagOff_AlwaysFFprobe(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isEBML: true, c: mappable()}
	d := &Dispatcher{fallback: fb, meta: m, mkvEnabled: func() bool { return false }}
	_, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls, "mkv flag off -> pure pass-through to ffprobe")
	assert.Equal(t, 0, m.reads)
}

// --- ISO-BMFF (mp4) path ---

func TestDispatcher_MP4_Mappable_FastPath(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isMP4: true, c: mappable()}
	d := &Dispatcher{fallback: fb, meta: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mp4")
	require.NoError(t, err)
	assert.Equal(t, 0, fb.calls, "mappable .mp4 must NOT hit ffprobe")
	assert.Equal(t, 1, m.reads)
	assert.Equal(t, "h264", res.Streams[0].Codec)
}

func TestDispatcher_MP4FlagOff_AlwaysFFprobe(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMeta{isMP4: true, c: mappable()}
	d := &Dispatcher{fallback: fb, meta: m, mp4Enabled: func() bool { return false }}
	_, err := d.ProbeStreams(context.Background(), "/x/movie.mp4")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls, "mp4 flag off -> pure pass-through to ffprobe")
	assert.Equal(t, 0, m.reads)
}

// The two gates are independent: mkv enabled + mp4 disabled means an .mp4 still
// goes to ffprobe even though the .mkv fast-path is live.
func TestDispatcher_GatesIndependent_MP4OffMKVOn(t *testing.T) {
	on := func() bool { return true }
	off := func() bool { return false }

	mkv := &fakeMeta{isEBML: true, c: mappable()}
	dMKV := &Dispatcher{fallback: &fakeFallback{}, meta: mkv, mkvEnabled: on, mp4Enabled: off}
	_, err := dMKV.ProbeStreams(context.Background(), "/x/a.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, mkv.reads, "mkv fast-path stays live")

	mp4 := &fakeMeta{isMP4: true, c: mappable()}
	fb := &fakeFallback{}
	dMP4 := &Dispatcher{fallback: fb, meta: mp4, mkvEnabled: on, mp4Enabled: off}
	_, err = dMP4.ProbeStreams(context.Background(), "/x/b.mp4")
	require.NoError(t, err)
	assert.Equal(t, 0, mp4.reads, "mp4 fast-path stays gated off")
	assert.Equal(t, 1, fb.calls)
}

func TestDispatcher_ImplementsStreamProber(t *testing.T) {
	var _ library.StreamProber = New(&fakeFallback{}, nil, nil)
}
