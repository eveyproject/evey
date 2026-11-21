package transcoding

import (
	"strings"
	"sync/atomic"

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

// demoAudioDefault turns on the demo's audio default (SetDemoAudioDefault).
var demoAudioDefault atomic.Bool

// SetDemoAudioDefault is set once at boot on the public demo: among the tracks
// of the language the profile picks, the default becomes the one a browser
// plays without a server encode (see demoAudioRank), so a demo visitor is not
// served a software audio encode on a small shared host when the file also
// carries a track the browser plays as is. Elsewhere the source's own flags
// keep deciding.
func SetDemoAudioDefault(on bool) { demoAudioDefault.Store(on) }

// demoAudioRank orders codecs for the demo default, lowest first: the ones a
// browser decodes natively, then AC-3/E-AC-3 that the web player decodes
// itself, then everything else the server re-encodes (an MP3 included: it is
// not passed through in fragmented MP4).
func demoAudioRank(codec string) int {
	switch strings.ToLower(codec) {
	case "aac", "opus", "flac":
		return 0
	case "eac3":
		return 1
	case "ac3":
		return 2
	default:
		return 3
	}
}

// DefaultAudioTrack is the audio-relative index (ffmpeg's 0:a:N) of the track
// prefLangs picks: the first stream whose language matches, in preference
// order; else the source's default-flagged track; else 0. A commentary track
// is never the implicit pick - the core's own selection skips it whatever the
// container flags - unless every track is one: commentary beats silence. On the
// demo (SetDemoAudioDefault) the tracks of the matched language are weighed
// instead: codec rank, then channels, then the default flag, then file order.
func DefaultAudioTrack(streams []*library.MediaStream, prefLangs []string) int {
	type audioEntry struct {
		audioIdx   int
		lang       string
		isDefault  bool
		commentary bool
		rank       int
		channels   int
	}
	var audio, main []audioEntry
	for _, s := range streams {
		if s == nil || s.StreamType != library.Audio {
			continue
		}
		e := audioEntry{audioIdx: len(audio), lang: langcode.Normalize(s.Language), isDefault: s.IsDefault, commentary: s.Commentary, rank: demoAudioRank(s.Codec)}
		if s.Channels != nil {
			e.channels = *s.Channels
		}
		audio = append(audio, e)
		if !e.commentary {
			main = append(main, e)
		}
	}
	if len(audio) == 0 {
		return 0
	}
	candidates := main
	if len(candidates) == 0 {
		candidates = audio
	}
	demo := demoAudioDefault.Load()
	best := func(pool []audioEntry) int {
		pick := pool[0]
		for _, a := range pool[1:] {
			switch {
			case a.rank != pick.rank:
				if a.rank < pick.rank {
					pick = a
				}
			case a.channels != pick.channels:
				if a.channels > pick.channels {
					pick = a
				}
			case a.isDefault && !pick.isDefault:
				pick = a
			}
		}
		return pick.audioIdx
	}
	for _, pref := range prefLangs {
		if pref == "" {
			continue
		}
		var matched []audioEntry
		for _, a := range candidates {
			if a.lang == pref {
				matched = append(matched, a)
			}
		}
		if len(matched) == 0 {
			continue
		}
		if demo {
			return best(matched)
		}
		return matched[0].audioIdx
	}
	if demo {
		return best(candidates)
	}
	for _, a := range candidates {
		if a.isDefault {
			return a.audioIdx
		}
	}
	return candidates[0].audioIdx
}
