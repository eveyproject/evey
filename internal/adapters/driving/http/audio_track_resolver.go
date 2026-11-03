package httpapi

import (
	"github.com/eveyproject/evey/internal/pkg/langcode"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

// normalizeLanguageCode folds the many spellings a probe emits (ISO 639-2/B,
// 639-2/T, BCP-47 macrolanguage tags, English names) to ISO 639-1. The table
// lives in pkg/langcode so the prober's real-file parity gate uses the exact
// same normalization (previously a second, incomplete copy drifted - it missed
// the 639-2/B codes whose 2-letter form isn't a prefix, e.g. dut→nl, tur→tr).
func normalizeLanguageCode(lang string) string {
	return langcode.Normalize(lang)
}

// buildPrefLangs assembles the ordered language preference list for audio track
// resolution. The order of precedence is:
//
//  1. Profile.AudioLanguages (explicit user override, ordered list)
//  2. Profile.Locale (language of the UI the user chose)
//  3. Server's metadata language (cfg.Metadata.Language)
//
// Duplicates are removed (first occurrence wins) and empty strings are dropped.
// All codes are normalised via normalizeLanguageCode so callers don't need to
// worry about ISO 639-1 vs 639-2 mismatches.
func buildPrefLangs(audioLanguages []string, locale, serverLang string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, raw := range append(audioLanguages, locale, serverLang) {
		n := normalizeLanguageCode(raw)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// resolveDefaultAudioTrack returns the audio-index (0-based among audio streams
// only, matching `ffmpeg -map 0:a:N`) of the best audio track for the given
// preference list.
//
// Resolution order:
//  1. First audio stream whose normalised Language matches a prefLang, tried in
//     prefLangs order (first preference wins).
//  2. First audio stream with IsDefault == true.
//  3. Audio index 0 (hard fallback - always valid).
//
// An empty prefLangs list skips step 1 and falls through to IsDefault / 0.
func resolveDefaultAudioTrack(streams []*domainlibrary.MediaStream, prefLangs []string) int {
	// Build the audio-only slice (preserves audio-index == position in this slice).
	type audioEntry struct {
		audioIdx  int
		lang      string
		isDefault bool
	}
	var audioStreams []audioEntry
	for _, s := range streams {
		if s == nil || s.StreamType != domainlibrary.Audio {
			continue
		}
		audioStreams = append(audioStreams, audioEntry{
			audioIdx:  len(audioStreams),
			lang:      normalizeLanguageCode(s.Language),
			isDefault: s.IsDefault,
		})
	}
	if len(audioStreams) == 0 {
		return 0
	}

	// Step 1: match by preferred language, in preference order.
	for _, pref := range prefLangs {
		if pref == "" {
			continue
		}
		for _, a := range audioStreams {
			if a.lang == pref {
				return a.audioIdx
			}
		}
	}

	// Step 2: source-flagged default.
	for _, a := range audioStreams {
		if a.isDefault {
			return a.audioIdx
		}
	}

	// Step 3: first audio track.
	return 0
}
