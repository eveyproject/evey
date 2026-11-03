package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
)

// --- normalizeLanguageCode ---

func TestNormalizeLanguageCode_TwoLetterPassThrough(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fr", normalizeLanguageCode("fr"))
	assert.Equal(t, "en", normalizeLanguageCode("en"))
	assert.Equal(t, "ja", normalizeLanguageCode("ja"))
}

func TestNormalizeLanguageCode_ISO6392B(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fr", normalizeLanguageCode("fre"))
	assert.Equal(t, "en", normalizeLanguageCode("eng"))
	assert.Equal(t, "de", normalizeLanguageCode("ger"))
	assert.Equal(t, "es", normalizeLanguageCode("spa"))
	assert.Equal(t, "it", normalizeLanguageCode("ita"))
	assert.Equal(t, "ja", normalizeLanguageCode("jpn"))
	assert.Equal(t, "zh", normalizeLanguageCode("chi"))
	assert.Equal(t, "zh", normalizeLanguageCode("zho"))
}

func TestNormalizeLanguageCode_ISO6392T(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fr", normalizeLanguageCode("fra"))
	assert.Equal(t, "de", normalizeLanguageCode("deu"))
	assert.Equal(t, "nl", normalizeLanguageCode("nld"))
}

// BCP-47 / ISO 639-3 macrolanguage codes (what modern MKVs carry in
// LanguageBCP47, surfaced by the in-process mkvgo prober) must fold like the
// legacy 639-2 "chi" ffprobe reports - else a "zh" audio preference would match
// an ffprobe-scanned file but not an mkvgo-scanned one.
func TestNormalizeLanguageCode_ChineseMacrolang(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "zh", normalizeLanguageCode("cmn")) // Mandarin
	assert.Equal(t, "zh", normalizeLanguageCode("yue")) // Cantonese
	assert.Equal(t, "zh", normalizeLanguageCode("chi")) // legacy 639-2 (ffprobe)
}

func TestNormalizeLanguageCode_FullName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fr", normalizeLanguageCode("French"))
	assert.Equal(t, "en", normalizeLanguageCode("English"))
	assert.Equal(t, "de", normalizeLanguageCode("German"))
}

func TestNormalizeLanguageCode_Unknown_PassThrough(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "tlh", normalizeLanguageCode("tlh")) // Klingon :)
	assert.Equal(t, "xx", normalizeLanguageCode("xx"))
}

func TestNormalizeLanguageCode_CaseInsensitive(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "fr", normalizeLanguageCode("FRE"))
	assert.Equal(t, "en", normalizeLanguageCode("ENG"))
	assert.Equal(t, "fr", normalizeLanguageCode("FR"))
}

// --- buildPrefLangs ---

func TestBuildPrefLangs_AudioLanguagesFirst(t *testing.T) {
	t.Parallel()
	got := buildPrefLangs([]string{"fr", "en"}, "de", "en")
	assert.Equal(t, []string{"fr", "en", "de"}, got)
}

func TestBuildPrefLangs_DeduplicatesLocale(t *testing.T) {
	t.Parallel()
	// Profile locale matches a profile AudioLanguage - deduplicated.
	got := buildPrefLangs([]string{"fr"}, "fr", "en")
	assert.Equal(t, []string{"fr", "en"}, got)
}

func TestBuildPrefLangs_DeduplicatesServerLang(t *testing.T) {
	t.Parallel()
	// Server lang already covered by audio languages.
	got := buildPrefLangs([]string{"en", "fr"}, "es", "en")
	assert.Equal(t, []string{"en", "fr", "es"}, got)
}

func TestBuildPrefLangs_EmptyInputs(t *testing.T) {
	t.Parallel()
	got := buildPrefLangs(nil, "", "en")
	assert.Equal(t, []string{"en"}, got)
}

func TestBuildPrefLangs_AllEmpty(t *testing.T) {
	t.Parallel()
	got := buildPrefLangs(nil, "", "")
	assert.Empty(t, got)
}

func TestBuildPrefLangs_NormalizesInputCodes(t *testing.T) {
	t.Parallel()
	// "fra" normalises to "fr", duplicate of "fr" in list.
	got := buildPrefLangs([]string{"fra", "eng"}, "fr", "en")
	assert.Equal(t, []string{"fr", "en"}, got)
}

// --- resolveDefaultAudioTrack ---

func audioStream(lang string, isDefault bool) *domainlibrary.MediaStream {
	return &domainlibrary.MediaStream{
		StreamType: domainlibrary.Audio,
		Language:   lang,
		IsDefault:  isDefault,
	}
}

func TestResolveDefaultAudioTrack_MatchesFirstPref(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("eng", true),  // audio-index 0
		audioStream("fre", false), // audio-index 1
	}
	// Prefer "fr" → should pick audio-index 1 (fre normalises to fr).
	got := resolveDefaultAudioTrack(streams, []string{"fr", "en"})
	assert.Equal(t, 1, got)
}

func TestResolveDefaultAudioTrack_FallsBackToSecondPref(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("eng", true),
		audioStream("ger", false),
	}
	// Prefer fr (not present), then en → picks audio-index 0.
	got := resolveDefaultAudioTrack(streams, []string{"fr", "en"})
	assert.Equal(t, 0, got)
}

func TestResolveDefaultAudioTrack_IsDefaultFallback(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("ger", false), // audio-index 0
		audioStream("jpn", true),  // audio-index 1
	}
	// No preference match → fall back to IsDefault.
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 1, got)
}

func TestResolveDefaultAudioTrack_ZeroFallback(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("ger", false),
		audioStream("jpn", false),
	}
	// No match, no IsDefault → audio-index 0.
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 0, got)
}

func TestResolveDefaultAudioTrack_EmptyPrefLangs_UsesIsDefault(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("eng", false),
		audioStream("fre", true),
	}
	got := resolveDefaultAudioTrack(streams, nil)
	assert.Equal(t, 1, got)
}

func TestResolveDefaultAudioTrack_NoAudioStreams(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, Language: "eng"},
	}
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 0, got)
}

func TestResolveDefaultAudioTrack_NilStreams(t *testing.T) {
	t.Parallel()
	got := resolveDefaultAudioTrack(nil, []string{"fr"})
	assert.Equal(t, 0, got)
}

func TestResolveDefaultAudioTrack_SkipsNilEntries(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		nil,
		audioStream("fre", false), // audio-index 0 (nil skipped, first real audio)
		audioStream("eng", true),  // audio-index 1
	}
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 0, got)
}

func TestResolveDefaultAudioTrack_VideoStreamIgnored(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		{StreamType: domainlibrary.Video, Language: "fre"}, // NOT counted in audio-index
		audioStream("eng", true),                           // audio-index 0
		audioStream("fre", false),                          // audio-index 1
	}
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 1, got)
}

func TestResolveDefaultAudioTrack_OrderRespected(t *testing.T) {
	t.Parallel()
	streams := []*domainlibrary.MediaStream{
		audioStream("eng", false), // audio-index 0
		audioStream("fre", false), // audio-index 1
		audioStream("deu", false), // audio-index 2
	}
	// Prefer de, then fr → de is index 2.
	got := resolveDefaultAudioTrack(streams, []string{"de", "fr"})
	assert.Equal(t, 2, got)
}

func TestResolveDefaultAudioTrack_3LetterCodeMatches(t *testing.T) {
	t.Parallel()
	// Stream has "fra" (ISO 639-2), prefLang is "fr" (ISO 639-1).
	streams := []*domainlibrary.MediaStream{
		audioStream("eng", true),
		audioStream("fra", false),
	}
	got := resolveDefaultAudioTrack(streams, []string{"fr"})
	assert.Equal(t, 1, got)
}

// The master and playback-info pick the same track: the commentary default
// skipped, the explicit choice honoured, an out-of-range choice falling back
// on the language default.
func TestMasterAndInfoPickTheSameTrack(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		{StreamIndex: 0, StreamType: "video", Codec: "h264"},
		{StreamIndex: 1, StreamType: "audio", Codec: "aac", Language: "eng", IsDefault: true, Commentary: true},
		{StreamIndex: 2, StreamType: "audio", Codec: "ac3", Language: "eng"},
		{StreamIndex: 3, StreamType: "audio", Codec: "dts", Language: "fre"},
	}
	master := &hlsVODServe{serverLang: func() string { return "en" }}
	for _, c := range []struct {
		langs []string
		query string
		want  int
	}{
		{nil, "", 1}, // the English main, not the commentary default
		{[]string{"fr"}, "", 2},
		{[]string{"fr"}, "?audioTrack=0", 0}, // an explicit choice may be the commentary
		{[]string{"fr"}, "?audioTrack=9", 2}, // out of range: the language default
		{[]string{"de"}, "", 1},              // no match: locale/server en, never the commentary
	} {
		prefs := apptranscoding.TranscodingPreferences{AudioLanguages: c.langs}
		r := httptest.NewRequest(http.MethodGet, "/m"+c.query, nil)
		m := master.resolveAudioTrackWith(r, streams, prefs)
		info, explicit := explicitAudioTrack(r, streams)
		if !explicit {
			info = resolveDefaultAudioTrack(streams, buildPrefLangs(prefs.AudioLanguages, prefs.Locale, "en"))
		}
		assert.Equal(t, c.want, m, "master langs=%v q=%s", c.langs, c.query)
		assert.Equal(t, m, info, "info langs=%v q=%s", c.langs, c.query)
	}
}
