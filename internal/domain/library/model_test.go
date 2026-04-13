package library_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
)

func TestLibraryTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  library.LibraryType
		want string
	}{
		{"movie", library.Movie, "movie"},
		{"tvshow", library.TVShow, "tvshow"},
		{"music", library.Music, "music"},
		{"photo", library.Photo, "photo"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if string(c.got) != c.want {
				t.Errorf("LibraryType %s = %q, want %q", c.name, c.got, c.want)
			}
		})
	}
}

func TestMediaItemTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  library.MediaItemType
		want string
	}{
		{library.MovieItem, "movie"},
		{library.Series, "series"},
		{library.Season, "season"},
		{library.Episode, "episode"},
		{library.Track, "track"},
		{library.Album, "album"},
		{library.PhotoItem, "photo"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.typ) != c.want {
				t.Errorf("MediaItemType = %q, want %q", c.typ, c.want)
			}
		})
	}
}

func TestStreamTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  library.StreamType
		want string
	}{
		{library.Video, "video"},
		{library.Audio, "audio"},
		{library.Subtitle, "subtitle"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.typ) != c.want {
				t.Errorf("StreamType = %q, want %q", c.typ, c.want)
			}
		})
	}
}

func TestPersonRoleConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		role library.PersonRole
		want string
	}{
		{library.Actor, "actor"},
		{library.Director, "director"},
		{library.Writer, "writer"},
		{library.Producer, "producer"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.role) != c.want {
				t.Errorf("PersonRole = %q, want %q", c.role, c.want)
			}
		})
	}
}

func TestFileOpConstants(t *testing.T) {
	t.Parallel()

	// order matters — FileOp is iota-based and used in switch statements
	if library.FileCreated != 0 {
		t.Errorf("FileCreated = %d, want 0", library.FileCreated)
	}
	if library.FileModified != 1 {
		t.Errorf("FileModified = %d, want 1", library.FileModified)
	}
	if library.FileRemoved != 2 {
		t.Errorf("FileRemoved = %d, want 2", library.FileRemoved)
	}
	if library.FileRenamed != 3 {
		t.Errorf("FileRenamed = %d, want 3", library.FileRenamed)
	}
}

func TestTrailer_UseSubtitles(t *testing.T) {
	t.Parallel()

	// when UseSubtitles is false, SubtitleLanguage is irrelevant
	t.Run("no subtitles", func(t *testing.T) {
		t.Parallel()
		tr := library.Trailer{
			Site:             "YouTube",
			Key:              "abc123",
			Language:         "en",
			Official:         true,
			Type:             "Trailer",
			UseSubtitles:     false,
			SubtitleLanguage: "",
		}
		if tr.UseSubtitles {
			t.Error("expected UseSubtitles=false")
		}
	})

	// when trailer language != server language, UseSubtitles=true and SubtitleLanguage is set
	t.Run("subtitle needed", func(t *testing.T) {
		t.Parallel()
		tr := library.Trailer{
			Site:             "YouTube",
			Key:              "xyz789",
			Language:         "fr",
			Official:         false,
			Type:             "Teaser",
			UseSubtitles:     true,
			SubtitleLanguage: "en",
		}
		if !tr.UseSubtitles {
			t.Error("expected UseSubtitles=true")
		}
		if tr.SubtitleLanguage == "" {
			t.Error("expected SubtitleLanguage to be set when UseSubtitles=true")
		}
	})
}

func TestMediaItem_HasFile(t *testing.T) {
	t.Parallel()
	noFile := &library.MediaItem{}
	if noFile.HasFile() {
		t.Error("empty FilePath = no file")
	}
	withFile := &library.MediaItem{FilePath: "/data/movie.mkv"}
	if !withFile.HasFile() {
		t.Error("non-empty FilePath = has file")
	}
}

func TestMediaItem_IsPlayable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		typ      string
		filePath string
		want     bool
	}{
		{"movie with file", "movie", "/f.mkv", true},
		{"episode with file", "episode", "/f.mkv", true},
		{"track with file", "track", "/f.flac", true},
		{"series (container)", "series", "", false},
		{"season (container)", "season", "", false},
		{"movie without file", "movie", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := &library.MediaItem{Type: library.MediaItemType(tt.typ), FilePath: tt.filePath}
			if got := m.IsPlayable(); got != tt.want {
				t.Errorf("IsPlayable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMediaItem_NilableFields(t *testing.T) {
	t.Parallel()

	item := &library.MediaItem{
		ID:    "m1",
		Title: "Test Movie",
		Type:  library.MovieItem,
	}

	if item.IndexNumber != nil {
		t.Error("IndexNumber should be nil by default")
	}
	if item.CommunityRating != nil {
		t.Error("CommunityRating should be nil by default")
	}
	if item.PremiereDate != nil {
		t.Error("PremiereDate should be nil by default")
	}
	if item.Year != nil {
		t.Error("Year should be nil by default")
	}
}

func TestMediaItem_TrailersSlice(t *testing.T) {
	t.Parallel()

	item := &library.MediaItem{
		ID:    "m1",
		Title: "Big Movie",
	}
	if item.Trailers != nil {
		t.Error("Trailers should be nil (not empty slice) before assignment")
	}

	item.Trailers = []library.Trailer{
		{Site: "YouTube", Key: "k1", Language: "en", Official: true},
		{Site: "YouTube", Key: "k2", Language: "ja", UseSubtitles: true, SubtitleLanguage: "en"},
	}
	if len(item.Trailers) != 2 {
		t.Errorf("expected 2 trailers, got %d", len(item.Trailers))
	}
	if !item.Trailers[1].UseSubtitles {
		t.Error("second trailer should require subtitles")
	}
}

func TestScanSchedule_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	s := &library.ScanSchedule{
		LibraryID:      "lib1",
		CronExpression: "0 * * * *",
		Enabled:        true,
		LastRunAt:      &now,
		NextRunAt:      nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if s.NextRunAt != nil {
		t.Error("NextRunAt should be nil when not scheduled")
	}
	if s.LastRunAt == nil {
		t.Error("LastRunAt should be set")
	}
}

func TestTrickplayManifest_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	m := &library.TrickplayManifest{
		ID:          "tp1",
		MediaItemID: "m1",
		Interval:    10,
		Width:       320,
		Height:      180,
		TileWidth:   10,
		TileHeight:  10,
		ImageCount:  100,
		ImagePath:   "/data/trickplay/m1",
		CreatedAt:   now,
	}
	if m.Interval <= 0 {
		t.Error("Interval must be positive")
	}
	if m.ImageCount <= 0 {
		t.Error("ImageCount must be positive")
	}
}

func TestCollection_OptionalTmdbID(t *testing.T) {
	t.Parallel()

	// without TmdbID — valid for manually created collections
	col := &library.Collection{
		ID:   "c1",
		Name: "My Collection",
	}
	if col.TmdbID != nil {
		t.Error("TmdbID should be nil for non-TMDB collections")
	}

	id := 12345
	col.TmdbID = &id
	if *col.TmdbID != 12345 {
		t.Errorf("TmdbID = %d, want 12345", *col.TmdbID)
	}
}

func TestMediaStream_Getters(t *testing.T) {
	t.Parallel()

	bitDepth := 10
	colorSpace := "bt2020nc"
	hdrType := "HDR10"
	s := &library.MediaStream{
		StreamType: "video",
		BitDepth:   &bitDepth,
		ColorSpace: &colorSpace,
		HDRType:    &hdrType,
	}
	if s.GetStreamType() != "video" {
		t.Errorf("GetStreamType() = %q, want %q", s.GetStreamType(), "video")
	}
	if *s.GetBitDepth() != 10 {
		t.Errorf("GetBitDepth() = %d, want 10", *s.GetBitDepth())
	}
	if *s.GetColorSpace() != "bt2020nc" {
		t.Errorf("GetColorSpace() = %q, want %q", *s.GetColorSpace(), "bt2020nc")
	}
	if *s.GetHDRType() != "HDR10" {
		t.Errorf("GetHDRType() = %q, want %q", *s.GetHDRType(), "HDR10")
	}

	nilStream := &library.MediaStream{StreamType: "audio"}
	if nilStream.GetBitDepth() != nil {
		t.Error("GetBitDepth() should be nil")
	}
	if nilStream.GetColorSpace() != nil {
		t.Error("GetColorSpace() should be nil")
	}
	if nilStream.GetHDRType() != nil {
		t.Error("GetHDRType() should be nil")
	}
}

func TestMediaItem_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		item    library.MediaItem
		wantErr bool
	}{
		{"valid", library.MediaItem{LibraryID: "lib1", Type: "movie", Title: "Test"}, false},
		{"missing library", library.MediaItem{Type: "movie", Title: "Test"}, true},
		{"missing type", library.MediaItem{LibraryID: "lib1", Title: "Test"}, true},
		{"missing title", library.MediaItem{LibraryID: "lib1", Type: "movie"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.item.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUserRating_Values(t *testing.T) {
	t.Parallel()

	// UserRating values: -1=dislike, 0=neutral, 1=like — documented in ports
	cases := []struct {
		name  string
		value int
	}{
		{"dislike", -1},
		{"neutral", 0},
		{"like", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := library.UserRating{
				ProfileID:   "p1",
				MediaItemID: "m1",
				Value:       c.value,
			}
			if r.Value != c.value {
				t.Errorf("Value = %d, want %d", r.Value, c.value)
			}
		})
	}
}
