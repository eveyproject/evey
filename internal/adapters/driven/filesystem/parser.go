package filesystem

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// languageTags maps filename tokens to (ISO 639-1 code, original tag).
// Based on the normalize_video project conventions.
var languageTags = map[string]string{
	// French
	"vf": "fr", "vff": "fr", "vfi": "fr", "vfq": "fr",
	"french": "fr", "truefrench": "fr", "vostfr": "fr", "subfrench": "fr",
	"fr": "fr", "fre": "fr", "fra": "fr",
	// English
	"vo": "en", "voen": "en", "eng": "en", "english": "en",
	"en": "en",
	// German
	"vostde": "de", "vode": "de", "german": "de", "deutsch": "de",
	"de": "de", "deu": "de", "ger": "de",
	// Spanish
	"vostes": "es", "voes": "es", "spanish": "es", "espanol": "es",
	"es": "es", "spa": "es",
	// Italian
	"vostit": "it", "voit": "it", "italian": "it", "italiano": "it",
	"it": "it", "ita": "it",
	// Japanese
	"vostjp": "ja", "vojp": "ja", "japanese": "ja", "jpn": "ja",
	"ja": "ja",
	// Portuguese
	"vostpt": "pt", "vopt": "pt", "portuguese": "pt",
	"pt": "pt", "por": "pt",
	// Russian
	"vostru": "ru", "voru": "ru", "russian": "ru",
	"ru": "ru", "rus": "ru",
	// Chinese
	"vostzh": "zh", "vozh": "zh", "chinese": "zh",
	"zh": "zh", "chi": "zh",
	// Arabic
	"vostar": "ar", "voar": "ar", "arabic": "ar",
	"ar": "ar", "ara": "ar",
	// Hindi
	"hindi": "hi", "hi": "hi", "hin": "hi",
	// Korean
	"korean": "ko", "ko": "ko", "kor": "ko",
	// Multi
	"multi": "multi",
}

// knownQualities for part-based detection (lowercase).
var knownQualities = map[string]bool{
	"2160p": true, "4k": true, "uhd": true, "8k": true,
	"1080p": true, "720p": true, "480p": true, "360p": true,
	"bluray": true, "bdrip": true, "brrip": true,
	"remux": true,
	"webrip": true, "webdl": true, "web": true,
	"hdtv": true,
	"dvdrip": true, "dvdscr": true, "dvd": true,
	"cam": true, "hdcam": true, "ts": true, "telesync": true, "screener": true,
}

var (
	reSeriesPart = regexp.MustCompile(`(?i)^[sS](\d{1,2})[eE](\d{1,3})$`)
	reSeasonX    = regexp.MustCompile(`(?i)^(\d{1,2})[xX](\d{1,3})$`)
	reYear       = regexp.MustCompile(`^((?:19|20)\d{2})$`)
	reCodec      = regexp.MustCompile(`(?i)\b(x264|x265|h\.?264|h\.?265|hevc|avc|av1|vp9|mpeg[24]|xvid|divx)\b`)
	reAudio      = regexp.MustCompile(`(?i)\b(aac|dts-hd\.?ma|dts-hd|dts|truehd|atmos|flac|ac3|eac3|dd5\.?1|dd7\.?1|dd2\.?0|opus|pcm|mp3|lpcm)\b`)
	reGroup      = regexp.MustCompile(`-([A-Za-z][A-Za-z0-9]{1,15})$`)
)

type ParsedInfo struct {
	Title        string
	CleanTitle   string
	Type         string // "movie" | "series"
	Season       *int
	Episode      *int
	SE           string // normalized form, e.g. "S01E05" (series only)
	Quality      string // lowercase quality tag
	Language     string // ISO 639-1 code or "multi"
	LanguageTag  string // original tag from filename, e.g. "vostfr"
	Year         *int
	Codec        string
	Audio        string
	ReleaseGroup string
}

type FilenameParser struct{}

// SplitFilename splits a video filename into tokens using all common separators.
func SplitFilename(filename string) []string {
	return strings.FieldsFunc(filename, func(r rune) bool {
		return r == ',' || r == ';' || r == '.' || r == ':' ||
			r == ' ' || r == '-' || r == '_' || r == '(' || r == ')'
	})
}

func (p *FilenameParser) Parse(filename string) ParsedInfo {
	return ParseFilename(filename)
}

func ParseFilename(filename string) ParsedInfo {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	parts := filterEmpty(SplitFilename(name))

	info := ParsedInfo{Type: "movie"}
	if len(parts) == 0 {
		return info
	}

	firstMetaIdx := detectMetadata(&info, parts)
	extractTitle(&info, parts, firstMetaIdx)
	extractCodecAudio(&info, name)
	extractReleaseGroup(&info, name)

	return info
}

func filterEmpty(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// detectMetadata scans parts for series info, year, language, quality.
// Returns the index of the first metadata token found.
func detectMetadata(info *ParsedInfo, parts []string) int {
	firstMetaIdx := len(parts)
	for i, raw := range parts {
		lower := strings.ToLower(raw)
		if info.Season == nil && tryExtractEpisode(info, raw, i, &firstMetaIdx) {
			continue
		}
		if info.Year == nil && tryExtractYear(info, raw, i, &firstMetaIdx) {
			continue
		}
		if info.Language == "" && tryExtractLanguage(info, lower, i, &firstMetaIdx) {
			continue
		}
		if info.Quality == "" && tryExtractQuality(info, lower, i, &firstMetaIdx) {
			continue
		}
	}
	return firstMetaIdx
}

func tryExtractEpisode(info *ParsedInfo, raw string, idx int, firstMeta *int) bool {
	for _, re := range []*regexp.Regexp{reSeriesPart, reSeasonX} {
		if m := re.FindStringSubmatch(raw); len(m) == 3 {
			s, _ := strconv.Atoi(m[1])
			e, _ := strconv.Atoi(m[2])
			info.Type = "series"
			info.Season = &s
			info.Episode = &e
			info.SE = fmt.Sprintf("S%02dE%02d", s, e)
			if idx < *firstMeta {
				*firstMeta = idx
			}
			return true
		}
	}
	return false
}

func tryExtractYear(info *ParsedInfo, raw string, idx int, firstMeta *int) bool {
	if m := reYear.FindStringSubmatch(raw); len(m) == 2 {
		y, _ := strconv.Atoi(m[1])
		info.Year = &y
		if idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	return false
}

func tryExtractLanguage(info *ParsedInfo, lower string, idx int, firstMeta *int) bool {
	if iso, ok := languageTags[lower]; ok {
		info.Language = iso
		info.LanguageTag = lower
		if idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	return false
}

func tryExtractQuality(info *ParsedInfo, lower string, idx int, firstMeta *int) bool {
	if knownQualities[lower] {
		info.Quality = lower
		if idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	return false
}

func extractTitle(info *ParsedInfo, parts []string, firstMetaIdx int) {
	var titleParts []string
	if firstMetaIdx > 0 {
		titleParts = parts[:firstMetaIdx]
	} else if len(parts) > 0 {
		titleParts = parts[:1]
	}
	rawTitle := strings.TrimRight(strings.Join(titleParts, " "), " ")
	info.Title = ucFirst(rawTitle)
	info.CleanTitle = strings.ToLower(info.Title)
}

func extractCodecAudio(info *ParsedInfo, name string) {
	if m := reCodec.FindStringSubmatch(name); len(m) >= 2 {
		info.Codec = normalizeCodec(m[1])
	}
	if m := reAudio.FindStringSubmatch(name); len(m) >= 2 {
		info.Audio = strings.ToLower(m[1])
	}
}

func extractReleaseGroup(info *ParsedInfo, name string) {
	if m := reGroup.FindStringSubmatch(name); len(m) >= 2 {
		if !isKnownTag(m[1]) {
			info.ReleaseGroup = m[1]
		}
	}
}

// ucFirst capitalises the first letter only (mirrors normalize_video's UcFirst).
func ucFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func normalizeCodec(raw string) string {
	lower := strings.ToLower(strings.ReplaceAll(raw, ".", ""))
	switch lower {
	case "h264", "x264", "avc":
		return "h264"
	case "h265", "x265", "hevc":
		return "hevc"
	}
	return lower
}

func isKnownTag(s string) bool {
	known := map[string]bool{
		"mkv": true, "mp4": true, "avi": true, "mov": true,
		"aac": true, "dts": true, "flac": true, "ac3": true, "eac3": true,
		"x264": true, "x265": true, "h264": true, "h265": true, "hevc": true, "avc": true,
		"hdr": true, "hdr10": true, "sdr": true,
		"remux": true, "bluray": true, "webrip": true, "hdtv": true,
		"1080p": true, "720p": true, "480p": true, "2160p": true,
		"proper": true, "repack": true, "extended": true, "unrated": true,
		"atmos": true, "truehd": true, "opus": true, "mp3": true,
		"vf": true, "vostfr": true, "vo": true, "eng": true, "multi": true,
	}
	return known[strings.ToLower(s)]
}
