package library

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
)

type fakeSearchQueue struct {
	ops map[string]string // media_item_id -> op
}

func newFakeSearchQueue() *fakeSearchQueue { return &fakeSearchQueue{ops: map[string]string{}} }

func (q *fakeSearchQueue) Enqueue(_ context.Context, id, op string) error { q.ops[id] = op; return nil }
func (q *fakeSearchQueue) List(_ context.Context, _ int) ([]library.PendingSearchWrite, error) {
	out := make([]library.PendingSearchWrite, 0, len(q.ops))
	for id, op := range q.ops {
		out = append(out, library.PendingSearchWrite{MediaItemID: id, Op: op})
	}
	return out, nil
}
func (q *fakeSearchQueue) Delete(_ context.Context, id string) error { delete(q.ops, id); return nil }

func TestDrainSearchWriteQueue(t *testing.T) {
	ctx := context.Background()

	t.Run("replays upserts + deletes then clears the queue", func(t *testing.T) {
		q := newFakeSearchQueue()
		q.ops["m1"] = library.SearchOpUpsert
		q.ops["m2"] = library.SearchOpDelete
		items := libfakes.NewFakeMediaItemQueryRepo()
		items.Add(&library.MediaItem{ID: "m1", Title: "Avatar"})
		search := libfakes.NewFakeSearchIndex()
		_ = search.IndexMediaItem(ctx, &library.MediaItem{ID: "m2"}, nil) // present, to be removed

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil)
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if n != 2 {
			t.Errorf("drained %d, want 2", n)
		}
		if len(q.ops) != 0 {
			t.Errorf("queue not cleared: %v", q.ops)
		}
		if search.Indexed() != 1 { // m1 indexed, m2 removed
			t.Errorf("indexed=%d, want 1 (m1 kept, m2 removed)", search.Indexed())
		}
	})

	t.Run("stops and preserves the queue when the engine is still down", func(t *testing.T) {
		q := newFakeSearchQueue()
		q.ops["m1"] = library.SearchOpUpsert
		items := libfakes.NewFakeMediaItemQueryRepo()
		items.Add(&library.MediaItem{ID: "m1"})
		search := libfakes.NewFakeSearchIndex()
		search.Err = errors.New("meili down")

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil)
		if err == nil {
			t.Fatal("expected an error while the engine is down")
		}
		if n != 0 {
			t.Errorf("drained %d, want 0", n)
		}
		if len(q.ops) != 1 {
			t.Errorf("queue must be preserved for the next tick, got %v", q.ops)
		}
	})

	t.Run("upsert of an item deleted since enqueue evicts the stale doc and drops the op", func(t *testing.T) {
		q := newFakeSearchQueue()
		q.ops["gone"] = library.SearchOpUpsert
		items := libfakes.NewFakeMediaItemQueryRepo() // "gone" absent → ErrNotFound
		search := libfakes.NewFakeSearchIndex()
		_ = search.IndexMediaItem(ctx, &library.MediaItem{ID: "gone"}, nil) // stale doc lingering

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil)
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if n != 1 {
			t.Errorf("drained %d, want 1", n)
		}
		if len(q.ops) != 0 {
			t.Errorf("queue not cleared: %v", q.ops)
		}
		if search.Indexed() != 0 {
			t.Errorf("stale doc not evicted, indexed=%d", search.Indexed())
		}
	})
}
