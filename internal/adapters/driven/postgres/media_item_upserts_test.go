package postgres

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// replaceMediaTrailers used to issue one INSERT per trailer - N round trips
// per scanned item. buildTrailerInsert must fold them into a single
// multi-row VALUES statement instead.
func TestBuildTrailerInsert_MultiRow(t *testing.T) {
	trailers := []library.Trailer{
		{Site: "YouTube", Key: "abc", Language: "en", Official: true, Type: "Trailer", UseSubtitles: false, SubtitleLanguage: ""},
		{Site: "YouTube", Key: "def", Language: "fr", Official: false, Type: "Teaser", UseSubtitles: true, SubtitleLanguage: "fr"},
	}

	query, args := buildTrailerInsert("mi1", trailers)

	require.NotEmpty(t, query, "must build a single query for a non-empty trailer list")
	assert.Contains(t, query, "VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10),($11,$12,$13,$14,$15,$16,$17,$18,$19,$20)",
		"must be one multi-row VALUES clause, not one query per trailer")

	require.Len(t, args, 20, "10 columns x 2 trailers")
	// Stable id format preserved - callers (and any external reference) depend on it.
	assert.Equal(t, "tlr-mi1-0", args[0])
	assert.Equal(t, "mi1", args[1])
	assert.Equal(t, 0, args[2])
	assert.Equal(t, "YouTube", args[3])
	assert.Equal(t, "abc", args[4])

	assert.Equal(t, "tlr-mi1-1", args[10])
	assert.Equal(t, 1, args[12])
	assert.Equal(t, "def", args[14])
}

// An empty trailer list must not produce an INSERT ... VALUES with zero
// rows (invalid SQL) - the caller must skip the exec entirely.
func TestBuildTrailerInsert_EmptyYieldsNoQuery(t *testing.T) {
	query, args := buildTrailerInsert("mi1", nil)
	assert.Empty(t, query)
	assert.Nil(t, args)
}
