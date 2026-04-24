package library_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
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
