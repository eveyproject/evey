package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
)

// FakeCollectionRepository combines command and query behaviour for tests that need both.
type FakeCollectionRepository struct {
	cmd   *FakeCollectionCommandRepo
	query *FakeCollectionQueryRepo
}

func NewFakeCollectionRepository(mediaItems *FakeMediaItemRepository) *FakeCollectionRepository {
	cmd := NewFakeCollectionCommandRepo()
	qry := NewFakeCollectionQueryRepo(cmd, mediaItems)
	return &FakeCollectionRepository{cmd: cmd, query: qry}
}

func (r *FakeCollectionRepository) CommandRepo() *FakeCollectionCommandRepo { return r.cmd }
func (r *FakeCollectionRepository) QueryRepo() *FakeCollectionQueryRepo     { return r.query }

func (r *FakeCollectionRepository) Create(ctx context.Context, col *library.Collection) error {
	return r.cmd.Create(ctx, col)
}
func (r *FakeCollectionRepository) Upsert(ctx context.Context, col *library.Collection) error {
	return r.cmd.Upsert(ctx, col)
}
func (r *FakeCollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	return r.cmd.Update(ctx, col)
}
func (r *FakeCollectionRepository) Delete(ctx context.Context, id string) error {
	return r.cmd.Delete(ctx, id)
}
func (r *FakeCollectionRepository) LinkItem(ctx context.Context, collectionID, mediaItemID string, sortOrder int) error {
	return r.cmd.LinkItem(ctx, collectionID, mediaItemID, sortOrder)
}
func (r *FakeCollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return r.query.GetByID(ctx, id)
}
func (r *FakeCollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	return r.query.List(ctx)
}
func (r *FakeCollectionRepository) GetItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	return r.query.GetItems(ctx, collectionID, limit, offset)
}
func (r *FakeCollectionRepository) CountItems(ctx context.Context, collectionID string) (int, error) {
	return r.query.CountItems(ctx, collectionID)
}
func (r *FakeCollectionRepository) GetItemsByCollectionIDs(ctx context.Context, collectionIDs []string) (map[string][]*library.MediaItem, error) {
	return r.query.GetItemsByCollectionIDs(ctx, collectionIDs)
}
func (r *FakeCollectionRepository) ListMoviesWithoutCollection(ctx context.Context, afterID string, limit int) ([]library.UnlinkedMovie, error) {
	return r.query.ListMoviesWithoutCollection(ctx, afterID, limit)
}
func (r *FakeCollectionRepository) UnlinkItem(ctx context.Context, collectionID, mediaItemID string) error {
	return r.cmd.UnlinkItem(ctx, collectionID, mediaItemID)
}
func (r *FakeCollectionRepository) SetItemOrder(ctx context.Context, collectionID string, mediaItemIDs []string) error {
	return r.cmd.SetItemOrder(ctx, collectionID, mediaItemIDs)
}
func (r *FakeCollectionRepository) GetByMediaItemID(ctx context.Context, mediaItemID string) ([]*library.Collection, error) {
	return r.query.GetByMediaItemID(ctx, mediaItemID)
}
