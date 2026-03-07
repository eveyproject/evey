package filesystem

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ParsedInfo struct {
	Title        string
	CleanTitle   string
	Type         string
	Season       *int
	Episode      *int
	Quality      string
	Language     string
	Year         *int
	Codec        string
	Audio        string
	ReleaseGroup string
}

var (
	reSeasonEpisode = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})`)
	reSeasonX       = regexp.MustCompile(`(\d{1,2})[Xx](\d{1,3})`)
	reYear          = regexp.MustCompile(`[\(\[]?((?:19|20)\d{2})[\)\]]?`)
	reQuality       = regexp.MustCompile(`(?i)(2160p|4[Kk]|1080p|720p|480p|remux|bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|hdrip)`)
	reLanguage      = regexp.MustCompile(`(?i)\b(english|french|german|spanish|italian|japanese|korean|chinese|portuguese|russian|arabic|hindi|multi)\b`)
	reCodec         = regexp.MustCompile(`(?i)\b(x264|x265|h\.?264|h\.?265|hevc|avc|av1|vp9|mpeg[24]|xvid|divx)\b`)
	reAudio         = regexp.MustCompile(`(?i)\b(aac|dts|dts-hd|dts-hd\.?ma|truehd|atmos|flac|ac3|eac3|dd5\.?1|dd7\.?1|dd2\.?0|7\.1|5\.1|2\.0|opus|pcm|mp3|lpcm)\b`)
	reReleaseGroup  = regexp.MustCompile(`(?i)(?:\-|\b)([A-Za-z][A-Za-z0-9]{1,15})$`)
	reJunk          = regexp.MustCompile(`(?i)(\.|\-|_|\[|\]|\(|\)|2160p|4k|1080p|720p|480p|remux|bluray|blu-ray|bdrip|brrip|webrip|web-dl|webdl|hdtv|dvdrip|hdrip|x264|x265|h264|h265|hevc|aac|dts|atmos|truehd|flac|ac3|dd5\.?1|7\.1|10bit|hdr|hdr10|dolby|vision|dv|proper|repack|extended|unrated|directors\.?cut|imax)`)
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

	if m := reCodec.FindStringSubmatch(name); len(m) >= 2 {
		info.Codec = normalizeCodec(m[1])
	}

	if m := reAudio.FindStringSubmatch(name); len(m) >= 2 {
		info.Audio = strings.ToLower(m[1])
	}

	if m := reReleaseGroup.FindStringSubmatch(name); len(m) >= 2 {
		group := m[1]
		if !isKnownTag(group) {
			info.ReleaseGroup = group
		}
	}

	titleEnd := len(name)
	for _, re := range []*regexp.Regexp{reSeasonEpisode, reSeasonX, reYear, reQuality} {
		if loc := re.FindStringIndex(name); loc != nil && loc[0] < titleEnd {
			titleEnd = loc[0]
		}
	}
	raw := name[:titleEnd]

	cleaned := strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(raw)
	cleaned = strings.TrimSpace(cleaned)

	info.Title = cleaned
	info.CleanTitle = strings.ToLower(reJunk.ReplaceAllString(cleaned, " "))
	info.CleanTitle = strings.Join(strings.Fields(info.CleanTitle), " ")

	return info
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
		"multi": true, "english": true, "french": true,
		"atmos": true, "truehd": true, "opus": true, "mp3": true,
	}
	return known[strings.ToLower(s)]
}
