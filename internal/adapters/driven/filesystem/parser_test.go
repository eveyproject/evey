package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int { return &v }

// --- SplitFilename ---

func TestSplitFilename_DotSeparated(t *testing.T) {
	assert.Equal(t, []string{"breaking", "bad", "s01e01", "vostfr", "1080p", "mkv"},
		SplitFilename("breaking.bad.s01e01.vostfr.1080p.mkv"))
}

func TestSplitFilename_HyphenSeparated(t *testing.T) {
	// hyphens are separators — same as normalize_video
	parts := SplitFilename("The-Office-S01E05-720p.mkv")
	assert.Contains(t, parts, "The")
	assert.Contains(t, parts, "Office")
	assert.Contains(t, parts, "S01E05")
}

func TestSplitFilename_MixedSeparators(t *testing.T) {
	parts := SplitFilename("Film_(2020)_1080p.mkv")
	assert.Contains(t, parts, "Film")
	assert.Contains(t, parts, "2020")
	assert.Contains(t, parts, "1080p")
}

// --- Type detection ---

func TestParse_SeriesWithSE(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The.Office.S01E05.720p.BluRay.x264-GROUP.mkv")

	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "The Office", info.Title)
	assert.Equal(t, "S01E05", info.SE)
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
	assert.Equal(t, "S03E10", info.SE)
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
	assert.Empty(t, info.SE)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2010, *info.Year)
	assert.Equal(t, "1080p", info.Quality)
	assert.Equal(t, "h264", info.Codec)
	assert.Equal(t, "dts", info.Audio)
	assert.Equal(t, "FGT", info.ReleaseGroup)
}

func TestParse_MovieWithYearInParens(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The Matrix (1999) 1080p.mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "The Matrix", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 1999, *info.Year)
}

func TestParse_4K_HEVC(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Dune.Part.Two.2024.2160p.x265.mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "Dune Part Two", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2024, *info.Year)
	assert.Equal(t, "2160p", info.Quality)
	assert.Equal(t, "hevc", info.Codec)
}

// --- SE normalization ---

func TestParse_SE_Normalized(t *testing.T) {
	tests := []struct {
		filename string
		se       string
		s        int
		e        int
	}{
		{"Series.S01E05.mkv", "S01E05", 1, 5},
		{"Series.S1E1.mkv", "S01E01", 1, 1},
		{"Series.S12E100.mkv", "S12E100", 12, 100},
		{"Series.1x05.mkv", "S01E05", 1, 5},
		{"Series.03x10.mkv", "S03E10", 3, 10},
	}
	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.se, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, "series", info.Type)
			assert.Equal(t, tt.se, info.SE)
			require.NotNil(t, info.Season)
			assert.Equal(t, tt.s, *info.Season)
			require.NotNil(t, info.Episode)
			assert.Equal(t, tt.e, *info.Episode)
		})
	}
}

// --- Language: ISO code + original tag ---

func TestParse_Language_VF(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Inception.2010.VF.1080p.BluRay.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "vf", info.LanguageTag)
}

func TestParse_Language_VOSTFR(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Interstellar.2014.VOSTFR.720p.WEB.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "vostfr", info.LanguageTag)
}

func TestParse_Language_TrueFrench(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("The.Matrix.1999.TrueFrench.1080p.BluRay.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "truefrench", info.LanguageTag)
}

func TestParse_Language_Multi(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Dune.2021.MULTI.2160p.UHD.BluRay.mkv")
	assert.Equal(t, "multi", info.Language)
	assert.Equal(t, "multi", info.LanguageTag)
}

func TestParse_Language_VO(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Everything.Everywhere.2022.VO.1080p.mkv")
	assert.Equal(t, "en", info.Language)
	assert.Equal(t, "vo", info.LanguageTag)
}

func TestParse_Language_ENG(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Movie.2020.1080p.BluRay.ENG.mkv")
	assert.Equal(t, "en", info.Language)
	assert.Equal(t, "eng", info.LanguageTag)
}

func TestParse_Language_VOSTDE(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Dark.S01E01.VOSTDE.1080p.mkv")
	assert.Equal(t, "de", info.Language)
	assert.Equal(t, "vostde", info.LanguageTag)
}

func TestParse_Language_VOSTES(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Money.Heist.S02E04.VOSTES.720p.mkv")
	assert.Equal(t, "es", info.Language)
	assert.Equal(t, "vostes", info.LanguageTag)
}

func TestParse_Language_None(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("avatar.2009.1080p.bluray.x264.mkv")
	assert.Equal(t, "", info.Language)
	assert.Equal(t, "", info.LanguageTag)
}

// --- Edge cases (from normalize_video) ---

func TestParse_TitleIsYear_1917(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("1917.2019.1080p.bluray.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "1917", info.Title)
	assert.NotNil(t, info.Year) // 2019 detected as production year
}

func TestParse_TitleIsYear_2012(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("2012.2009.1080p.bluray.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "2012", info.Title)
}

func TestParse_TitleIsYear_Serie1923(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("1923.s01e01.1080p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "1923", info.Title)
	assert.Equal(t, "S01E01", info.SE)
}

func TestParse_TitleIsYear_Serie1883(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("1883.s02e05.vf.720p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "1883", info.Title)
	assert.Equal(t, "S02E05", info.SE)
	assert.Equal(t, "fr", info.Language)
}

// --- Audio ---

func TestParse_AudioFormats(t *testing.T) {
	tests := []struct {
		filename string
		audio    string
	}{
		{"Movie.2020.1080p.DTS.mkv", "dts"},
		{"Movie.2020.1080p.AAC.mkv", "aac"},
		{"Movie.2020.1080p.TrueHD.mkv", "truehd"},
		{"Movie.2020.1080p.FLAC.mkv", "flac"},
		{"Movie.2020.1080p.AC3.mkv", "ac3"},
		{"Movie.2020.1080p.Atmos.mkv", "atmos"},
		{"Movie.2020.1080p.EAC3.mkv", "eac3"},
	}
	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.audio, func(t *testing.T) {
			assert.Equal(t, tt.audio, p.Parse(tt.filename).Audio)
		})
	}
}

// --- Codec ---

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
			assert.Equal(t, tt.codec, p.Parse(tt.filename).Codec)
		})
	}
}

// --- Quality ---

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
		{"Movie.WEBRip.mkv", "webrip"},
		{"Movie.HDTV.mkv", "hdtv"},
		{"Movie.REMUX.mkv", "remux"},
		{"Movie.NoQuality.mkv", ""},
	}
	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			assert.Equal(t, tt.quality, p.Parse(tt.filename).Quality)
		})
	}
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
			assert.Equal(t, tt.group, p.Parse(tt.filename).ReleaseGroup)
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
	assert.Nil(t, info.Season)
	assert.Nil(t, info.Episode)
	assert.Empty(t, info.SE)
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
	assert.Equal(t, "S02E03", info.SE)
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

func TestParse_ComplexSerie(t *testing.T) {
	p := NewFilenameParser()
	info := p.Parse("Game.of.Thrones.S08E06.1080p.x265.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "Game of Thrones", info.Title)
	assert.Equal(t, "S08E06", info.SE)
	require.NotNil(t, info.Season)
	assert.Equal(t, 8, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 6, *info.Episode)
	assert.Equal(t, "1080p", info.Quality)
	assert.Equal(t, "hevc", info.Codec)
}

// --- normalize_video fixture compatibility ---

func TestParse_NormalizeVideoMovies(t *testing.T) {
	tests := []struct {
		filename string
		title    string
		lang     string
		langTag  string
		quality  string
	}{
		{"avatar.2009.1080p.bluray.x264.mkv", "Avatar", "", "", "1080p"},
		{"inception.2010.vf.1080p.bluray.mkv", "Inception", "fr", "vf", "1080p"},
		{"interstellar.2014.vostfr.720p.web.mkv", "Interstellar", "fr", "vostfr", "720p"},
		{"the.matrix.1999.truefrench.1080p.bluray.mkv", "The matrix", "fr", "truefrench", "1080p"},
		{"dune.2021.multi.2160p.uhd.bluray.mkv", "Dune", "multi", "multi", "2160p"},
		{"parasite.2019.vostfr.1080p.web.mkv", "Parasite", "fr", "vostfr", "1080p"},
		{"oppenheimer.2023.vf.4k.bluray.mkv", "Oppenheimer", "fr", "vf", "4k"},
	}
	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, tt.title, info.Title, "title")
			assert.Equal(t, tt.lang, info.Language, "language ISO")
			assert.Equal(t, tt.langTag, info.LanguageTag, "language tag")
			assert.Equal(t, tt.quality, info.Quality, "quality")
		})
	}
}

func TestParse_NormalizeVideoSeries(t *testing.T) {
	tests := []struct {
		filename string
		title    string
		lang     string
		langTag  string
		se       string
		season   int
		episode  int
	}{
		{"breaking.bad.s01e01.vostfr.1080p.mkv", "Breaking bad", "fr", "vostfr", "S01E01", 1, 1},
		{"game.of.thrones.s05e09.vf.720p.mkv", "Game of thrones", "fr", "vf", "S05E09", 5, 9},
		{"the.wire.s02e03.multi.1080p.mkv", "The wire", "multi", "multi", "S02E03", 2, 3},
		{"stranger.things.s04e01.vostfr.2160p.mkv", "Stranger things", "fr", "vostfr", "S04E01", 4, 1},
		{"the.mandalorian.1x05.vf.1080p.mkv", "The mandalorian", "fr", "vf", "S01E05", 1, 5},
		{"dark.s01e01.vostde.1080p.mkv", "Dark", "de", "vostde", "S01E01", 1, 1},
		{"money.heist.s02e04.vostes.720p.mkv", "Money heist", "es", "vostes", "S02E04", 2, 4},
	}
	p := NewFilenameParser()
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := p.Parse(tt.filename)
			assert.Equal(t, "series", info.Type, "type")
			assert.Equal(t, tt.title, info.Title, "title")
			assert.Equal(t, tt.lang, info.Language, "language ISO")
			assert.Equal(t, tt.langTag, info.LanguageTag, "language tag")
			assert.Equal(t, tt.se, info.SE, "SE")
			require.NotNil(t, info.Season)
			assert.Equal(t, tt.season, *info.Season, "season")
			require.NotNil(t, info.Episode)
			assert.Equal(t, tt.episode, *info.Episode, "episode")
		})
	}
}
