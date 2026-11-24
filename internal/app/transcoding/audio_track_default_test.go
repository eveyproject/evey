package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/library"
)

// A commentary track is never the implicit pick, whatever flags it carries:
// the core's own selection (selectedAudioSpec) skips it, and the surfaces
// that name the served track must pick the same one. Only a file whose every
// track is commentary falls back on one - commentary beats silence.
func TestDefaultAudioTrack_SkipsACommentaryTrack(t *testing.T) {
	audio := func(codec, lang string, def, commentary bool) *library.MediaStream {
		return &library.MediaStream{StreamType: library.Audio, Codec: codec, Language: lang, IsDefault: def, Commentary: commentary}
	}
	streams := []*library.MediaStream{
		audio("aac", "eng", true, true),
		audio("ac3", "eng", false, false),
		audio("ac3", "fre", false, false),
	}
	assert.Equal(t, 1, DefaultAudioTrack(streams, PreferredAudioLanguages(nil, "en", "en")), "the English main track, not the English commentary default")
	assert.Equal(t, 1, DefaultAudioTrack(streams, nil), "no language: the first non-commentary track, not the commentary default")
	assert.Equal(t, 2, DefaultAudioTrack(streams, []string{"fr"}))
	assert.Equal(t, 1, DefaultAudioTrack(streams, []string{"de"}), "no match: still never the commentary")

	// The core picks the same track implicitly.
	spec := selectedAudioSpec(toStreamSpecs(streams), TranscodingPreferences{AudioTrack: -1})
	assert.False(t, spec.Commentary)
	assert.Equal(t, "ac3", spec.Codec)

	// A French commentary is not the French pick when a French main exists.
	frCommentaryFirst := []*library.MediaStream{audio("ac3", "fre", false, true), audio("aac", "fre", true, false)}
	assert.Equal(t, 1, DefaultAudioTrack(frCommentaryFirst, []string{"fr"}))

	// Every track commentary: the former order (language, default, first).
	allCommentary := []*library.MediaStream{audio("aac", "eng", false, true), audio("ac3", "fre", true, true)}
	assert.Equal(t, 1, DefaultAudioTrack(allCommentary, nil), "the default flag")
	assert.Equal(t, 0, DefaultAudioTrack(allCommentary, []string{"en"}), "the language")
}

// On the public demo the default among the tracks of the picked language is
// the one a browser plays without a server encode: Tears of Steel's MP3
// (first, default-flagged) loses to its E-AC-3 5.1, and an AAC still beats
// any Dolby track. Off the demo the file's own order and flags decide.
func TestDefaultAudioTrack_DemoPrefersATrackPlayedWithoutAnEncode(t *testing.T) {
	ch := func(n int) *int { return &n }
	audio := func(codec, lang string, def bool, channels int) *library.MediaStream {
		return &library.MediaStream{StreamType: library.Audio, Codec: codec, Language: lang, IsDefault: def, Channels: ch(channels)}
	}
	tearsOfSteel := []*library.MediaStream{audio("mp3", "eng", true, 2), audio("ac3", "eng", false, 6), audio("eac3", "eng", false, 6)}
	bigBuckBunny := []*library.MediaStream{audio("mp3", "eng", true, 2), audio("ac3", "eng", true, 6)}
	aacAndDolby := []*library.MediaStream{audio("eac3", "eng", true, 6), audio("aac", "eng", false, 2)}
	otherLanguage := []*library.MediaStream{audio("mp3", "fre", true, 2), audio("eac3", "eng", false, 6)}
	en := PreferredAudioLanguages(nil, "en", "en")

	assert.Equal(t, 0, DefaultAudioTrack(tearsOfSteel, en), "off the demo: the first English track")
	assert.Equal(t, 0, DefaultAudioTrack(bigBuckBunny, en))

	SetDemoAudioDefault(true)
	t.Cleanup(func() { SetDemoAudioDefault(false) })
	assert.Equal(t, 2, DefaultAudioTrack(tearsOfSteel, en), "the E-AC-3 5.1, not the MP3 the server would encode")
	assert.Equal(t, 1, DefaultAudioTrack(bigBuckBunny, en), "the AC-3 over the MP3")
	assert.Equal(t, 1, DefaultAudioTrack(aacAndDolby, en), "an AAC plays natively, no WASM decode")
	assert.Equal(t, 0, DefaultAudioTrack(otherLanguage, []string{"fr"}), "the language still comes first")
	assert.Equal(t, 1, DefaultAudioTrack(otherLanguage, []string{"de"}), "no language match: ranked over every track")
}

// Matroska's test5 as the demo served it (measured): an unlabelled main
// stereo track, then an English mono track titled "Commentary" without the
// container's commentary flag. An English profile was given the commentary.
func TestDefaultAudioTrack_ATitledCommentaryIsNeverTheImplicitPick(t *testing.T) {
	two, one := 2, 1
	streams := []*library.MediaStream{
		{StreamType: library.Video, Codec: "h264"},
		{StreamType: library.Audio, Codec: "aac", Language: "und", Channels: &two, IsDefault: true},
		{StreamType: library.Audio, Codec: "aac", Language: "eng", Channels: &one, Title: "Commentary"},
	}
	for _, demo := range []bool{false, true} {
		SetDemoAudioDefault(demo)
		if got := DefaultAudioTrack(streams, []string{"en"}); got != 0 {
			t.Errorf("demo=%v: default = audio %d, want the main track 0", demo, got)
		}
	}
	SetDemoAudioDefault(false)
}
