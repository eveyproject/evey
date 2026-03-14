package photo

import "context"

type Repository interface {
	Upsert(ctx context.Context, p Photo) error
	GetByID(ctx context.Context, id string) (*Photo, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID string, limit, offset int) ([]Photo, int, error)
	ListByAlbum(ctx context.Context, libraryID, album string, limit, offset int) ([]Photo, int, error)
	GetAlbums(ctx context.Context, libraryID string) ([]Album, error)
	Search(ctx context.Context, libraryID, query string, limit int) ([]Photo, error)
	CountByLibrary(ctx context.Context, libraryID string) (int, error)
}
