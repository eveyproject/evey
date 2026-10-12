package library_test

import (
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
)

func TestNewMediaItem_HappyPath(t *testing.T) {
	t.Parallel()

	m, err := library.NewMediaItem("lib-1", library.MovieItem, "Big Buck Bunny")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.LibraryID != "lib-1" {
		t.Errorf("LibraryID = %q, want %q", m.LibraryID, "lib-1")
	}
	if m.Type != library.MovieItem {
		t.Errorf("Type = %q, want %q", m.Type, library.MovieItem)
	}
	if m.Title != "Big Buck Bunny" {
		t.Errorf("Title = %q, want %q", m.Title, "Big Buck Bunny")
	}
	if m.ScanTitle != "Big Buck Bunny" {
		t.Errorf("ScanTitle should default to Title; got %q", m.ScanTitle)
	}
	if m.AddedAt.IsZero() || m.UpdatedAt.IsZero() {
		t.Error("expected AddedAt/UpdatedAt to be stamped")
	}
}

func TestNewMediaItem_TrimsTitle(t *testing.T) {
	t.Parallel()

	m, err := library.NewMediaItem("lib-1", library.MovieItem, "   Spaced Out   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Title != "Spaced Out" {
		t.Errorf("Title = %q, want %q (trimmed)", m.Title, "Spaced Out")
	}
}

func TestNewMediaItem_InvalidLibraryID(t *testing.T) {
	t.Parallel()

	_, err := library.NewMediaItem("", library.MovieItem, "Movie")
	if !errors.Is(err, library.ErrInvalidLibraryID) {
		t.Fatalf("err = %v, want ErrInvalidLibraryID", err)
	}
}

func TestNewMediaItem_InvalidType(t *testing.T) {
	t.Parallel()

	_, err := library.NewMediaItem("lib-1", "banana", "Movie")
	if !errors.Is(err, library.ErrInvalidMediaItemType) {
		t.Fatalf("err = %v, want ErrInvalidMediaItemType", err)
	}

	_, err = library.NewMediaItem("lib-1", "", "Movie")
	if !errors.Is(err, library.ErrInvalidMediaItemType) {
		t.Fatalf("err = %v, want ErrInvalidMediaItemType (empty type)", err)
	}
}

func TestNewMediaItem_EmptyTitle(t *testing.T) {
	t.Parallel()

	cases := []string{"", "   ", "\t\n"}
	for _, title := range cases {
		_, err := library.NewMediaItem("lib-1", library.MovieItem, title)
		if !errors.Is(err, library.ErrEmptyTitle) {
			t.Errorf("title=%q: err = %v, want ErrEmptyTitle", title, err)
		}
	}
}

func TestNewMediaItem_EpisodeMissingParent(t *testing.T) {
	t.Parallel()

	_, err := library.NewMediaItem("lib-1", library.Episode, "S01E01")
	if !errors.Is(err, library.ErrMissingParent) {
		t.Fatalf("err = %v, want ErrMissingParent", err)
	}
}

func TestNewMediaItem_SeasonMissingParent(t *testing.T) {
	t.Parallel()

	_, err := library.NewMediaItem("lib-1", library.Season, "Season 1")
	if !errors.Is(err, library.ErrMissingParent) {
		t.Fatalf("err = %v, want ErrMissingParent", err)
	}
}

func TestNewMediaItem_EpisodeWithParent(t *testing.T) {
	t.Parallel()

	m, err := library.NewMediaItem("lib-1", library.Episode, "Pilot",
		library.WithMediaItemParent("series-42"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ParentID != "series-42" {
		t.Errorf("ParentID = %q, want %q", m.ParentID, "series-42")
	}
}

func TestNewMediaItem_SeasonWithParent(t *testing.T) {
	t.Parallel()

	m, err := library.NewMediaItem("lib-1", library.Season, "Season 1",
		library.WithMediaItemParent("series-42"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ParentID != "series-42" {
		t.Errorf("ParentID = %q, want %q", m.ParentID, "series-42")
	}
}

func TestNewMediaItem_WithYear(t *testing.T) {
	t.Parallel()

	m, err := library.NewMediaItem("lib-1", library.MovieItem, "Some Movie",
		library.WithMediaItemYear(2010),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Catalog == nil {
		t.Fatal("Catalog should be allocated by WithMediaItemYear")
	}
	if m.Catalog.Year == nil || *m.Catalog.Year != 2010 {
		t.Errorf("Catalog.Year = %v, want 2010", m.Catalog.Year)
	}
}

func TestNewMediaItem_WithOverview(t *testing.T) {
	t.Parallel()

	overview := "A group pulls off a daring heist..."
	m, err := library.NewMediaItem("lib-1", library.MovieItem, "Some Movie",
		library.WithMediaItemOverview(overview),
		library.WithMediaItemTagline("Nothing is as it seems."),
		library.WithMediaItemSortTitle("Some Movie"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Overview != overview {
		t.Errorf("Overview = %q, want %q", m.Overview, overview)
	}
	if m.Tagline != "Nothing is as it seems." {
		t.Errorf("Tagline not applied: %q", m.Tagline)
	}
	if m.SortTitle != "Some Movie" {
		t.Errorf("SortTitle not applied: %q", m.SortTitle)
	}
}

func TestNewMediaItem_WithFileAndID(t *testing.T) {
	t.Parallel()

	file := &library.MediaFile{Path: "/data/movies/some-movie.mkv", Size: 12345}
	m, err := library.NewMediaItem("lib-1", library.MovieItem, "Some Movie",
		library.WithMediaItemID("mi-1"),
		library.WithMediaItemFile(file),
		library.WithMediaItemIndexNumber(1),
		library.WithMediaItemScanTitle("Some Movie (2010)"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "mi-1" {
		t.Errorf("ID = %q, want %q", m.ID, "mi-1")
	}
	if !m.HasFile() {
		t.Error("HasFile() should be true after WithMediaItemFile")
	}
	if m.IndexNumber == nil || *m.IndexNumber != 1 {
		t.Errorf("IndexNumber = %v, want 1", m.IndexNumber)
	}
	if m.ScanTitle != "Some Movie (2010)" {
		t.Errorf("ScanTitle = %q, want override", m.ScanTitle)
	}
}
