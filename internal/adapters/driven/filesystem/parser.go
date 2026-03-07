package filesystem

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ParsedInfo struct {
	Title      string
	CleanTitle string
	Type       string
	Season     *int
	Episode    *int
	Quality    string
	Language   string
	Year       *int
}

var (
	// S01E05, s01e05, S1E5
	reSeasonEpisode = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})`)
	// 01x05
	reSeasonX = regexp.MustCompile(`(\d{1,2})[Xx](\d{1,3})`)
	// year in parentheses or standalone 4-digit year between 1900-2099
	reYear = regexp.MustCompile(`[\(\[]?((?:19|20)\d{2})[\)\]]?`)
	// quality patterns
	reQuality = regexp.MustCompile(`(?i)(2160p|4[Kk]|1080p|720p|480p|remux|bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|hdrip)`)
	// language tags
	reLanguage = regexp.MustCompile(`(?i)\b(english|french|german|spanish|italian|japanese|korean|chinese|portuguese|russian|arabic|hindi|multi)\b`)
	// junk tokens to remove for clean title
	reJunk = regexp.MustCompile(`(?i)(\.|\-|_|\[|\]|\(|\)|2160p|4k|1080p|720p|480p|remux|bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|hdrip|x264|x265|h264|h265|hevc|aac|dts|atmos|truehd|flac|ac3|dd5\.?1|7\.1|10bit|hdr|hdr10|dolby|vision|dv|proper|repack|extended|unrated|directors\.?cut|imax)`)
)

type FilenameParser struct{}

func NewFilenameParser() *FilenameParser {
	return &FilenameParser{}
}

func (p *FilenameParser) Parse(filename string) ParsedInfo {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	info := ParsedInfo{Type: "movie"}

	if m := reSeasonEpisode.FindStringSubmatch(name); len(m) == 3 {
		info.Type = "series"
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		info.Season = &s
		info.Episode = &e
	} else if m := reSeasonX.FindStringSubmatch(name); len(m) == 3 {
		info.Type = "series"
		s, _ := strconv.Atoi(m[1])
		e, _ := strconv.Atoi(m[2])
		info.Season = &s
		info.Episode = &e
	}

	if m := reYear.FindStringSubmatch(name); len(m) == 2 {
		y, _ := strconv.Atoi(m[1])
		info.Year = &y
	}

	if m := reQuality.FindStringSubmatch(name); len(m) >= 2 {
		info.Quality = strings.ToLower(m[1])
	}

	if m := reLanguage.FindStringSubmatch(name); len(m) >= 2 {
		info.Language = strings.ToLower(m[1])
	}

	// Extract title: everything before the first known pattern match
	titleEnd := len(name)
	for _, re := range []*regexp.Regexp{reSeasonEpisode, reSeasonX, reYear, reQuality} {
		if loc := re.FindStringIndex(name); loc != nil && loc[0] < titleEnd {
			titleEnd = loc[0]
		}
	}
	raw := name[:titleEnd]

	// Clean separators
	cleaned := strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(raw)
	cleaned = strings.TrimSpace(cleaned)

	info.Title = cleaned
	info.CleanTitle = strings.ToLower(reJunk.ReplaceAllString(cleaned, " "))
	info.CleanTitle = strings.Join(strings.Fields(info.CleanTitle), " ")

	return info
}
