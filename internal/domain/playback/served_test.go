package playback_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/playback"
)

func leg(engine string, copied bool) *playback.ServedLeg {
	return &playback.ServedLeg{Engine: engine, Copy: copied, Track: -1}
}

// The method names what the bytes were, read from the served legs alone.
func TestServedMethod(t *testing.T) {
	cases := []struct {
		name   string
		served playback.Served
		want   playback.PlayMethod
	}{
		{"nothing served yet", playback.Served{}, ""},
		{"the file itself", playback.Served{Video: leg(playback.EngineFile, true), Audio: leg(playback.EngineFile, true)}, playback.DirectPlay},
		{"packager, both copied", playback.Served{Video: leg(playback.EngineMkvgo, true), Audio: leg(playback.EngineMkvgo, true)}, playback.DirectStream},
		{"copied video, encoded audio", playback.Served{Video: leg(playback.EngineMkvgo, true), Audio: leg(playback.EngineFFmpeg, false)}, playback.Transcode},
		{"encoded video", playback.Served{Video: leg(playback.EngineFFmpeg, false), Audio: leg(playback.EngineFFmpeg, true)}, playback.Transcode},
		{"one leg seen so far, copied", playback.Served{Video: leg(playback.EngineMkvgo, true)}, playback.DirectStream},
		// A copied AC-3 rendition the browser decodes in wasm: the server sent
		// the source's samples, the method says so.
		{"copied audio for a wasm decoder", playback.Served{Video: leg(playback.EngineMkvgo, true), Audio: leg(playback.EngineFFmpeg, true)}, playback.DirectStream},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.served.Method(); got != c.want {
				t.Errorf("Method() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestServedSummary(t *testing.T) {
	s := playback.Served{
		Video: &playback.ServedLeg{Engine: playback.EngineMkvgo, Copy: true, Codec: "h264", Width: 1920, Height: 1080, Track: -1},
		Audio: &playback.ServedLeg{Engine: playback.EngineFFmpeg, Codec: "aac", SourceCodec: "mp3", Track: 1},
	}
	method, v, a, res := s.Summary()
	if method != "transcode" || v != "h264" || a != "aac" || res != "1080p" {
		t.Errorf("Summary() = %q %q %q %q", method, v, a, res)
	}
	if m, v, a, r := (playback.Served{}).Summary(); m != "" || v != "" || a != "" || r != "" {
		t.Errorf("empty Summary() = %q %q %q %q", m, v, a, r)
	}
}

// Two observations of the same bytes are the same leg whatever the time they
// were seen at; any other difference is a change.
func TestServedLegSameBytes(t *testing.T) {
	a := playback.ServedLeg{Engine: playback.EngineFFmpeg, Codec: "aac", Track: 1, At: time.Unix(1, 0)}
	b := a
	b.At = time.Unix(99, 0)
	if !a.SameBytes(b) {
		t.Error("same bytes seen at two times must compare equal")
	}
	b.Track = 2
	if a.SameBytes(b) {
		t.Error("another track is another leg")
	}
}

func TestPlayedSameState(t *testing.T) {
	a := playback.Played{Source: playback.PlayedHLS, AudioTrack: 0, AudioCodec: "aac", At: time.Unix(1, 0)}
	b := a
	b.At = time.Unix(50, 0)
	if !a.SameState(b) {
		t.Error("same state reported twice must compare equal")
	}
	b.AudioDecoder = playback.DecoderWasm
	if a.SameState(b) {
		t.Error("another decoder is another state")
	}
}
