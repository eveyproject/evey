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
		"isolates":            {"\u2066Title\u2069", "Title"},
		"zero width":          {"Ti\u200Btle\uFEFF", "Title"},
		"line breaks kept":    {"First line.\nSecond\tline.", "First line.\nSecond\tline."},
		"invalid utf8":        {"Ti\xfftle", "Title"},
		"trimmed":             {"  Title \n", "Title"},
		"accents and scripts": {"Amélie 千と千尋", "Amélie 千と千尋"},
		"empty":               {"", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, metadata.SanitizeText(tc.in))
		})
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
