package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	info := ParseFilename("The.Office.S01E05.720p.BluRay.x264-GROUP.mkv")

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
	info := ParseFilename("Breaking.Bad.03x10.1080p.mkv")

	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "S03E10", info.SE)
	require.NotNil(t, info.Season)
	assert.Equal(t, 3, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 10, *info.Episode)
	assert.Equal(t, "1080p", info.Quality)
}

func TestParse_MovieWithYear(t *testing.T) {
	info := ParseFilename("Inception.2010.1080p.BluRay.x264.DTS-FGT.mkv")

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
	info := ParseFilename("The Matrix (1999) 1080p.mkv")

	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "The Matrix", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 1999, *info.Year)
}

func TestParse_4K_HEVC(t *testing.T) {
	info := ParseFilename("Dune.Part.Two.2024.2160p.x265.mkv")

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
		// Space-separated SxxExx (real-world: `Dr House S03 E17.avi`).
		// Without normalization the parser saw two unrelated tokens
		// "S03" + "E17" and fell back to movie type, causing one orphan
		// SERIES row per episode in the prod DB.
		{"Dr House S03 E17.avi", "S03E17", 3, 17},
		{"Show.Name.S1.E5.mkv", "S01E05", 1, 5},
		{"Show Name S12 E100 1080p.mkv", "S12E100", 12, 100},
	}
	for _, tt := range tests {
		t.Run(tt.se, func(t *testing.T) {
			info := ParseFilename(tt.filename)
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
	info := ParseFilename("Inception.2010.VF.1080p.BluRay.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "vf", info.LanguageTag)
}

func TestParse_Language_VOSTFR(t *testing.T) {
	info := ParseFilename("Interstellar.2014.VOSTFR.720p.WEB.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "vostfr", info.LanguageTag)
}

func TestParse_Language_TrueFrench(t *testing.T) {
	info := ParseFilename("The.Matrix.1999.TrueFrench.1080p.BluRay.mkv")
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "truefrench", info.LanguageTag)
}

func TestParse_Language_Multi(t *testing.T) {
	info := ParseFilename("Dune.2021.MULTI.2160p.UHD.BluRay.mkv")
	assert.Equal(t, "multi", info.Language)
	assert.Equal(t, "multi", info.LanguageTag)
}

func TestParse_Language_VO(t *testing.T) {
	info := ParseFilename("Everything.Everywhere.2022.VO.1080p.mkv")
	assert.Equal(t, "en", info.Language)
	assert.Equal(t, "vo", info.LanguageTag)
}

func TestParse_Language_ENG(t *testing.T) {
	info := ParseFilename("Movie.2020.1080p.BluRay.ENG.mkv")
	assert.Equal(t, "en", info.Language)
	assert.Equal(t, "eng", info.LanguageTag)
}

func TestParse_Language_VOSTDE(t *testing.T) {
	info := ParseFilename("Dark.S01E01.VOSTDE.1080p.mkv")
	assert.Equal(t, "de", info.Language)
	assert.Equal(t, "vostde", info.LanguageTag)
}

func TestParse_Language_VOSTES(t *testing.T) {
	info := ParseFilename("Money.Heist.S02E04.VOSTES.720p.mkv")
	assert.Equal(t, "es", info.Language)
	assert.Equal(t, "vostes", info.LanguageTag)
}

func TestParse_Language_None(t *testing.T) {
	info := ParseFilename("avatar.2009.1080p.bluray.x264.mkv")
	assert.Empty(t, info.Language)
	assert.Empty(t, info.LanguageTag)
}

// --- Edge cases (from normalize_video) ---

func TestParse_TitleIsYear_1917(t *testing.T) {
	info := ParseFilename("1917.2019.1080p.bluray.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "1917", info.Title)
	assert.NotNil(t, info.Year) // 2019 detected as production year
}

func TestParse_TitleIsYear_2012(t *testing.T) {
	info := ParseFilename("2012.2009.1080p.bluray.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "2012", info.Title)
}

func TestParse_TitleIsYear_Serie1923(t *testing.T) {
	info := ParseFilename("1923.s01e01.1080p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "1923", info.Title)
	assert.Equal(t, "S01E01", info.SE)
}

func TestParse_TitleIsYear_Serie1883(t *testing.T) {
	info := ParseFilename("1883.s02e05.vf.720p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "1883", info.Title)
	assert.Equal(t, "S02E05", info.SE)
	assert.Equal(t, "fr", info.Language)
}

// Real-world regression: a year token at the very start of the filename
// must NOT be treated as a metadata boundary when more title words follow.
// Otherwise the parser drops the rest of the title and TMDB matches the
// wrong movie ("2001 L'odyssée De L'espace" → "Master Q 2001").
func TestParse_LeadingYearKeepsTitle(t *testing.T) {
	info := ParseFilename("2001 L'odyssée De L'espace - 1080p.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "2001 L'odyssée De L'espace", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2001, *info.Year)
}

// Inverse case: when the title-like leading year is followed by an actual
// release year, the later year wins and the title sticks to the lead token.
func TestParse_LeadingYearOverwrittenByLater(t *testing.T) {
	info := ParseFilename("1999.2024.1080p.mkv")
	assert.Equal(t, "1999", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2024, *info.Year)
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
	for _, tt := range tests {
		t.Run(tt.audio, func(t *testing.T) {
			assert.Equal(t, tt.audio, ParseFilename(tt.filename).Audio)
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
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.codec, ParseFilename(tt.filename).Codec)
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
		// Bare-number resolutions (no `p` suffix). Some upstream renamers
		// omit the `p` — without these, the digits leak into the title.
		{"Movie.2160.mkv", "2160"},
		{"Movie.1080.mkv", "1080"},
		{"Movie.720.mkv", "720"},
		// Bogus 4-digit + P that aren't canonical (real-world: prod orphans
		// shipped with "2016P" / "2017P", probably renamer typos for 2160P).
		// Treat as quality so they don't pollute the title.
		{"Movie.2016P.mkv", "2016p"},
		{"Movie.2017P.mkv", "2017p"},
		// Production gaps surfaced on pré-prod orphans: HDRIP/HDR not in
		// the original map left "Spectral HDRIP" stranded forever.
		{"Movie.HDRIP.mkv", "hdrip"},
		{"Movie.HDR.mkv", "hdr"},
	}
	for _, tt := range tests {
		t.Run(tt.quality, func(t *testing.T) {
			assert.Equal(t, tt.quality, ParseFilename(tt.filename).Quality)
		})
	}
}

func TestParse_EditionSuffixStripped(t *testing.T) {
	tests := []struct {
		filename  string
		wantTitle string
	}{
		{"Danse avec les loups - version longue.mkv", "Danse avec les loups"},
		{"L'effet papillon - director's cut.mkv", "L'effet papillon"},
		{"L'effet papillon directors cut.mkv", "L'effet papillon"},
		{"Movie - Extended Cut - 1080p.mkv", "Movie"},
		{"Apocalypse Now Redux.mkv", "Apocalypse Now"},
		{"Blade Runner Final Cut.mkv", "Blade Runner"},
		{"Movie - Theatrical Cut.mkv", "Movie"},
		{"Movie Special Edition.mkv", "Movie"},
		{"Movie Remastered.mkv", "Movie"},
		// Title that legitimately contains "uncut" word stays unaffected
		// only if it's not a standalone token — here it is, so it's
		// stripped. Documents intentional risk.
		{"Some Movie Uncut.mkv", "Some Movie"},
		// Production gaps surfaced on pré-prod Saw + Léon orphans: "dc"
		// (director's cut abbreviation), "extreme" (alt edition tag), and
		// "extended" standalone (without an explicit "cut"/"edition"
		// follow-up token).
		{"Saw iv dc.mkv", "Saw iv"},
		{"Saw iii dc extreme.mkv", "Saw iii"},
		{"Léon remastered extended.mkv", "Léon"},
		{"Léon extended.mkv", "Léon"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.wantTitle, ParseFilename(tt.filename).Title)
		})
	}
}

func TestParse_TitleNotPollutedByOrphanResolution(t *testing.T) {
	// Regression: "Mr Wolf - 1080p.mkv" used to land as title "Mr Wolf",
	// but "Mr Wolf - 1080.mkv" (no `p`) was leaking "1080" into the title
	// and TMDB then matched the animated title "The Trial of Mr. Wolf" instead.
	tests := []struct {
		filename  string
		wantTitle string
	}{
		{"Mr Wolf - 1080p.mkv", "Mr Wolf"},
		{"Mr Wolf - 1080.mkv", "Mr Wolf"},
		{"Mr Wolf 1080.mkv", "Mr Wolf"},
		{"American Ultra - 1080.mkv", "American Ultra"},
		{"American Ultra 1080.mkv", "American Ultra"},
		{"Die hard 2 - 2016P.mkv", "Die hard 2"},
		{"Die hard 2 2016P.mkv", "Die hard 2"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.wantTitle, ParseFilename(tt.filename).Title)
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
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			assert.Equal(t, tt.group, ParseFilename(tt.filename).ReleaseGroup)
		})
	}
}

func TestParse_CleanTitle(t *testing.T) {
	info := ParseFilename("The.Dark.Knight.2008.1080p.BluRay.x264.DTS-FGT.mkv")
	assert.Equal(t, "The Dark Knight", info.Title)
	assert.Equal(t, "the dark knight", info.CleanTitle)
}

func TestParse_NoMatches(t *testing.T) {
	info := ParseFilename("random_file.mkv")
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
	info := ParseFilename("/media/tv/The.Office.S02E03.720p.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "The Office", info.Title)
	assert.Equal(t, "S02E03", info.SE)
	require.NotNil(t, info.Season)
	assert.Equal(t, 2, *info.Season)
	require.NotNil(t, info.Episode)
	assert.Equal(t, 3, *info.Episode)
}

func TestParse_UnderscoreSeparators(t *testing.T) {
	info := ParseFilename("My_Movie_2020_1080p_BluRay.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "My Movie", info.Title)
	require.NotNil(t, info.Year)
	assert.Equal(t, 2020, *info.Year)
}

func TestParse_ComplexSerie(t *testing.T) {
	info := ParseFilename("Game.of.Thrones.S08E06.1080p.x265.mkv")
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
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := ParseFilename(tt.filename)
			assert.Equal(t, tt.title, info.Title, "title")
			assert.Equal(t, tt.lang, info.Language, "language ISO")
			assert.Equal(t, tt.langTag, info.LanguageTag, "language tag")
			assert.Equal(t, tt.quality, info.Quality, "quality")
		})
	}
}

// --- Regression: weak 2-letter ISO codes must not truncate title ---
// (before the fix, "It welcome..." got truncated to "It" because "it" was
// matched as Italian at idx 0; "Le flic de shanghai" became "Le flic"
// because "de" was matched as German at idx 2.)

func TestParse_TitleStartingWithLanguageLikeWord_It(t *testing.T) {
	info := ParseFilename("It welcome to derry S01E03 - MULTI - 1080P.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "It welcome to derry", info.Title)
	assert.Equal(t, "S01E03", info.SE)
	assert.Equal(t, "multi", info.Language)
}

func TestParse_TitleWithFrenchPrepositionDe(t *testing.T) {
	info := ParseFilename("Le flic de shanghai S01E01.avi")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "Le flic de shanghai", info.Title)
	assert.Equal(t, "S01E01", info.SE)
}

func TestParse_WeakLanguageAfterStrongMarker_Still_Works(t *testing.T) {
	// bare "FR" after year is still a language tag
	info := ParseFilename("Movie.2020.FR.mkv")
	assert.Equal(t, "movie", info.Type)
	assert.Equal(t, "Movie", info.Title)
	assert.Equal(t, "fr", info.Language)
	assert.Equal(t, "fr", info.LanguageTag)
}

func TestParse_WeakLanguageBeforeStrongMarker_IgnoredAsTitle(t *testing.T) {
	// "IT" without any strong marker before it stays in the title
	info := ParseFilename("IT Crowd S01E01.mkv")
	assert.Equal(t, "series", info.Type)
	assert.Equal(t, "IT Crowd", info.Title)
	assert.Equal(t, "S01E01", info.SE)
	assert.Empty(t, info.Language) // "IT" was part of the title, not a lang tag
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
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := ParseFilename(tt.filename)
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

func TestSetExtraParserTokens_QualityAndEdition(t *testing.T) {
	// Admin extends the baseline maps at runtime via the parser_config
	// server_config row → SetExtraParserTokens. Verify both axes (quality
	// + edition suffix) flow through ParseFilename. Cleanup resets the
	// globals so other tests in this package see a clean baseline.
	t.Cleanup(func() { SetExtraParserTokens(nil, nil) })

	// Quality tokens must be single-word (the parser splits on . _ - SPACE).
	// Edition suffixes go through a regex pre-tokenisation so they CAN span
	// multiple words.
	SetExtraParserTokens(
		[]string{"customquality", "WEIRDTAG"},
		[]string{"distributor cut", "anniversary"},
	)
	tests := []struct {
		filename  string
		wantTitle string
		wantQual  string
	}{
		// Extra quality token is recognised (case-insensitive).
		{"Movie.CustomQuality.mkv", "Movie", "customquality"},
		{"Movie.WEIRDTAG.mkv", "Movie", "weirdtag"},
		// Extra edition suffix is stripped from the title (multi-word OK
		// because the regex runs before tokenisation).
		{"Movie - distributor cut.mkv", "Movie", ""},
		{"Movie Anniversary.mkv", "Movie", ""},
		// Baseline still works alongside the extras.
		{"Movie.1080p.mkv", "Movie", "1080p"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			info := ParseFilename(tt.filename)
			assert.Equal(t, tt.wantTitle, info.Title, "title")
			assert.Equal(t, tt.wantQual, info.Quality, "quality")
		})
	}
}

func TestSetExtraParserTokens_NilClears(t *testing.T) {
	// Round-trip: setting tokens then clearing them returns the parser to
	// its baseline behaviour.
	SetExtraParserTokens([]string{"foo"}, []string{"bar"})
	assert.Equal(t, "foo", ParseFilename("Movie.foo.mkv").Quality)
	SetExtraParserTokens(nil, nil)
	got := ParseFilename("Movie.foo.mkv")
	assert.Equal(t, "", got.Quality, "extras must be cleared")
	assert.Equal(t, "Movie foo", got.Title, "foo leaks back into title once cleared")
}
