package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

func TestParse_SeriesWithSE(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The.Office.S01E05.720p.BluRay.x264-GROUP.mkv")

	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "The Office", info.Title)
	require.NotNil(t, info.Season)
	assert.Equal(t, 1, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 5, *info.Episode)
	assert.Equal(t, "720p", info.Quality)
	assert.Equal(t, "h264", info.Codec)
	assert.Equal(t, "GROUP", info.ReleaseGroup)
}

func TestParse_SeriesWithSeasonX(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Breaking.Bad.03x10.1080p.mkv")

	assert.Equal(t, "series", info.Type)
	require.NotNil(t, info.Season)
	assert.Equal(t, 3, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 10, *info.Episode)
	assert.Equal(t, "1080p", info.Quality)
}

func TestParse_MovieWithYear(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Inception.2010.1080p.BluRay.x264.DTS-FGT.mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "Inception", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2010, *info.Year)
	assert.Equal(t, "1080p", info.Quality)
	assert.Equal(t, "h264", info.Codec)
	assert.Equal(t, "dts", info.Audio)
	assert.Equal(t, "FGT", info.ReleaseGroup)
}

func TestParse_MovieWithYearInParens(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The Matrix (1999) [1080p].mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "The Matrix", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 1999, *info.Year)
}

func TestParse_4K_HEVC(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Dune.Part.Two.2024.2160p.WEB-DL.x265.DTS-HD.MA-SPARKS.mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "Dune Part Two", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2024, *info.Year)
	assert.Equal(t, "2160p", info.Quality)
	assert.Equal(t, "hevc", info.Codec)
}

func TestParse_AudioFormats(t *testing.T) {
	tests := []struct {
		filename string
		audio    string
	}{
		{"Movie.2020.1080p.BluRay.DTS.mkv", "dts"},
		{"Movie.2020.1080p.BluRay.AAC.mkv", "aac"},
		{"Movie.2020.1080p.BluRay.TrueHD.mkv", "truehd"},
		{"Movie.2020.1080p.BluRay.FLAC.mkv", "flac"},
		{"Movie.2020.1080p.BluRay.AC3.mkv", "ac3"},
		{"Movie.2020.1080p.BluRay.Atmos.mkv", "atmos"},
		{"Movie.2020.1080p.BluRay.DD5.1.mkv", "dd5.1"},
		{"Movie.2020.1080p.BluRay.EAC3.mkv", "eac3"},
	}

	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.audio, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, tt.audio, info.Audio)
		})
	}
}

func TestParse_CodecVariations(t *testing.T) {
	tests := []struct {
		filename string
		codec    string
	}{
		{"Movie.x264.mkv", "h264"},
		{"Movie.x265.mkv", "hevc"},
		{"Movie.H264.mkv", "h264"},
		{"Movie.H.265.mkv", "hevc"},
		{"Movie.HEVC.mkv", "hevc"},
		{"Movie.AVC.mkv", "h264"},
		{"Movie.AV1.mkv", "av1"},
		{"Movie.VP9.mkv", "vp9"},
	}

	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, tt.codec, info.Codec)
		})
	}
}

func TestParse_Quality(t *testing.T) {
	tests := []struct {
		filename string
		quality  string
	}{
		{"Movie.2160p.mkv", "2160p"},
		{"Movie.4K.mkv", "4k"},
		{"Movie.1080p.mkv", "1080p"},
		{"Movie.720p.mkv", "720p"},
		{"Movie.480p.mkv", "480p"},
		{"Movie.BluRay.mkv", "bluray"},
		{"Movie.WEB-DL.mkv", "web-dl"},
		{"Movie.WEBRip.mkv", "webrip"},
		{"Movie.HDTV.mkv", "hdtv"},
		{"Movie.REMUX.mkv", "remux"},
	}

	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, tt.quality, info.Quality)
		})
	}
}

func TestParse_Language(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Movie.2020.1080p.French.mkv")
	assert.Equal(t, "french", info.Language)
}

func TestParse_MultiLanguage(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Movie.2020.1080p.Multi.mkv")
	assert.Equal(t, "multi", info.Language)
}

func TestParse_ReleaseGroup(t *testing.T) {
	tests := []struct {
		filename string
		group    string
	}{
		{"Movie.2020.1080p.BluRay-SPARKS.mkv", "SPARKS"},
		{"Movie.2020.1080p.BluRay-FGT.mkv", "FGT"},
		{"Movie.2020.1080p.BluRay-YTS.mkv", "YTS"},
		{"Movie.2020.1080p.BluRay-RARBG.mkv", "RARBG"},
	}

	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, tt.group, info.ReleaseGroup)
		})
	}
}

func TestParse_CleanTitle(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The.Dark.Knight.2008.1080p.BluRay.x264.DTS-FGT.mkv")
	assert.Equal(t, "The Dark Knight", info.Title)
	assert.Equal(t, "the dark knight", info.CleanTitle)
}

func TestParse_NoMatches(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("random_file.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "random file", info.Title)
	assert.Nil(t, info.Season)
	assert.Nil(t, info.Episode)
	assert.Nil(t, info.Year)
	assert.Empty(t, info.Quality)
	assert.Empty(t, info.Codec)
	assert.Empty(t, info.Audio)
}

func TestParse_FullPath(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("/media/tv/The.Office.S02E03.720p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "The Office", info.Title)
	require.NotNil(t, info.Season)
	assert.Equal(t, 2, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 3, *info.Episode)
}

func TestParse_UnderscoreSeparators(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("My_Movie_2020_1080p_BluRay.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "My Movie", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2020, *info.Year)
}

func TestParse_ComplexName(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Game.of.Thrones.S08E06.1080p.WEB-DL.x265.DTS-HD-FLUX.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "Game of Thrones", info.Title)
	require.NotNil(t, info.Season)
	assert.Equal(t, 8, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 6, *info.Episode)
	assert.Equal(t, "1080p", info.Quality)
	assert.Equal(t, "hevc", info.Codec)
}
