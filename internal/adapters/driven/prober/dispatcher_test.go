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

type fakeMKV struct {
	isEBML bool
	c      *extmkv.Container
	err    error
	reads  int
}

func (m *fakeMKV) IsEBML(string) bool { return m.isEBML }
func (m *fakeMKV) ReadMeta(context.Context, string) (*extmkv.Container, error) {
	m.reads++
	return m.c, m.err
}

func mappable() *extmkv.Container {
	return &extmkv.Container{Tracks: []extmkv.Track{
		{Type: extmkv.VideoTrack, Codec: "h264", Width: new(uint32(1920)), Height: new(uint32(1080)), ColorSpace: new(cicpBT709)},
	}}
}

func TestDispatcher_NonEBML_DelegatesToFFprobe(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMKV{isEBML: false}
	d := &Dispatcher{fallback: fb, mkv: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mp4")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls)
	assert.Equal(t, 0, m.reads, "must not even open a non-EBML file with mkvgo")
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

func TestDispatcher_EBML_Mappable_FastPath(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMKV{isEBML: true, c: mappable()}
	d := &Dispatcher{fallback: fb, mkv: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 0, fb.calls, "mappable .mkv must NOT hit ffprobe")
	assert.Equal(t, 1, m.reads)
	assert.Equal(t, "h264", res.Streams[0].Codec)
}

func TestDispatcher_EBML_ReadMetaError_Fallback(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMKV{isEBML: true, err: errors.New("corrupt head")}
	d := &Dispatcher{fallback: fb, mkv: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls, "ReadMeta error -> ffprobe is the authority")
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

func TestDispatcher_EBML_Unmappable_Fallback(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMKV{isEBML: true, c: &extmkv.Container{}} // zero tracks -> unmappable
	d := &Dispatcher{fallback: fb, mkv: m}
	res, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls)
	assert.Equal(t, "FFPROBE_SENTINEL", res.Streams[0].Codec)
}

func TestDispatcher_FlagOff_AlwaysFFprobe(t *testing.T) {
	fb := &fakeFallback{}
	m := &fakeMKV{isEBML: true, c: mappable()}
	d := &Dispatcher{fallback: fb, mkv: m, enabled: func() bool { return false }}
	_, err := d.ProbeStreams(context.Background(), "/x/movie.mkv")
	require.NoError(t, err)
	assert.Equal(t, 1, fb.calls, "flag off -> pure pass-through to ffprobe")
	assert.Equal(t, 0, m.reads)
}

func TestDispatcher_ImplementsStreamProber(t *testing.T) {
	var _ library.StreamProber = New(&fakeFallback{}, nil)
}
