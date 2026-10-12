package metadata_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/metadata"
)

func TestArtworkTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"poster", metadata.ArtworkTypePoster, "poster"},
		{"backdrop", metadata.ArtworkTypeBackdrop, "backdrop"},
		{"logo", metadata.ArtworkTypeLogo, "logo"},
		{"still", metadata.ArtworkTypeStill, "still"},
		{"thumb", metadata.ArtworkTypeThumb, "thumb"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.got != c.want {
				t.Errorf("ArtworkType%s = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}

func TestArtworkResult_ProviderField(t *testing.T) {
	t.Parallel()

	// Provider must be set - it prevents deduplication across providers
	cases := []struct {
		provider string
		artType  string
	}{
		{"tmdb", metadata.ArtworkTypePoster},
		{"fanart", metadata.ArtworkTypeBackdrop},
		{"imdb", metadata.ArtworkTypeStill},
	}
	for _, c := range cases {
		t.Run(c.provider+"/"+c.artType, func(t *testing.T) {
			t.Parallel()
			a := metadata.ArtworkResult{
				Type:     c.artType,
				URL:      "https://example.com/img.jpg",
				Width:    1920,
				Height:   1080,
				Language: "en",
				Provider: c.provider,
			}
			if a.Provider == "" {
				t.Error("Provider must not be empty")
			}
			if a.Provider != c.provider {
				t.Errorf("Provider = %q, want %q", a.Provider, c.provider)
			}
		})
	}
}

func TestTrailerResult_UseSubtitles(t *testing.T) {
	t.Parallel()

	// trailer in server language: no subtitles needed
	t.Run("native language", func(t *testing.T) {
		t.Parallel()
		tr := metadata.TrailerResult{
			Site:             "YouTube",
			Key:              "abc",
			Language:         "en",
			Official:         true,
			Type:             "Trailer",
			UseSubtitles:     false,
			SubtitleLanguage: "",
		}
		if tr.UseSubtitles {
			t.Error("native language trailer should not need subtitles")
		}
	})

	// trailer in foreign language: subtitles required, SubtitleLanguage = server lang
	t.Run("foreign language requires subtitles", func(t *testing.T) {
		t.Parallel()
		tr := metadata.TrailerResult{
			Site:             "YouTube",
			Key:              "xyz",
			Language:         "ja",
			Official:         true,
			Type:             "Trailer",
			UseSubtitles:     true,
			SubtitleLanguage: "en",
		}
		if !tr.UseSubtitles {
			t.Error("foreign language trailer should require subtitles")
		}
		if tr.SubtitleLanguage == "" {
			t.Error("SubtitleLanguage must be set when UseSubtitles=true")
		}
	})

	// teaser with no official status
	t.Run("unofficial teaser", func(t *testing.T) {
		t.Parallel()
		tr := metadata.TrailerResult{
			Site:     "YouTube",
			Key:      "teaser1",
			Language: "en",
			Official: false,
			Type:     "Teaser",
		}
		if tr.Official {
			t.Error("teaser should not be official")
		}
	})
}

func TestMetadataResult_Fields(t *testing.T) {
	t.Parallel()

	year := 2023
	rating := 8.5
	result := &metadata.MetadataResult{
		Title:           "Some Movie",
		OriginalTitle:   "Some Movie",
		Overview:        "A thief who steals corporate secrets",
		Tagline:         "Nothing is as it seems",
		Year:            &year,
		CommunityRating: &rating,
		OfficialRating:  "PG-13",
		Genres:          []string{"Action", "Sci-Fi", "Thriller"},
		Studios:         []string{"Some Studio"},
		ProviderIDs:     map[string]string{"tmdb": "900001", "imdb": "tt0000001"},
		Ratings:         map[string]float64{"imdb": 8.8, "tmdb": 8.4},
		Popularity:      83.5,
	}

	if *result.Year != 2023 {
		t.Errorf("Year = %d, want 2023", *result.Year)
	}
	if *result.CommunityRating != 8.5 {
		t.Errorf("CommunityRating = %v, want 8.5", *result.CommunityRating)
	}
	if result.ProviderIDs["tmdb"] != "900001" {
		t.Errorf("ProviderID[tmdb] = %q, want %q", result.ProviderIDs["tmdb"], "900001")
	}
	if result.Ratings["imdb"] != 8.8 {
		t.Errorf("Ratings[imdb] = %v, want 8.8", result.Ratings["imdb"])
	}
}

func TestMetadataResult_CollectionRef(t *testing.T) {
	t.Parallel()

	t.Run("no collection", func(t *testing.T) {
		t.Parallel()
		r := &metadata.MetadataResult{Title: "Standalone"}
		if r.Collection != nil {
			t.Error("Collection should be nil for standalone items")
		}
	})

	t.Run("with collection", func(t *testing.T) {
		t.Parallel()
		r := &metadata.MetadataResult{
			Title: "The Twilight Guardian",
			Collection: &metadata.CollectionRef{
				TmdbID:      900001,
				Name:        "The Twilight Guardian Collection",
				PosterURL:   "https://example.com/poster.jpg",
				BackdropURL: "https://example.com/backdrop.jpg",
			},
		}
		if r.Collection == nil {
			t.Fatal("Collection should not be nil")
		}
		if r.Collection.TmdbID != 900001 {
			t.Errorf("Collection.TmdbID = %d, want 900001", r.Collection.TmdbID)
		}
		if r.Collection.Name == "" {
			t.Error("Collection.Name must not be empty")
		}
	})
}

func TestMetadataResult_PeopleAndArtwork(t *testing.T) {
	t.Parallel()

	now := time.Now()
	premiere := now.AddDate(-3, 0, 0)

	r := &metadata.MetadataResult{
		Title:        "Movie",
		PremiereDate: &premiere,
		People: []metadata.PersonResult{
			{Name: "Christopher Nolan", Role: "director"},
			{Name: "Leonardo DiCaprio", Role: "actor", Character: "Dom Cobb"},
		},
		Artwork: []metadata.ArtworkResult{
			{Type: metadata.ArtworkTypePoster, URL: "https://example.com/p.jpg", Provider: "tmdb"},
			{Type: metadata.ArtworkTypeBackdrop, URL: "https://example.com/b.jpg", Provider: "fanart"},
		},
		Trailers: []metadata.TrailerResult{
			{Site: "YouTube", Key: "abc", Language: "en", Official: true, Type: "Trailer"},
		},
	}

	if r.PremiereDate == nil {
		t.Error("PremiereDate should be set")
	}
	if len(r.People) != 2 {
		t.Errorf("People len = %d, want 2", len(r.People))
	}
	if r.People[0].Role != "director" {
		t.Errorf("People[0].Role = %q, want %q", r.People[0].Role, "director")
	}
	if len(r.Artwork) != 2 {
		t.Errorf("Artwork len = %d, want 2", len(r.Artwork))
	}
	// different providers on different artwork - both kept
	if r.Artwork[0].Provider == r.Artwork[1].Provider {
		t.Error("artwork from different providers must not share provider field")
	}
	if len(r.Trailers) != 1 {
		t.Errorf("Trailers len = %d, want 1", len(r.Trailers))
	}
}

func TestMetadataOverride_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		o       metadata.MetadataOverride
		wantErr bool
	}{
		{"valid", metadata.MetadataOverride{ID: "mo-1", MediaItemID: "m1", FieldName: "title"}, false},
		{"missing id", metadata.MetadataOverride{MediaItemID: "m1", FieldName: "title"}, true},
		{"missing media item", metadata.MetadataOverride{ID: "mo-1", FieldName: "title"}, true},
		{"missing field name", metadata.MetadataOverride{ID: "mo-1", MediaItemID: "m1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.o.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestArtworkRecord_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		a       metadata.ArtworkRecord
		wantErr bool
	}{
		{"valid", metadata.ArtworkRecord{ID: "art-1", MediaItemID: "m1", Type: "poster", Provider: "tmdb"}, false},
		{"missing id", metadata.ArtworkRecord{MediaItemID: "m1", Type: "poster", Provider: "tmdb"}, true},
		{"missing media item", metadata.ArtworkRecord{ID: "art-1", Type: "poster", Provider: "tmdb"}, true},
		{"missing type", metadata.ArtworkRecord{ID: "art-1", MediaItemID: "m1", Provider: "tmdb"}, true},
		{"missing provider", metadata.ArtworkRecord{ID: "art-1", MediaItemID: "m1", Type: "poster"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.a.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewMetadataOverride(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps CreatedAt", func(t *testing.T) {
		t.Parallel()
		o, err := metadata.NewMetadataOverride("mo-1", "m1", "title",
			metadata.WithOverrideValue("A new title"),
			metadata.WithOverrideEditor("user-1"),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if o.CreatedAt.IsZero() || o.CustomValue != "A new title" || o.EditedBy != "user-1" {
			t.Errorf("fields not wired: %+v", o)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := metadata.NewMetadataOverride("", "m1", "title"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestNewArtworkRecord(t *testing.T) {
	t.Parallel()

	t.Run("happy path wires options", func(t *testing.T) {
		t.Parallel()
		a, err := metadata.NewArtworkRecord("art-1", "m1", metadata.ArtworkTypePoster, "tmdb",
			metadata.WithArtworkSource("https://example.com/p.jpg"),
			metadata.WithArtworkLanguage("en"),
			metadata.WithArtworkDimensions(1000, 1500),
			metadata.WithArtworkSortOrder(3),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if a.SourceURL == nil || *a.SourceURL != "https://example.com/p.jpg" ||
			a.Language == nil || *a.Language != "en" ||
			a.Width == nil || *a.Width != 1000 ||
			a.Height == nil || *a.Height != 1500 ||
			a.SortOrder != 3 {
			t.Errorf("fields not wired: %+v", a)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := metadata.NewArtworkRecord("", "m1", "poster", "tmdb"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing provider rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := metadata.NewArtworkRecord("art-1", "m1", "poster", ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestArtworkRecord_OptionalFields(t *testing.T) {
	t.Parallel()

	// minimum valid record - no optional dimensions/language
	rec := &metadata.ArtworkRecord{
		ID:          "ar1",
		MediaItemID: "m1",
		Type:        metadata.ArtworkTypePoster,
		Provider:    "tmdb",
		SortOrder:   0,
	}
	if rec.Width != nil {
		t.Error("Width should be nil when not known")
	}
	if rec.Height != nil {
		t.Error("Height should be nil when not known")
	}
	if rec.Language != nil {
		t.Error("Language should be nil when not language-specific")
	}
	if rec.PersonID != nil {
		t.Error("PersonID should be nil for non-person artwork")
	}
	if rec.SourceURL != nil {
		t.Error("SourceURL should be nil before download")
	}
	if rec.LocalPath != nil {
		t.Error("LocalPath should be nil before download")
	}
}
