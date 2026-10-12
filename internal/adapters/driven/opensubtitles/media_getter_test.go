package opensubtitles

import (
	"context"
	"errors"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type fakeLibQuerySvc struct {
	items  map[string]*domainlibrary.MediaItem
	getErr error
}

func (f *fakeLibQuerySvc) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.items[id], nil
}

func TestLibraryMediaGetter_GetMediaItem_Success(t *testing.T) {
	q := &fakeLibQuerySvc{items: map[string]*domainlibrary.MediaItem{
		"mi-1": {ID: shared.MediaItemID("mi-1"), File: &domainlibrary.MediaFile{Path: "/media/movie.mkv"}, Title: "Movie"},
	}}
	g := NewLibraryMediaGetter(q)

	info, err := g.GetMediaItem(context.Background(), "mi-1")
	if err != nil {
		t.Fatalf("GetMediaItem: %v", err)
	}
	if info.ID != "mi-1" || info.FilePath != "/media/movie.mkv" || info.Title != "Movie" {
		t.Errorf("info = %+v", info)
	}
}

func TestLibraryMediaGetter_GetMediaItem_Error(t *testing.T) {
	q := &fakeLibQuerySvc{getErr: errors.New("db offline")}
	g := NewLibraryMediaGetter(q)

	_, err := g.GetMediaItem(context.Background(), "mi-x")
	if err == nil {
		t.Fatal("expected error propagated from query svc")
	}
}
