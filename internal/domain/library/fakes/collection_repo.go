package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

// FakeCollectionRepository combines command and query behaviour for tests that need both.
type FakeCollectionRepository struct {
	cmd   *FakeCollectionCommandRepo
	query *FakeCollectionQueryRepo
	mu    sync.RWMutex
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
func (r *FakeCollectionRepository) Update(ctx context.Context, col *library.Collection) error {
	return r.cmd.Update(ctx, col)
}
func (r *FakeCollectionRepository) Delete(ctx context.Context, id string) error {
	return r.cmd.Delete(ctx, id)
}
func (r *FakeCollectionRepository) AddItem(ctx context.Context, collectionID, mediaItemID string) error {
	return r.cmd.AddItem(ctx, collectionID, mediaItemID)
}
func (r *FakeCollectionRepository) RemoveItem(ctx context.Context, collectionID, mediaItemID string) error {
	return r.cmd.RemoveItem(ctx, collectionID, mediaItemID)
}
func (r *FakeCollectionRepository) GetByID(ctx context.Context, id string) (*library.Collection, error) {
	return r.query.GetByID(ctx, id)
}
func (r *FakeCollectionRepository) List(ctx context.Context) ([]*library.Collection, error) {
	return r.query.List(ctx)
}
func (r *FakeCollectionRepository) GetSmartItems(ctx context.Context, col *library.Collection, limit, offset int) ([]*library.MediaItem, error) {
	return r.query.GetSmartItems(ctx, col, limit, offset)
}
func (r *FakeCollectionRepository) CountSmartItems(ctx context.Context, col *library.Collection) (int, error) {
	return r.query.CountSmartItems(ctx, col)
}
func (r *FakeCollectionRepository) GetManualItems(ctx context.Context, collectionID string, limit, offset int) ([]*library.MediaItem, error) {
	return r.query.GetManualItems(ctx, collectionID, limit, offset)
}
func (r *FakeCollectionRepository) CountManualItems(ctx context.Context, collectionID string) (int, error) {
	return r.query.CountManualItems(ctx, collectionID)
}
