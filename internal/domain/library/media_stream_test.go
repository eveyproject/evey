package library_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
)

func TestStreamType_StringRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  library.StreamType
		want string
	}{
		{library.StreamTypeVideo, "video"},
		{library.StreamTypeAudio, "audio"},
		{library.StreamTypeSubtitle, "subtitle"},
		{library.StreamTypeAttachment, "attachment"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if got := c.typ.String(); got != c.want {
				t.Errorf("StreamType(%q).String() = %q, want %q", c.typ, got, c.want)
			}
		})
	}
}

func TestStreamType_IsValidStrict(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		typ  library.StreamType
		want bool
	}{
		{"video", library.StreamTypeVideo, true},
		{"audio", library.StreamTypeAudio, true},
		{"subtitle", library.StreamTypeSubtitle, true},
		{"attachment", library.StreamTypeAttachment, true},
		{"empty", library.StreamType(""), false},
		{"garbage", library.StreamType("not-a-stream"), false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.typ.IsValidStrict(); got != c.want {
				t.Errorf("StreamType(%q).IsValidStrict() = %v, want %v", c.typ, got, c.want)
			}
		})
	}
}

func TestMediaStream_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		typ       library.StreamType
		expectErr bool
	}{
		{"video valid", library.StreamTypeVideo, false},
		{"audio valid", library.StreamTypeAudio, false},
		{"subtitle valid", library.StreamTypeSubtitle, false},
		{"attachment valid", library.StreamTypeAttachment, false},
		{"empty rejected", library.StreamType(""), true},
		{"garbage rejected", library.StreamType("data"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := &library.MediaStream{StreamType: c.typ}
			err := s.Validate()
			if c.expectErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", c.typ)
				}
				if !errors.Is(err, library.ErrInvalidStreamType) {
					t.Errorf("expected ErrInvalidStreamType, got %v", err)
				}
			} else if err != nil {
				t.Errorf("expected no error for %q, got %v", c.typ, err)
			}
		})
	}
}

// TestStreamType_LegacyAliases pins the equivalence between the legacy
// short-form constants (Video/Audio/Subtitle) and the new StreamType-
// prefixed names. Once the cascade lets us delete the short-forms, this
// test deletes with them.
func TestStreamType_LegacyAliases(t *testing.T) {
	t.Parallel()

	if library.Video != library.StreamTypeVideo {
		t.Errorf("Video=%q, StreamTypeVideo=%q: must match", library.Video, library.StreamTypeVideo)
	}
	if library.Audio != library.StreamTypeAudio {
		t.Errorf("Audio=%q, StreamTypeAudio=%q: must match", library.Audio, library.StreamTypeAudio)
	}
	if library.Subtitle != library.StreamTypeSubtitle {
		t.Errorf("Subtitle=%q, StreamTypeSubtitle=%q: must match", library.Subtitle, library.StreamTypeSubtitle)
	}
}

func TestIsCoverArtStream(t *testing.T) {
	h264 := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: "h264"}
	png := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: "png"}
	mjpeg := &library.MediaStream{StreamType: library.StreamTypeVideo, Codec: "mjpeg"}
	audio := &library.MediaStream{StreamType: library.StreamTypeAudio, Codec: "png"}

	if !library.HasRealVideoStream([]*library.MediaStream{h264, png}) {
		t.Fatal("h264 next to a png cover: expected a real video stream")
	}
	if library.HasRealVideoStream([]*library.MediaStream{mjpeg, nil}) {
		t.Fatal("a sole mjpeg stream is not a real video stream by codec")
	}
	if !library.IsCoverArtStream(png, true) {
		t.Fatal("png beside a real video stream must be cover art")
	}
	if library.IsCoverArtStream(mjpeg, false) {
		t.Fatal("a sole mjpeg stream must stay the picture")
	}
	if library.IsCoverArtStream(audio, true) || library.IsCoverArtStream(nil, true) {
		t.Fatal("only video streams can be cover art")
	}
}

// One vocabulary table for both probers: ffprobe's field order and mkvgo's
// verdict read as the same fact, anything else stays unknown (never a guess).
func TestScanTypeOf_ReadsBothProberVocabularies(t *testing.T) {
	t.Parallel()
	cases := map[string]library.ScanType{
		"tt": library.ScanInterlaced, "bb": library.ScanInterlaced, "tb": library.ScanInterlaced, "bt": library.ScanInterlaced,
		"interlaced": library.ScanInterlaced, " Interlaced ": library.ScanInterlaced,
		"progressive": library.ScanProgressive, "PROGRESSIVE": library.ScanProgressive,
		"": library.ScanUnknown, "unknown": library.ScanUnknown, "mbaff": library.ScanUnknown,
	}
	for raw, want := range cases {
		assert.Equal(t, want, library.ScanTypeOf(raw), "%q", raw)
		got := library.NormalizeFieldOrder(raw)
		if want == library.ScanUnknown {
			assert.Nil(t, got, "%q is not persisted", raw)
		} else if assert.NotNil(t, got, raw) {
			assert.Equal(t, strings.ToLower(strings.TrimSpace(raw)), *got, "the word is kept, only normalised")
		}
	}
	var nilStream *library.MediaStream
	assert.Equal(t, library.ScanUnknown, nilStream.ScanType())
	assert.Equal(t, library.ScanUnknown, (&library.MediaStream{}).ScanType())
	assert.Equal(t, library.ScanInterlaced, (&library.MediaStream{FieldOrder: new("tt")}).ScanType())
}
