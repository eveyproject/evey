package library_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
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

func TestFileOpConstants(t *testing.T) {
	t.Parallel()

	// order matters - FileOp is iota-based and used in switch statements
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
	withFile := &library.MediaItem{File: &library.MediaFile{Path: "/data/movie.mkv"}}
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
			m := &library.MediaItem{Type: library.MediaItemType(tt.typ)}
			if tt.filePath != "" {
				m.File = &library.MediaFile{Path: tt.filePath}
			}
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
	if item.Classification != nil {
		t.Error("Classification should be nil by default")
	}
	if item.Catalog != nil {
		t.Error("Catalog should be nil by default")
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

func TestNewCollection(t *testing.T) {
	t.Parallel()

	c, err := library.NewCollection("col-1", "Marvel", nil, "added_at")
	if err != nil {
		t.Fatalf("valid collection: %v", err)
	}
	if c.ID != "col-1" || c.Name != "Marvel" || c.SortBy != "added_at" {
		t.Errorf("fields not set: %+v", c)
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		t.Error("timestamps must be stamped")
	}

	if _, err := library.NewCollection("", "Marvel", nil, ""); err == nil {
		t.Error("empty ID must be rejected")
	}
	if _, err := library.NewCollection("col-1", "", nil, ""); err == nil {
		t.Error("empty name must be rejected")
	}
}

func TestNewScanSchedule(t *testing.T) {
	t.Parallel()

	s, err := library.NewScanSchedule("lib1", "0 * * * *", true, nil)
	if err != nil {
		t.Fatalf("valid schedule: %v", err)
	}
	if s.LibraryID != "lib1" || s.CronExpression != "0 * * * *" || !s.Enabled {
		t.Errorf("fields not set: %+v", s)
	}

	if _, err := library.NewScanSchedule("", "0 * * * *", true, nil); err == nil {
		t.Error("empty library ID must be rejected")
	}
	if _, err := library.NewScanSchedule("lib1", "", true, nil); err == nil {
		t.Error("empty cron expression must be rejected")
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

	// without TmdbID - valid for manually created collections
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

func TestMediaStream_FrameRateMode(t *testing.T) {
	t.Parallel()

	unset := &library.MediaStream{StreamType: "video"}
	if unset.FrameRateMode != "" {
		t.Errorf("FrameRateMode = %q, want \"\" (unknown) by default", unset.FrameRateMode)
	}

	vfr := &library.MediaStream{StreamType: "video", FrameRateMode: "vfr"}
	if vfr.FrameRateMode != "vfr" {
		t.Errorf("FrameRateMode = %q, want %q", vfr.FrameRateMode, "vfr")
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
		{"invalid type", library.MediaItem{LibraryID: "lib1", Type: "banana", Title: "Test"}, true},
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

func TestNewLibrary(t *testing.T) {
	t.Parallel()

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		l, err := library.NewLibrary("lib-1", "Movies", library.Movie,
			library.WithLibraryPaths([]string{"/media/movies"}),
			library.WithLibraryScanInterval(60),
			library.WithLibraryScanOnStartup(true),
			library.WithLibraryRealtimeMonitoring(true),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if l.ID != "lib-1" || l.Name != "Movies" || l.Type != library.Movie {
			t.Errorf("core fields wrong: %+v", l)
		}
		if !l.Enabled {
			t.Error("should default to Enabled=true")
		}
		if l.ScanIntervalMinutes != 60 || !l.ScanOnStartup || !l.RealtimeMonitoring {
			t.Errorf("options not applied: %+v", l)
		}
		if l.CreatedAt.IsZero() || l.UpdatedAt.IsZero() {
			t.Error("timestamps not stamped")
		}
	})

	t.Run("missing ID", func(t *testing.T) {
		t.Parallel()
		_, err := library.NewLibrary("", "Movies", library.Movie)
		if err == nil {
			t.Fatal("expected error on empty ID")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		_, err := library.NewLibrary("lib-1", "", library.Movie)
		if err == nil {
			t.Fatal("expected error on empty name")
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		t.Parallel()
		_, err := library.NewLibrary("lib-1", "Movies", "banana")
		if err == nil {
			t.Fatal("expected error on invalid type")
		}
	})
}

func TestLibrary_Validate(t *testing.T) {
	t.Parallel()
	valid := &library.Library{ID: "lib-1", Name: "Movies", Type: library.Movie}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid library rejected: %v", err)
	}

	cases := []*library.Library{
		{Name: "Movies", Type: library.Movie},
		{ID: "lib-1", Type: library.Movie},
		{ID: "lib-1", Name: "Movies"},
		{ID: "lib-1", Name: "Movies", Type: "banana"},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d should fail: %+v", i, c)
		}
	}
}

func TestLibraryType_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[library.LibraryType]bool{
		library.Movie:           true,
		library.TVShow:          true,
		library.Music:           true,
		library.Photo:           true,
		library.LibraryType(""): false,
		"garbage":               false,
	}
	for typ, want := range cases {
		if typ.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", typ, !want, want)
		}
	}
}

func TestMediaItemType_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[library.MediaItemType]bool{
		library.MovieItem:         true,
		library.Series:            true,
		library.Season:            true,
		library.Episode:           true,
		library.Track:             true,
		library.Album:             true,
		library.PhotoItem:         true,
		library.MediaItemType(""): false,
		"garbage":                 false,
	}
	for typ, want := range cases {
		if typ.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", typ, !want, want)
		}
	}
}

func TestStreamType_IsValid(t *testing.T) {
	t.Parallel()
	cases := map[library.StreamType]bool{
		library.Video:          true,
		library.Audio:          true,
		library.Subtitle:       true,
		library.StreamType(""): false,
		"garbage":              false,
	}
	for typ, want := range cases {
		if typ.IsValid() != want {
			t.Errorf("%q.IsValid() = %v, want %v", typ, !want, want)
		}
	}
}

func TestUserRating_Values(t *testing.T) {
	t.Parallel()

	// UserRating values: -1=dislike, 0=neutral, 1=like - documented in ports
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

// media_files.bitrate sat at 0 on every row for the whole life of the column:
// probers report per-TRACK bitrates (a Matroska BPS tag ~80% of a real library
// lacks), never a container-level one. Consumers therefore re-derived it - or
// forked ffprobe for it. This is that derivation, in one place.
func mustDur(ms int64) shared.Duration {
	d, err := shared.NewDurationMs(ms)
	if err != nil {
		panic(err)
	}
	return d
}

func TestMediaFile_BitrateBps(t *testing.T) {
	cases := []struct {
		name string
		file *library.MediaFile
		want int64
	}{
		{"a stored value wins", &library.MediaFile{Bitrate: 5_000_000, Size: 1, Duration: mustDur(1000)}, 5_000_000},
		{"derived from size and duration", &library.MediaFile{Size: 1_000_000, Duration: mustDur(8000)}, 1_000_000},
		{"no duration, no guess", &library.MediaFile{Size: 1_000_000}, 0},
		{"no size, no guess", &library.MediaFile{Duration: mustDur(8000)}, 0},
		{"nil file is zero, never a panic", nil, 0},
	}
	for _, c := range cases {
		if got := c.file.BitrateBps(); got != c.want {
			t.Errorf("%s: BitrateBps() = %d, want %d", c.name, got, c.want)
		}
	}
}
