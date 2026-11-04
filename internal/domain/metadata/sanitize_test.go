package metadata_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/stretchr/testify/assert"
)

func TestSanitizeText(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"nul byte":            {"Title\x00", "Title"},
		"bell and escape":     {"Ti\x07tle\x1b", "Title"},
		"c1 control":          {"Ti\u0085tle", "Title"},
		"rtl override":        {"\u202Eeltit", "eltit"},
		"zero width":          {"Ti\u200Btle\uFEFF", "Title"},
		"line breaks kept":    {"First line.\nSecond\tline.", "First line.\nSecond\tline."},
		"invalid utf8":        {"Ti\xfftle", "Title"},
		"trimmed":             {"  Title \n", "Title"},
		"accents and scripts": {"Am\u00E9lie \u5343\u3068\u5343\u5C0B", "Am\u00E9lie \u5343\u3068\u5343\u5C0B"},
		"empty":               {"", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, metadata.SanitizeText(tc.in))
		})
	}
}

// Text a title is written with comes out as it went in: the joiners of a
// Persian word, an Indic conjunct and an emoji family, an isolate around an
// Arabic word, an RLM after a Hebrew one. The previous rule stripped them.
func TestSanitizeText_KeepsLegitimateFormatCharacters(t *testing.T) {
	for name, in := range map[string]string{
		"persian zwnj":   "\u0645\u06cc\u200c\u062e\u0648\u0627\u0647\u0645",
		"hindi zwj":      "\u0915\u094d\u200d\u0937",
		"emoji family":   "\U0001F468\u200d\U0001F469\u200d\U0001F467",
		"arabic isolate": "\u2067\u0641\u064a\u0644\u0645\u2069 - Title",
		"hebrew rlm":     "\u05e9\u05dc\u05d5\u05dd\u200f 2",
		"lrm":            "Title\u200e",
	} {
		assert.Equal(t, in, metadata.SanitizeText(in), name)
	}
}

func TestMetadataResult_Sanitize(t *testing.T) {
	r := &metadata.MetadataResult{
		Title: "Ti\x00tle", OriginalTitle: "\u202EOrig", Overview: "Over\x07view", Tagline: " Tag ",
		Genres: []string{"Dra\x00ma"}, Studios: []string{"\u200BStudio"}, AlternativeTitles: []string{"Alt\x1f"},
		People:     []metadata.PersonResult{{Name: "Na\x00me", Role: "Ac\x07tor", Character: "\u202EChar", ProviderID: "42"}},
		Collection: &metadata.CollectionRef{Name: "Coll\x00ection", TmdbID: 7},
	}
	r.Sanitize()
	assert.Equal(t, "Title", r.Title)
	assert.Equal(t, "Orig", r.OriginalTitle)
	assert.Equal(t, "Overview", r.Overview)
	assert.Equal(t, "Tag", r.Tagline)
	assert.Equal(t, []string{"Drama"}, r.Genres)
	assert.Equal(t, []string{"Studio"}, r.Studios)
	assert.Equal(t, []string{"Alt"}, r.AlternativeTitles)
	assert.Equal(t, metadata.PersonResult{Name: "Name", Role: "Actor", Character: "Char", ProviderID: "42"}, r.People[0])
	assert.Equal(t, "Collection", r.Collection.Name)
	var nilResult *metadata.MetadataResult
	nilResult.Sanitize()
}
