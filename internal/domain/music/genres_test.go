package music

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitGenres(t *testing.T) {
	cases := map[string][]string{
		"":                           {},
		"Rock":                       {"Rock"},
		"Alternative/Pop/Rock/Metal": {"Alternative", "Pop", "Rock", "Metal"},
		"Rap/Hip Hop/Pop":            {"Rap", "Hip Hop", "Pop"},
		" Rock ; Blues ,Jazz|Soul ":  {"Rock", "Blues", "Jazz", "Soul"},
		"Rock/rock/ROCK":             {"Rock"},
		"//;;":                       {},
	}
	for raw, want := range cases {
		assert.Equal(t, want, SplitGenres(raw), raw)
	}
}
