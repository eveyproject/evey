package postgres

import (
	"testing"
	"unicode/utf8"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression for the prod scan loop: a Latin-1 filename byte (a lone 0x80)
// parsed into the title made every media_items insert fail with
// "invalid byte sequence for encoding UTF8", so the file never indexed and the
// log spammed every scan. sanitizeMediaItemText must strip invalid bytes from
// all text fields while leaving valid UTF-8 (including multi-byte "À") intact.
func TestSanitizeMediaItemText_StripsInvalidUTF8(t *testing.T) {
	bad := "À la poursuite\x80 de demain" // valid "À" (0xC3 0x80) + a stray lone 0x80
	require.False(t, utf8.ValidString(bad), "fixture must be invalid UTF-8")

	item := &library.MediaItem{
		Title:         bad,
		ScanTitle:     bad,
		SortTitle:     bad,
		OriginalTitle: bad,
		Overview:      bad,
		Tagline:       bad,
		File:          &library.MediaFile{Path: "/media/" + bad + ".mkv", ContainerFormat: "mkv\x80", Fingerprint: "fp\x80"},
		Catalog:       &library.MediaCatalog{Genres: []string{"Action\x80"}, Studios: []string{"Studio\x80"}, Tags: []string{"tag\x80"}},
	}

	sanitizeMediaItemText(item)

	for name, s := range map[string]string{
		"Title": item.Title, "ScanTitle": item.ScanTitle, "SortTitle": item.SortTitle,
		"OriginalTitle": item.OriginalTitle, "Overview": item.Overview, "Tagline": item.Tagline,
		"File.Path": item.File.Path, "File.ContainerFormat": item.File.ContainerFormat, "File.Fingerprint": item.File.Fingerprint,
		"Genre": item.Catalog.Genres[0], "Studio": item.Catalog.Studios[0], "Tag": item.Catalog.Tags[0],
	} {
		assert.True(t, utf8.ValidString(s), "%s must be valid UTF-8 after sanitize: %q", name, s)
	}
	// The valid multi-byte "À" survives; only the stray byte is dropped.
	assert.Equal(t, "À la poursuite de demain", item.Title)
}

func TestSanitizeMediaItemText_LeavesValidUTF8Unchanged(t *testing.T) {
	good := "À la poursuite de demain"
	item := &library.MediaItem{Title: good, File: &library.MediaFile{Path: "/media/" + good + ".mkv"}}
	sanitizeMediaItemText(item)
	assert.Equal(t, good, item.Title)
	assert.Equal(t, "/media/"+good+".mkv", item.File.Path)
}

// Nil File/Catalog must not panic (movies without enriched metadata at scan).
func TestSanitizeMediaItemText_NilSubobjects(t *testing.T) {
	item := &library.MediaItem{Title: "ok"}
	assert.NotPanics(t, func() { sanitizeMediaItemText(item) })
}
