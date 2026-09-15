package filesystem

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// extraTokensMu protects the admin-configurable extension of the static
// knownQualities map and reEditionSuffix regex. Defaults stay hardcoded
// (knownQualities + reEditionSuffix below) so the parser always has a
// sensible baseline; admins extend at runtime via SetExtraParserTokens
// (called from the app layer on boot and on server_config update).
var (
	extraTokensMu  sync.RWMutex
	extraQualities = map[string]bool{}
	extraEditionRe *regexp.Regexp // nil = no extra suffix patterns wired
)

// SetExtraParserTokens replaces the runtime token extensions. Pass nil or
// empty slices to clear. Tokens are lowercased here (the lookup path also
// lowercases) so callers can pass either case. Edition suffixes are joined
// into a single alternation regex so they're applied in one regex pass
// alongside the static reEditionSuffix.
func SetExtraParserTokens(qualities, editionSuffixes []string) {
	qs := make(map[string]bool, len(qualities))
	for _, q := range qualities {
		q = strings.TrimSpace(strings.ToLower(q))
		if q != "" {
			qs[q] = true
		}
	}
	var re *regexp.Regexp
	parts := make([]string, 0, len(editionSuffixes))
	for _, s := range editionSuffixes {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		parts = append(parts, regexp.QuoteMeta(s))
	}
	if len(parts) > 0 {
		// (?i) for case-insensitive, \b word boundaries so "dc" doesn't
		// match inside "advance" or "redcoat".
		re = regexp.MustCompile(`(?i)\b(?:` + strings.Join(parts, "|") + `)\b`)
	}
	extraTokensMu.Lock()
	extraQualities = qs
	extraEditionRe = re
	extraTokensMu.Unlock()
}

// extraQualityMatch is the runtime-config lookup used by tryExtractQuality.
// Returns false for the empty/no-extras case so the static map remains
// authoritative when the admin hasn't configured anything.
func extraQualityMatch(lower string) bool {
	extraTokensMu.RLock()
	defer extraTokensMu.RUnlock()
	return extraQualities[lower]
}

// extraEditionRegex returns the current admin-configured edition suffix
// regex (or nil). Read-locked so concurrent Parse calls see a consistent
// snapshot.
func extraEditionRegex() *regexp.Regexp {
	extraTokensMu.RLock()
	defer extraTokensMu.RUnlock()
	return extraEditionRe
}

// strongLanguageTags maps unambiguous filename tokens to ISO 639-1 codes.
// These always anchor the title end when they appear in a filename -
// nobody writes a movie called "French" or "VOSTFR".
var strongLanguageTags = map[string]string{
	// French
	"vf": "fr", "vff": "fr", "vfi": "fr", "vfq": "fr",
	"french": "fr", "truefrench": "fr", "vostfr": "fr", "subfrench": "fr",
	"fre": "fr", "fra": "fr",
	// English
	"vo": "en", "voen": "en", "eng": "en", "english": "en",
	// German
	"vostde": "de", "vode": "de", "german": "de", "deutsch": "de",
	"deu": "de", "ger": "de",
	// Spanish
	"vostes": "es", "voes": "es", "spanish": "es", "espanol": "es",
	"spa": "es",
	// Italian
	"vostit": "it", "voit": "it", "italian": "it", "italiano": "it",
	"ita": "it",
	// Japanese
	"vostjp": "ja", "vojp": "ja", "japanese": "ja", "jpn": "ja",
	// Portuguese
	"vostpt": "pt", "vopt": "pt", "portuguese": "pt", "por": "pt",
	// Russian
	"vostru": "ru", "voru": "ru", "russian": "ru", "rus": "ru",
	// Chinese
	"vostzh": "zh", "vozh": "zh", "chinese": "zh", "chi": "zh",
	// Arabic
	"vostar": "ar", "voar": "ar", "arabic": "ar", "ara": "ar",
	// Hindi
	"hindi": "hi", "hin": "hi",
	// Korean
	"korean": "ko", "kor": "ko",
	// Multi
	"multi": "multi",
}

// weakLanguageTags are bare 2-letter ISO codes that also happen to be
// common words in natural language ("it" = English pronoun + Italian ISO,
// "de" = French preposition + German ISO, "fr" = rare but "en"/"es"/"ja"
// exist as prepositions/interjections in various langs). They only count
// as language tags when they appear AFTER a strong metadata marker
// (S0xE0x / year / quality / strong language tag / codec) - otherwise a
// title like "It welcome to derry S01E03" would truncate to "It" because
// idx 0 gets flagged as Italian.
var weakLanguageTags = map[string]string{
	"fr": "fr", "en": "en", "de": "de", "es": "es",
	"it": "it", "ja": "ja", "pt": "pt", "ru": "ru",
	"zh": "zh", "ar": "ar", "hi": "hi", "ko": "ko",
}

// knownQualities for part-based detection (lowercase).
// Includes the bare-number resolutions (no `p` suffix) because some upstream
// renamers omit the `p` - without these entries, a token like `1080` leaks
// into the title and TMDB then matches on a polluted query (e.g. "Mr Wolf
// 1080" → no match, "American Ultra 1080" → no match).
var knownQualities = map[string]bool{
	"2160p": true, "2160": true, "4k": true, "uhd": true, "8k": true,
	"1080p": true, "1080": true,
	"720p": true, "720": true,
	"480p": true, "480": true,
	"360p": true, "360": true,
	"bluray": true, "bdrip": true, "brrip": true,
	"remux":  true,
	"webrip": true, "webdl": true, "web": true, "web-dl": true,
	"hdtv": true, "hdrip": true, "hdr": true,
	"dvdrip": true, "dvdscr": true, "dvd": true,
	"cam": true, "hdcam": true, "ts": true, "telesync": true, "screener": true,
}

// reBogusResolution matches 4-digit + P/p tokens that the standard quality
// set doesn't cover (e.g. "2016P" - likely a typo for 2160P from some
// renaming tool). Filtering these out keeps them from polluting the title.
var reBogusResolution = regexp.MustCompile(`^\d{3,4}[pP]$`)

var (
	reSeriesPart = regexp.MustCompile(`(?i)^[sS](\d{1,2})[eE](\d{1,3})$`)
	reSeasonX    = regexp.MustCompile(`(?i)^(\d{1,2})[xX](\d{1,3})$`)
	reYear       = regexp.MustCompile(`^((?:19|20)\d{2})$`)
	reCodec      = regexp.MustCompile(`(?i)\b(x264|x265|h\.?264|h\.?265|hevc|avc|av1|vp9|mpeg[24]|xvid|divx)\b`)
	reAudio      = regexp.MustCompile(`(?i)\b(aac|dts-hd\.?ma|dts-hd|dts|truehd|atmos|flac|ac3|eac3|dd5\.?1|dd7\.?1|dd2\.?0|opus|pcm|mp3|lpcm)\b`)
	reGroup      = regexp.MustCompile(`-([A-Za-z][A-Za-z0-9]{1,15})$`)
	// reSEDelimited collapses `S03 E17`, `S03.E17`, `S03_E17` into a single
	// token before split. Real-world Dr House rips ship with a space between
	// the season and episode markers, which made the parser leave the SxxExx
	// in the title - and the TV scan service then created one orphan SERIES
	// per file instead of grouping them under "Dr House".
	reSEDelimited = regexp.MustCompile(`(?i)([sS]\d{1,2})[ ._-]+([eE]\d{1,3})`)
	// reEditionSuffix strips editorial cut tags that would otherwise leak
	// into the title and break the TMDB exact-match shortcut. Covers FR
	// ("version longue") and EN ("director's cut", "extended cut", etc.)
	// variants. Stripped during pre-tokenisation so the resulting title is
	// what the user wanted to search ("Danse avec les loups" instead of
	// "Danse avec les loups version longue").
	reEditionSuffix = regexp.MustCompile(`(?i)\b(director(?:'s|s)?[ ._-]?cut|version[ ._-]longue|extended(?:[ ._-](?:cut|edition))?|theatrical[ ._-]cut|final[ ._-]cut|special[ ._-]edition|criterion(?:[ ._-](?:collection|edition))?|remastered|uncut|unrated|redux|extreme[ ._-]edition|extreme|dc)\b`)
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
	// Collapse `S03 E17` / `S03.E17` / `S03_E17` into the canonical `S03E17`
	// so tokenisation produces a single SxxExx token that reSeriesPart can
	// match. Without this step the season+episode markers leak into the
	// title and the TV scan service treats every file as its own SERIES.
	name = reSEDelimited.ReplaceAllString(name, "$1$2")
	// Strip editorial cut suffixes ("director's cut", "version longue",
	// etc.) before tokenisation. Without this they end up tacked onto the
	// title (e.g. "Danse avec les loups version longue") and the TMDB exact
	// title shortcut never fires.
	name = reEditionSuffix.ReplaceAllString(name, " ")
	// Admin-configured extra edition suffixes (set via the admin server-
	// config UI / parser.config server_config key) - applied after the
	// static regex so admins can add patterns without having to wait for
	// a release. A common case: distributor-specific edition names not
	// covered by the baseline list ("dc", "extreme", "imax", …).
	if re := extraEditionRegex(); re != nil {
		name = re.ReplaceAllString(name, " ")
	}

	parts := filterEmpty(SplitFilename(name))
	// Strip leading/trailing non-alphanumeric runes from each token so a
	// SxxExx (or year/quality) glued to punctuation the splitter doesn't treat
	// as a separator still classifies. Real-world + synthetic culprits: "+S02E06+",
	// "[S02E06]", "{1080p}". Without this the marker leaks into the title and the
	// TV scan creates one SERIES per episode (the Furies-duplicated-6x bug).
	// Done at the TOKEN level only - `name` stays intact for the codec/audio/
	// release-group regexes below, which rely on "." / "-" inside tokens
	// (x264, h.264, dd5.1, -GROUP).
	for i := range parts {
		parts[i] = trimTokenPunct(parts[i])
	}
	parts = filterEmpty(parts)

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

// trimTokenPunct strips leading/trailing non-alphanumeric runes from a token
// (keeping internal ones so "h.264"-style tokens are untouched if ever passed).
// Unicode-aware so accented titles survive.
func trimTokenPunct(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
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
		// No `info.Year == nil` guard: a later year token in the filename
		// is more likely the release year than a leading one that's part
		// of the title (e.g. `1917.2019.1080p` → title=1917, year=2019).
		// tryExtractYear handles the overwrite.
		if tryExtractYear(info, raw, i, &firstMetaIdx) {
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
		// A year at position 0 is almost always part of the title itself
		// ("2001 A Space Odyssey", "300", "1917"). Anchoring firstMeta=0
		// here would truncate the title to just that token and feed bogus
		// queries to TMDB. Only treat a year as a metadata boundary when
		// some content precedes it.
		if idx > 0 && idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	return false
}

func tryExtractLanguage(info *ParsedInfo, lower string, idx int, firstMeta *int) bool {
	if iso, ok := strongLanguageTags[lower]; ok {
		info.Language = iso
		info.LanguageTag = lower
		if idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	// Weak 2-letter ISO codes require a strong marker to the left to rule
	// out natural-language collisions. They never move firstMeta - only
	// strong markers anchor the title/metadata boundary.
	if iso, ok := weakLanguageTags[lower]; ok && *firstMeta < idx {
		info.Language = iso
		info.LanguageTag = lower
		return true
	}
	return false
}

func tryExtractQuality(info *ParsedInfo, lower string, idx int, firstMeta *int) bool {
	if knownQualities[lower] || extraQualityMatch(lower) {
		info.Quality = lower
		if idx < *firstMeta {
			*firstMeta = idx
		}
		return true
	}
	// Catch `2016P`, `2017P`, etc. - these look like resolutions but aren't
	// in the canonical set. They show up in misconfigured renamer output;
	// the safe move is to drop them rather than leak them into the title.
	// Already-classified years are handled by tryExtractYear before this.
	if reBogusResolution.MatchString(lower) {
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

// ucFirst capitalises the first letter only, rune-aware. s[:1] would slice the
// first BYTE, mangling any multi-byte first rune (e.g. "Æon", "Über", "千と千尋")
// into the replacement char + an orphan continuation byte, which then poisons
// the metadata-provider search for foreign titles.
func ucFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s // not valid UTF-8 at the head; leave it untouched
	}
	return string(unicode.ToUpper(r)) + s[size:]
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

var knownTags = map[string]bool{
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

func isKnownTag(s string) bool {
	return knownTags[strings.ToLower(s)]
}
