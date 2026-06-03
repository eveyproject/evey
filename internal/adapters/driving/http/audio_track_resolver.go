package httpapi

import (
	"strings"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// normalizeLanguageCode maps common ISO 639-2/B (bibliographic), ISO 639-2/T
// (terminological), and full English names to their ISO 639-1 two-letter form.
// Unknown codes are passed through as-is (lower-cased) so that an exact match
// on an exotic tag still works.
func normalizeLanguageCode(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch lang {
	// French
	case "fre", "fra", "french":
		return "fr"
	// English
	case "eng", "english":
		return "en"
	// Spanish
	case "spa", "spanish":
		return "es"
	// German
	case "ger", "deu", "german":
		return "de"
	// Italian
	case "ita", "italian":
		return "it"
	// Japanese
	case "jpn", "japanese":
		return "ja"
	// Chinese (Mandarin / generic). Includes the ISO 639-3 / BCP-47 macrolanguage
	// codes (cmn = Mandarin, yue = Cantonese, …) that modern MKVs carry in the
	// LanguageBCP47 element — the in-process mkvgo prober surfaces those, whereas
	// ffprobe reports the legacy 639-2 "chi"; both must fold to "zh".
	case "chi", "zho", "chinese", "cmn", "yue", "wuu", "hak", "nan":
		return "zh"
	// Portuguese
	case "por", "portuguese":
		return "pt"
	// Korean
	case "kor", "korean":
		return "ko"
	// Russian
	case "rus", "russian":
		return "ru"
	// Arabic
	case "ara", "arabic":
		return "ar"
	// Dutch
	case "dut", "nld", "dutch":
		return "nl"
	// Polish
	case "pol", "polish":
		return "pl"
	// Swedish
	case "swe", "swedish":
		return "sv"
	// Turkish
	case "tur", "turkish":
		return "tr"
	}
	// Pass-through: two-letter tags are already ISO 639-1; exotic 3-letter tags
	// are kept so an exact match on the stream's Language field still resolves.
	return lang
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
//  3. Audio index 0 (hard fallback — always valid).
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
