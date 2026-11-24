package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainplayback "github.com/eveyproject/evey/internal/domain/playback"
)

// The served witness's fixtures are real ffmpeg inits (lavfi test patterns):
// a muxed H.264 + AAC init, an MP3 audio-only init, an HEVC video-only init.
func witnessInit(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "served_witness", name))
	require.NoError(t, err)
	return b
}

type recordedLeg struct {
	profile, media string
	leg            domainplayback.Leg
	l              domainplayback.ServedLeg
}

type legRecorder struct {
	mu   sync.Mutex
	legs []recordedLeg
}

func (r *legRecorder) record(_ context.Context, profileID, mediaID string, leg domainplayback.Leg, l domainplayback.ServedLeg) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.legs = append(r.legs, recordedLeg{profileID, mediaID, leg, l})
	return nil
}

func (r *legRecorder) byLeg(leg domainplayback.Leg) []domainplayback.ServedLeg {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domainplayback.ServedLeg
	for _, x := range r.legs {
		if x.leg == leg {
			out = append(out, x.l)
		}
	}
	return out
}

func witnessRequest(method string) *http.Request {
	req := httptest.NewRequest(method, "/api/v1/media/m1/hls/x", nil)
	return withHTTPClaims(req, testClaims(false))
}

// An init that goes out is read: its codecs, picture and channels are what
// the session is told, next to the machinery the handler declared.
func TestServedWitness_ReadsTheInitThatWentOut(t *testing.T) {
	rec := &legRecorder{}
	sw := &servedWitness{record: rec.record}
	init := witnessInit(t, "init_h264_aac.mp4")
	h := sw.wrap(func(w http.ResponseWriter, r *http.Request) {
		st := deliveryStampOf(r)
		st.stampVideo("m1", domainplayback.EngineFFmpeg, false, "qsv", "hevc")
		st.stampAudio("m1", domainplayback.EngineFFmpeg, false, 1, "mp3", false)
		st.stampInit("ffmpeg:fp:0")
		_, _ = w.Write(init)
	})
	h(httptest.NewRecorder(), witnessRequest(http.MethodGet))

	video := rec.byLeg(domainplayback.LegVideo)
	audio := rec.byLeg(domainplayback.LegAudio)
	require.Len(t, video, 1)
	require.Len(t, audio, 1)
	assert.Equal(t, "h264", video[0].Codec, "read from the init, not declared")
	assert.Equal(t, 640, video[0].Width)
	assert.Equal(t, 360, video[0].Height)
	assert.Equal(t, "hevc", video[0].SourceCodec)
	assert.Equal(t, "qsv", video[0].Backend)
	assert.False(t, video[0].Copy)
	assert.Equal(t, "aac", audio[0].Codec)
	assert.Equal(t, "mp3", audio[0].SourceCodec)
	assert.Equal(t, 1, audio[0].Track)
	assert.Equal(t, "p1", rec.legs[0].profile)
	assert.Equal(t, "m1", rec.legs[0].media)
}

// A segment is described by the init it decodes with: read once when the
// witness never saw that init go out (a browser keeps it cached), then from
// memory.
func TestServedWitness_ASegmentIsDescribedByItsInit(t *testing.T) {
	rec := &legRecorder{}
	sw := &servedWitness{record: rec.record}
	init := witnessInit(t, "init_mp3.mp4")
	reads := 0
	h := sw.wrap(func(w http.ResponseWriter, r *http.Request) {
		st := deliveryStampOf(r)
		st.stampAudio("m1", domainplayback.EngineFFmpeg, true, 0, "", false)
		st.stampMedia("ffmpeg-audio:fp:0", func(context.Context) ([]byte, error) { reads++; return init, nil })
		_, _ = w.Write([]byte("moof+mdat"))
	})
	for range 3 {
		h(httptest.NewRecorder(), witnessRequest(http.MethodGet))
	}
	assert.Equal(t, 1, reads, "the init is read once")
	audio := rec.byLeg(domainplayback.LegAudio)
	require.Len(t, audio, 3)
	assert.Equal(t, "mp3", audio[2].Codec)
	assert.True(t, audio[2].Copy)
	assert.Empty(t, audio[2].SourceCodec, "a copy names no source codec beside the same codec")
}

// A rung whose init carries no audio did not carry the audio leg: the
// handler declares both, the bytes say which went out.
func TestServedWitness_AVideoOnlyRungCarriesNoAudioLeg(t *testing.T) {
	rec := &legRecorder{}
	sw := &servedWitness{record: rec.record}
	init := witnessInit(t, "init_hevc_video_only.mp4")
	h := sw.wrap(func(w http.ResponseWriter, r *http.Request) {
		st := deliveryStampOf(r)
		st.stampVideo("m1", domainplayback.EngineFFmpeg, true, "", "")
		st.stampAudio("m1", domainplayback.EngineFFmpeg, true, 0, "", false)
		st.stampInit("ffmpeg:fp:0")
		_, _ = w.Write(init)
	})
	h(httptest.NewRecorder(), witnessRequest(http.MethodGet))
	require.Len(t, rec.byLeg(domainplayback.LegVideo), 1)
	assert.Equal(t, "hevc", rec.byLeg(domainplayback.LegVideo)[0].Codec)
	assert.Empty(t, rec.byLeg(domainplayback.LegAudio))
}

// Only a response that carried bytes is evidence: an error, a HEAD, an empty
// body and bytes that cannot be described are not recorded.
func TestServedWitness_NothingServedNothingRecorded(t *testing.T) {
	init := witnessInit(t, "init_h264_aac.mp4")
	cases := []struct {
		name    string
		method  string
		handler http.HandlerFunc
	}{
		{"a 503", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			deliveryStampOf(r).stampVideo("m1", domainplayback.EngineFFmpeg, false, "", "")
			deliveryStampOf(r).stampInit("k")
			http.Error(w, "not yet", http.StatusServiceUnavailable)
		}},
		{"a HEAD", http.MethodHead, func(w http.ResponseWriter, r *http.Request) {
			deliveryStampOf(r).stampVideo("m1", domainplayback.EngineFFmpeg, false, "", "")
			deliveryStampOf(r).stampInit("k")
			w.WriteHeader(http.StatusOK)
		}},
		{"an empty 200", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			deliveryStampOf(r).stampVideo("m1", domainplayback.EngineFFmpeg, false, "", "")
			deliveryStampOf(r).stampInit("k")
			w.WriteHeader(http.StatusOK)
		}},
		{"an init that does not parse", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			deliveryStampOf(r).stampVideo("m1", domainplayback.EngineFFmpeg, false, "", "")
			deliveryStampOf(r).stampInit("k")
			_, _ = w.Write([]byte("not an mp4"))
		}},
		{"a segment whose init cannot be read", http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
			deliveryStampOf(r).stampVideo("m1", domainplayback.EngineFFmpeg, false, "", "")
			deliveryStampOf(r).stampMedia("k2", func(context.Context) ([]byte, error) { return nil, errors.New("gone") })
			_, _ = w.Write(init)
		}},
		{"a playlist (nothing stamped)", http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("#EXTM3U\n"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &legRecorder{}
			sw := &servedWitness{record: rec.record}
			sw.wrap(tc.handler)(httptest.NewRecorder(), witnessRequest(tc.method))
			assert.Empty(t, rec.legs)
		})
	}
}

// A direct play reads the whole film through one request: it is recorded at
// its first bytes, while the handler is still writing, from the file's own
// streams. The audio leg names no track and no codec: the file carries every
// track and the player picks.
func TestServedWitness_ADirectPlayIsRecordedAtItsFirstBytes(t *testing.T) {
	rec := &legRecorder{}
	sw := &servedWitness{record: rec.record}
	w, hgt := 1920, 1080
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "mjpeg"},
		{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264", Width: &w, Height: &hgt},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "ac3"},
		{StreamType: domainlibrary.StreamTypeAudio, Codec: "aac"},
	}
	recordedWhileWriting := false
	h := sw.wrap(func(rw http.ResponseWriter, r *http.Request) {
		deliveryStampOf(r).stampFile("m1", "file:m1:fp", func(context.Context) ([]*domainlibrary.MediaStream, error) { return streams, nil })
		rw.WriteHeader(http.StatusPartialContent)
		_, _ = rw.Write([]byte("first range bytes"))
		recordedWhileWriting = len(rec.legs) > 0
		_, _ = rw.Write([]byte("the rest of the film"))
	})
	h(httptest.NewRecorder(), witnessRequest(http.MethodGet))

	assert.True(t, recordedWhileWriting, "recorded at the first bytes, not when the film ends")
	video := rec.byLeg(domainplayback.LegVideo)
	audio := rec.byLeg(domainplayback.LegAudio)
	require.Len(t, video, 1, "recorded once per response")
	require.Len(t, audio, 1)
	assert.Equal(t, domainplayback.EngineFile, video[0].Engine)
	assert.Equal(t, "h264", video[0].Codec, "the picture, not the cover art")
	assert.Equal(t, 1080, video[0].Height)
	assert.True(t, video[0].Copy)
	assert.Equal(t, -1, audio[0].Track)
	assert.Empty(t, audio[0].Codec)
}

// A packager rendition names the source track by the number the file gives
// it; the witness maps it to the audio ordinal the session speaks.
func TestAudioOrdinalOfSourceTrack(t *testing.T) {
	id := func(n int) *int { return &n }
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.StreamTypeVideo, SourceTrackID: id(1)},
		{StreamType: domainlibrary.StreamTypeAudio, SourceTrackID: id(2)},
		{StreamType: domainlibrary.StreamTypeSubtitle, SourceTrackID: id(3)},
		{StreamType: domainlibrary.StreamTypeAudio, SourceTrackID: id(4)},
	}
	ord, ok := audioOrdinalOfSourceTrack(streams, 4)
	assert.True(t, ok)
	assert.Equal(t, 1, ord)
	_, ok = audioOrdinalOfSourceTrack(streams, 3)
	assert.False(t, ok, "a subtitle is not an audio track")
}

func TestPackagerInitOf(t *testing.T) {
	cases := []struct {
		name, init string
		isInit, ok bool
	}{
		{"init.mp4", "init.mp4", true, true},
		{"seg00042.m4s", "init.mp4", false, true},
		{"init_a2.mp4", "init_a2.mp4", true, true},
		{"seg_a2_00042.m4s", "init_a2.mp4", false, true},
		{"playlist.m3u8", "", false, false},
		{"sub1.vtt", "", false, false},
	}
	for _, c := range cases {
		init, isInit, ok := packagerInitOf(c.name)
		assert.Equal(t, c.init, init, c.name)
		assert.Equal(t, c.isInit, isInit, c.name)
		assert.Equal(t, c.ok, ok, c.name)
	}
}
