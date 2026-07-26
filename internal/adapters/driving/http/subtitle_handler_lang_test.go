package httpapi

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The DB stores ffprobe's BCP-47-ish tag ("fr-FR"); Matroska language elements
// are usually bare ISO 639-2 codes ("fre"). Raw equality between the two
// rejected legitimate tracks on EVERY playback of such a file (prod 2026-07-26:
// "subtitle: mkvgo language mismatch vs DB, db_lang=fr-FR mkvgo_lang=fre" -
// permanent silent fallback to the ffmpeg fork). The guard's real job is to
// catch ORDINAL DRIFT (DB row pointing at a different track after a re-mux),
// so region subtags still disqualify when BOTH sides carry one.
func TestSubtitleLanguageMatches(t *testing.T) {
	cases := []struct {
		db, mkv string
		want    bool
	}{
		// The prod bug class: same language, different spelling conventions.
		{"fr-FR", "fre", true},
		{"fr-FR", "fra", true},
		{"fr", "fre", true},
		{"en", "eng", true},
		{"de-DE", "ger", true},
		{"nl", "dut", true}, // 639-2/B whose 2-letter form is not a prefix

		// Genuinely different languages: still a mismatch.
		{"fr-FR", "eng", false},
		{"fr", "de", false},

		// Both sides region-qualified and disagreeing = possible ordinal drift
		// between two variants of the same language (fr-FR vs fr-CA tracks in
		// the same file) - keep rejecting.
		{"fr-FR", "fr-CA", false},
		{"fr-FR", "fr-FR", true},

		// One side region-less: it cannot disprove the other's region.
		{"fr-CA", "fre", true},
		{"pt-BR", "por", true},

		// Unknown on either side never rejects (existing guard semantics).
		{"", "fre", true},
		{"fr-FR", "", true},

		// Exotic codes pass through Normalize unchanged - exact match still works.
		{"qaa", "qaa", true},
		{"qaa", "qab", false},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s_vs_%s", c.db, c.mkv), func(t *testing.T) {
			assert.Equal(t, c.want, subtitleLanguageMatches(c.db, c.mkv),
				"db=%q mkvgo=%q", c.db, c.mkv)
		})
	}
}
