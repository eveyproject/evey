package photo

import (
	"context"

	"github.com/evey-project/evey/internal/domain/photo"
)

type QueryService struct {
	repo photo.Repository
}

func NewQueryService(repo photo.Repository) *QueryService {
	return &QueryService{repo: repo}
}

func (s *QueryService) ListPhotos(ctx context.Context, libraryID string, limit, offset int) ([]photo.Photo, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.repo.ListByLibrary(ctx, libraryID, limit, offset)
}

func (s *QueryService) GetAlbums(ctx context.Context, libraryID string) ([]photo.Album, error) {
	return s.repo.GetAlbums(ctx, libraryID)
}

func (s *QueryService) GetPhoto(ctx context.Context, id string) (*photo.Photo, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *QueryService) Search(ctx context.Context, libraryID, query string, limit int) ([]photo.Photo, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.Search(ctx, libraryID, query, limit)
}
