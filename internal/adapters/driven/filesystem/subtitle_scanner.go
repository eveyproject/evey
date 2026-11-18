package filesystem

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
)

// subtitleExtensions maps a sidecar's extension to the codec its stream is
// stored under. The picture formats take the prober's codec names, which is
// what tells every player to fetch them as decoded images rather than text.
var subtitleExtensions = map[string]string{
	".srt": "srt",
	".ass": "ass",
	".ssa": "ssa",
	".vtt": "vtt",
	".sup": "hdmv_pgs_subtitle",
	// A VobSub is an .idx/.sub pair, listed once under its .idx. A .sub alone
	// is not taken: the extension is also MicroDVD's text format.
	".idx": "dvd_subtitle",
}

type SubtitleScanner struct{}

func NewSubtitleScanner() *SubtitleScanner {
	return &SubtitleScanner{}
}

// ScanForSubtitles finds sidecar subtitle files matching the video filename.
// Patterns: video.srt, video.en.srt, video.forced.srt, video.en.forced.srt
func (s *SubtitleScanner) ScanForSubtitles(videoPath string) ([]library.ExternalSubtitle, error) {
	dir := filepath.Dir(videoPath)
	videoBase := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	prefix := strings.ToLower(videoBase)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var subs []library.ExternalSubtitle
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		format, ok := subtitleExtensions[ext]
		if !ok {
			continue
		}

		base := strings.TrimSuffix(name, filepath.Ext(name))
		if !strings.EqualFold(base, videoBase) && !strings.HasPrefix(strings.ToLower(base), prefix+".") {
			continue
		}
		if ext == ".idx" && !hasSibling(entries, base, ".sub") {
			continue
		}

		sub := library.ExternalSubtitle{
			Path:   filepath.Join(dir, name),
			Format: format,
		}

		// Parse language and forced flag from middle segments
		// e.g. "movie.en.forced.srt" → segments = ["en", "forced"]
		remainder := strings.TrimPrefix(strings.ToLower(base), prefix)
		remainder = strings.TrimPrefix(remainder, ".")
		if remainder != "" {
			parts := strings.SplitSeq(remainder, ".")
			for p := range parts {
				if p == "forced" || p == "sdh" || p == "cc" {
					if p == "forced" {
						sub.IsForced = true
					}
					continue
				}
				if sub.Language == "" {
					// Subtitle files use bare 2-letter ISO codes legitimately
					// (movie.fr.srt, movie.en.srt), so we accept both strong
					// and weak tags here - the filename.ext positional parsing
					// rules out the title-truncation collision seen in video
					// filenames.
					if iso, ok := strongLanguageTags[p]; ok {
						sub.Language = iso
					} else if iso, ok := weakLanguageTags[p]; ok {
						sub.Language = iso
					} else if (len(p) == 2 || len(p) == 3) && isAlpha(p) {
						// An unrecognised 2-3 LETTER code is taken as a language
						// (covers ISO codes we don't enumerate). Guard on alpha so a
						// numeric segment like "movie.99.srt" isn't stored as "99".
						sub.Language = p
					}
				}
			}
		}

		subs = append(subs, sub)
	}
	return subs, nil
}

// isAlpha reports whether s is non-empty and all ASCII letters (used to keep a
// numeric filename segment from being mistaken for a language code).
// hasSibling reports whether the directory holds base+ext, whatever the case
// of the extension.
func hasSibling(entries []os.DirEntry, base, ext string) bool {
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && strings.EqualFold(filepath.Ext(n), ext) && strings.TrimSuffix(n, filepath.Ext(n)) == base {
			return true
		}
	}
	return false
}

func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
