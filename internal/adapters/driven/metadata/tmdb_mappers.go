package metadata

import (
	"strconv"
	"strings"
	"time"

	domain "github.com/evey-project/evey/internal/domain/metadata"
)

const maxTrailersPerLanguage = 3

func genreMapForType(mediaType string) map[int]string {
	if mediaType == "tv" {
		return tvGenres
	}
	return movieGenres
}

func mapSearchResult(item tmdbSearchItem, mediaType string, genreMap map[int]string) domain.MetadataResult {
	result := domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": strconv.Itoa(item.ID)},
		Popularity:  item.Popularity,
	}

	if mediaType == "tv" {
		result.Title = item.Name
		result.OriginalTitle = item.OriginalName
		result.PremiereDate = parseDate(item.FirstAirDate)
	} else {
		result.Title = item.Title
		result.OriginalTitle = item.OriginalTitle
		result.PremiereDate = parseDate(item.ReleaseDate)
	}

	result.Overview = item.Overview

	if result.PremiereDate != nil {
		y := result.PremiereDate.Year()
		result.Year = &y
	}

	if item.VoteAverage > 0 {
		r := item.VoteAverage
		result.CommunityRating = &r
	}

	for _, gid := range item.GenreIDs {
		if name, ok := genreMap[gid]; ok {
			result.Genres = append(result.Genres, name)
		}
	}

	return result
}

func mapDetailResult(d tmdbDetailResponse, mediaType string, serverLang string) domain.MetadataResult {
	result := domain.MetadataResult{
		ProviderIDs: map[string]string{"tmdb": strconv.Itoa(d.ID)},
		Ratings:     map[string]float64{},
		Tagline:     d.Tagline,
		Overview:    d.Overview,
	}

	mapDetailExternalIDs(&result, d.ExternalIDs)
	mapDetailTitleAndDate(&result, d, mediaType)
	applyRegionalTitle(&result, d, serverLang)
	mapDetailRating(&result, d.VoteAverage)

	for _, g := range d.Genres {
		result.Genres = append(result.Genres, g.Name)
	}
	for _, s := range d.ProductionCompanies {
		result.Studios = append(result.Studios, s.Name)
	}

	result.People = mapDetailPeople(d.Credits)
	result.Artwork = mapDetailArtwork(d.Images)

	if d.Videos != nil {
		result.Trailers = selectTrailers(d.Videos.Results, serverLang)
	}

	result.Collection = mapDetailCollection(d.BelongsToCollection)

	if len(d.Seasons) > 0 {
		result.SeasonEpisodeCounts = make(map[int]int, len(d.Seasons))
		for _, s := range d.Seasons {
			result.SeasonEpisodeCounts[s.SeasonNumber] = s.EpisodeCount
		}
	}

	mapDetailCertifications(&result, d, mediaType)

	return result
}

// mapDetailCertifications fills CertificationsByCountry and sets OfficialRating
// to the US entry as a backcompat default for the parental gate.
func mapDetailCertifications(result *domain.MetadataResult, d tmdbDetailResponse, mediaType string) {
	certs := map[string]string{}
	if mediaType == "tv" {
		if d.ContentRatings != nil {
			for _, r := range d.ContentRatings.Results {
				if r.Rating != "" {
					certs[r.Country] = r.Rating
				}
			}
		}
	} else {
		if d.ReleaseDates != nil {
			for _, r := range d.ReleaseDates.Results {
				// Prefer the first non-empty certification across the country's release types.
				for _, entry := range r.ReleaseDates {
					if entry.Certification != "" {
						certs[r.Country] = entry.Certification
						break
					}
				}
			}
		}
	}
	if len(certs) == 0 {
		return
	}
	result.CertificationsByCountry = certs
	if us, ok := certs["US"]; ok && result.OfficialRating == "" {
		result.OfficialRating = us
	}
}

func mapDetailExternalIDs(result *domain.MetadataResult, ext *tmdbExternalIDs) {
	if ext == nil {
		return
	}
	if ext.IMDBID != "" {
		result.ProviderIDs["imdb"] = ext.IMDBID
	}
	if ext.TVDBID > 0 {
		result.ProviderIDs["thetvdb"] = strconv.Itoa(ext.TVDBID)
	}
}

func mapDetailTitleAndDate(result *domain.MetadataResult, d tmdbDetailResponse, mediaType string) {
	if mediaType == "tv" {
		result.Title = d.Name
		result.OriginalTitle = d.OriginalName
		result.PremiereDate = parseDate(d.FirstAirDate)
	} else {
		result.Title = d.Title
		result.OriginalTitle = d.OriginalTitle
		result.PremiereDate = parseDate(d.ReleaseDate)
	}
	if result.PremiereDate != nil {
		y := result.PremiereDate.Year()
		result.Year = &y
	}
}

// applyRegionalTitle replaces a foreign-language title with the regional
// alternative title for the server's locale. TMDB only translates a `title`
// when an editor added a localized one; for many foreign productions it returns
// the original-language title even for `language=fr`, while the French title
// lives only in alternative_titles (e.g. a German doc whose FR title is
// "Les grottes immergées …"). We override only when TMDB clearly gave us no
// localized title — result.Title still equals the original title AND the work
// is in a different language than the server — so a genuine localized title is
// never clobbered.
func applyRegionalTitle(result *domain.MetadataResult, d tmdbDetailResponse, serverLang string) {
	if serverLang == "" || result.Title == "" || result.Title != result.OriginalTitle {
		return
	}
	lang := strings.ToLower(strings.SplitN(serverLang, "-", 2)[0])
	if d.OriginalLanguage != "" && d.OriginalLanguage == lang {
		return // already in the server's language — nothing to localize
	}
	regions := regionsForLanguage(lang)
	if len(regions) == 0 {
		return
	}
	alts := d.AlternativeTitles.all()
	for _, want := range regions {
		for _, a := range alts {
			if strings.EqualFold(a.Region, want) && a.Title != "" {
				result.Title = a.Title
				return
			}
		}
	}
}

// regionsForLanguage maps a language code to the ISO-3166 country codes whose
// alternative titles are most likely to carry that language's release title,
// in priority order. Falls back to the uppercased language code (correct for
// fr→FR, de→DE, es→ES, it→IT, … where language and country codes coincide).
func regionsForLanguage(lang string) []string {
	switch lang {
	case "en":
		return []string{"US", "GB", "CA", "AU"}
	case "pt":
		return []string{"BR", "PT"}
	case "zh":
		return []string{"CN", "TW", "HK", "SG"}
	case "ja":
		return []string{"JP"}
	case "ko":
		return []string{"KR"}
	case "hi":
		return []string{"IN"}
	case "":
		return nil
	default:
		return []string{strings.ToUpper(lang)}
	}
}

func mapDetailRating(result *domain.MetadataResult, voteAverage float64) {
	if voteAverage <= 0 {
		return
	}
	r := voteAverage
	result.CommunityRating = &r
	result.Ratings["tmdb"] = voteAverage
}

func mapDetailPeople(credits *tmdbCredits) []domain.PersonResult {
	if credits == nil {
		return nil
	}
	var people []domain.PersonResult
	for _, c := range credits.Cast {
		people = append(people, domain.PersonResult{
			Name:       c.Name,
			Role:       "Actor",
			Character:  c.Character,
			ThumbURL:   profileURL(c.ProfilePath),
			ProviderID: strconv.Itoa(c.ID),
		})
	}
	for _, c := range credits.Crew {
		if c.Job == "Director" {
			people = append(people, domain.PersonResult{
				Name:       c.Name,
				Role:       "Director",
				ThumbURL:   profileURL(c.ProfilePath),
				ProviderID: strconv.Itoa(c.ID),
			})
		}
	}
	return people
}

func mapDetailArtwork(images *tmdbImages) []domain.ArtworkResult {
	if images == nil {
		return nil
	}
	var artwork []domain.ArtworkResult
	artwork = appendImages(artwork, images.Posters, domain.ArtworkTypePoster, 5)
	artwork = appendImages(artwork, images.Backdrops, domain.ArtworkTypeBackdrop, 5)
	artwork = appendImages(artwork, images.Logos, domain.ArtworkTypeLogo, 3)
	return artwork
}

func appendImages(dst []domain.ArtworkResult, imgs []tmdbImage, artType string, limit int) []domain.ArtworkResult {
	for i, img := range imgs {
		if i >= limit {
			break
		}
		dst = append(dst, domain.ArtworkResult{
			Type:     artType,
			URL:      imageBaseURL + img.FilePath,
			Width:    img.Width,
			Height:   img.Height,
			Language: img.ISO639_1,
			Provider: "tmdb",
		})
	}
	return dst
}

func mapDetailCollection(col *tmdbCollection) *domain.CollectionRef {
	if col == nil {
		return nil
	}
	ref := &domain.CollectionRef{
		TmdbID: col.ID,
		Name:   col.Name,
	}
	if col.PosterPath != "" {
		ref.PosterURL = "https://image.tmdb.org/t/p/w500" + col.PosterPath
	}
	if col.BackdropPath != "" {
		ref.BackdropURL = imageBaseURL + col.BackdropPath
	}
	return ref
}

// selectTrailers filters videos to type "Trailer", keeps max 3 per language,
// sorted official-first within each language group.
func selectTrailers(videos []tmdbVideo, _ string) []domain.TrailerResult {
	sortByOfficial := func(a, b tmdbVideo) int {
		if a.Official && !b.Official {
			return -1
		}
		if !a.Official && b.Official {
			return 1
		}
		return 0
	}

	byLang := map[string][]tmdbVideo{}
	for _, v := range videos {
		if v.Type != "Trailer" || v.Key == "" {
			continue
		}
		byLang[v.Language] = append(byLang[v.Language], v)
	}

	var result []domain.TrailerResult
	for _, vids := range byLang {
		sortVideos(vids, sortByOfficial)
		for i, v := range vids {
			if i >= maxTrailersPerLanguage {
				break
			}
			result = append(result, domain.TrailerResult{
				Site:     v.Site,
				Key:      v.Key,
				Language: v.Language,
				Official: v.Official,
				Type:     v.Type,
			})
		}
	}
	return result
}

func sortVideos(vs []tmdbVideo, less func(a, b tmdbVideo) int) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && less(vs[j], vs[j-1]) < 0; j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
}

func profileURL(path string) string {
	if path == "" {
		return ""
	}
	return imageBaseURL + path
}

func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}
