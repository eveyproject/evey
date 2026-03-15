package music

import "context"

type TrackRepository interface {
	Upsert(ctx context.Context, t Track) error
	GetByID(ctx context.Context, id string) (*Track, error)
	Delete(ctx context.Context, id string) error
	ListByLibrary(ctx context.Context, libraryID string, limit, offset int, search string) ([]Track, int, error)
	ListByAlbum(ctx context.Context, album, artist string) ([]Track, error)
	ListByArtist(ctx context.Context, artist string, limit, offset int) ([]Track, int, error)
	Search(ctx context.Context, query string, limit int) ([]Track, error)
	GetAlbums(ctx context.Context, libraryID string) ([]Album, error)
	GetArtists(ctx context.Context, libraryID string) ([]Artist, error)
	CountByLibrary(ctx context.Context, libraryID string) (int, error)
}
