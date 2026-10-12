package prober

import (
	"context"
	"errors"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mp4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
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
	// trackEndMs answers TrackEndMs (with trackEndErr); trackEndCalls counts.
	trackEndMs    int64
	trackEndErr   error
	trackEndCalls int
}

func (m *fakeMeta) TrackEndMs(context.Context, string) (int64, error) {
	m.trackEndCalls++
	return m.trackEndMs, m.trackEndErr
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

// --- gateMP4Drops ---

// A dropped video or audio track means the container holds media the fast-path
// result would silently omit (a scan would record e.g. an h263 camera mp4 as
// audio-only) - those files must error out of the fast-path so the dispatcher
// falls back to ffprobe. Non-media handler drops (hint/tmcd/meta, Type == "")
// and subtitle drops (QuickTime chapter text) stay fast-path.
func TestGateMP4Drops(t *testing.T) {
	c := mappable()
	cases := []struct {
		name    string
		dropped []mp4.DroppedTrack
		wantErr bool
	}{
		{"no drops", nil, false},
		{"dropped video (h263 sample entry)", []mp4.DroppedTrack{
			{ID: 1, Type: extmkv.VideoTrack, Codec: "s263", Reason: "unsupported sample entry"}}, true},
		{"dropped audio", []mp4.DroppedTrack{
			{ID: 2, Type: extmkv.AudioTrack, Codec: "qdm2", Reason: "unsupported sample entry"}}, true},
		{"dropped subtitle (chapter text) tolerated", []mp4.DroppedTrack{
			{ID: 3, Type: extmkv.SubtitleTrack, Codec: "text", Reason: "unsupported sample entry"}}, false},
		{"dropped non-media handler tolerated", []mp4.DroppedTrack{
			{ID: 4, Codec: "tmcd", Reason: "non-media handler"}}, false},
		{"benign drop then video drop still errors", []mp4.DroppedTrack{
			{ID: 4, Codec: "tmcd", Reason: "non-media handler"},
			{ID: 1, Type: extmkv.VideoTrack, Codec: "s263", Reason: "unsupported sample entry"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gateMP4Drops(c, tc.dropped, nil)
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				assert.Same(t, c, got)
			}
		})
	}

	// An OpenMeta error passes through regardless of drops.
	_, err := gateMP4Drops(nil, nil, errors.New("parse failed"))
	require.Error(t, err)
}

// A Matroska that declares no Duration (written as a live stream) gets it
// measured from the tracks' ends; a declared one is never re-measured, and a
// tail that cannot be read leaves the duration unknown without leaving the
// fast path.
func TestDispatcher_MeasuresTheDurationAFileDoesNotDeclare(t *testing.T) {
	undeclared := mappable()
	m := &fakeMeta{isEBML: true, c: undeclared, trackEndMs: 1_200_042}
	d := New(&fakeFallback{}, nil, nil)
	d.meta = m
	res, err := d.ProbeStreams(context.Background(), "/live.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Duration.Milliseconds(); got != 1_200_042 {
		t.Fatalf("duration must come from the tracks' ends, got %d", got)
	}
	if m.trackEndCalls != 1 {
		t.Fatalf("one measure, got %d", m.trackEndCalls)
	}

	declared := mappable()
	declared.DurationMs = 5_000
	m = &fakeMeta{isEBML: true, c: declared, trackEndMs: 1_200_042}
	d.meta = m
	res, _ = d.ProbeStreams(context.Background(), "/normal.mkv")
	if res.Duration.Milliseconds() != 5_000 || m.trackEndCalls != 0 {
		t.Fatalf("a declared duration is kept and never re-measured: %d calls=%d", res.Duration.Milliseconds(), m.trackEndCalls)
	}

	m = &fakeMeta{isEBML: true, c: mappable(), trackEndErr: context.DeadlineExceeded}
	d.meta = m
	res, err = d.ProbeStreams(context.Background(), "/unreadable.mkv")
	if err != nil || !res.Duration.IsZero() || res.Streams[0].Codec == "FFPROBE_SENTINEL" {
		t.Fatalf("an unreadable tail keeps the fast path with an unknown duration: err=%v dur=%d", err, res.Duration.Milliseconds())
	}

	mp4 := &fakeMeta{isMP4: true, c: mappable(), trackEndMs: 99}
	d.meta = mp4
	res, _ = d.ProbeStreams(context.Background(), "/a.mp4")
	if mp4.trackEndCalls != 0 || !res.Duration.IsZero() {
		t.Fatalf("the tail walk is Matroska-only: calls=%d", mp4.trackEndCalls)
	}
}
