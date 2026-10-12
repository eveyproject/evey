package photo

import (
	"context"
	"errors"

	"github.com/eveyproject/evey/internal/domain/photo"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type QueryService struct {
	repo photo.QueryRepo
}

// NewQueryService takes only the READ half of the store (ADR-0009): a query
// service that could write would be a category error the compiler now rejects.
func NewQueryService(repo photo.QueryRepo) *QueryService {
	return &QueryService{repo: repo}
}

// CountPhotos returns the library's photo count without reading its rows.
func (s *QueryService) CountPhotos(ctx context.Context, libraryID shared.LibraryID) (int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.CountPhotos")
	defer span.End()
	return s.repo.CountByLibrary(ctx, libraryID)
}

func (s *QueryService) ListPhotos(ctx context.Context, libraryID shared.LibraryID, limit, offset int) ([]photo.Photo, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.ListPhotos")
	defer span.End()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	return s.repo.ListByLibrary(ctx, libraryID, limit, offset)
}

func (s *QueryService) ListPhotosByAlbum(ctx context.Context, libraryID shared.LibraryID, album string, limit, offset int) ([]photo.Photo, int, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.ListPhotosByAlbum")
	defer span.End()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	return s.repo.ListByAlbum(ctx, libraryID, album, limit, offset)
}

// ListPhotosCursor is the keyset form of ListPhotos: no total, page forward
// only via the cursor of the last item returned. nil cursor = first page.
func (s *QueryService) ListPhotosCursor(ctx context.Context, libraryID shared.LibraryID, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.ListPhotosCursor")
	defer span.End()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	return s.repo.ListByLibraryCursor(ctx, libraryID, cursor, limit)
}

// ListPhotosByAlbumCursor is the keyset form of ListPhotosByAlbum. See ListPhotosCursor.
func (s *QueryService) ListPhotosByAlbumCursor(ctx context.Context, libraryID shared.LibraryID, album string, cursor *photo.PhotoCursor, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.ListPhotosByAlbumCursor")
	defer span.End()
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	return s.repo.ListByAlbumCursor(ctx, libraryID, album, cursor, limit)
}

func (s *QueryService) GetAlbums(ctx context.Context, libraryID shared.LibraryID) ([]photo.Album, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.GetAlbums")
	defer span.End()
	return s.repo.GetAlbums(ctx, libraryID)
}

func (s *QueryService) GetPhoto(ctx context.Context, id string) (*photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.GetPhoto")
	defer span.End()
	p, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, shared.ErrNotFound) {
		return nil, nil
	}
	return p, err
}

func (s *QueryService) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]photo.Photo, error) {
	ctx, span := tracing.StartSpan(ctx, "app.photo.Search")
	defer span.End()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.Search(ctx, libraryID, query, limit)
}
