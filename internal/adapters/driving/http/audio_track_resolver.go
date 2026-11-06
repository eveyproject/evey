package httpapi

import (
	"github.com/eveyproject/evey/internal/pkg/langcode"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
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

// buildPrefLangs is the ordered language preference list for audio track
// resolution, shared with every surface that names the served track.
func buildPrefLangs(audioLanguages []string, locale, serverLang string) []string {
	return apptranscoding.PreferredAudioLanguages(audioLanguages, locale, serverLang)
}

// resolveDefaultAudioTrack is the audio-relative index of the best audio
// track for the given preference list (language, then default flag, then 0).
func resolveDefaultAudioTrack(streams []*domainlibrary.MediaStream, prefLangs []string) int {
	return apptranscoding.DefaultAudioTrack(streams, prefLangs)
}
