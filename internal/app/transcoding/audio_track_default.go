package transcoding

import (
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/langcode"
)

// PreferredAudioLanguages is the ordered language list a default audio track
// is picked by: the profile's own list, then its UI locale, then the server's
// metadata language. One list for every surface that names the served track
// (HLS master, playback info, stream info), so they all pick the same one.
// Duplicates keep their first position, empties are dropped, every code is
// folded to ISO 639-1.
func PreferredAudioLanguages(audioLanguages []string, locale, serverLang string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, raw := range append(append([]string{}, audioLanguages...), locale, serverLang) {
		n := langcode.Normalize(raw)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// DefaultAudioTrack is the audio-relative index (ffmpeg's 0:a:N) of the track
// prefLangs picks: the first stream whose language matches, in preference
// order; else the source's default-flagged track; else 0.
func DefaultAudioTrack(streams []*library.MediaStream, prefLangs []string) int {
	type audioEntry struct {
		audioIdx  int
		lang      string
		isDefault bool
	}
	var audio []audioEntry
	for _, s := range streams {
		if s == nil || s.StreamType != library.Audio {
			continue
		}
		audio = append(audio, audioEntry{audioIdx: len(audio), lang: langcode.Normalize(s.Language), isDefault: s.IsDefault})
	}
	if len(audio) == 0 {
		return 0
	}
	for _, pref := range prefLangs {
		if pref == "" {
			continue
		}
		for _, a := range audio {
			if a.lang == pref {
				return a.audioIdx
			}
		}
	}
	for _, a := range audio {
		if a.isDefault {
			return a.audioIdx
		}
	}
	return 0
}
