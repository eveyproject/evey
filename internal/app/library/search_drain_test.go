package library

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
)

// countingItemRepo wraps the fake query repo to record how many times
// GetByID vs GetByIDs is called, so a test can prove the drain batches the
// re-read instead of issuing one round trip per pending item.
type countingItemRepo struct {
	*libfakes.FakeMediaItemQueryRepo
	getByIDCalls  int
	getByIDsCalls int
}

func (r *countingItemRepo) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	r.getByIDCalls++
	return r.FakeMediaItemQueryRepo.GetByID(ctx, id)
}

func (r *countingItemRepo) GetByIDs(ctx context.Context, ids []shared.MediaItemID) ([]*library.MediaItem, error) {
	r.getByIDsCalls++
	return r.FakeMediaItemQueryRepo.GetByIDs(ctx, ids)
}

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
		items.Add(&library.MediaItem{ID: "m1", Title: "Some Movie"})
		search := libfakes.NewFakeSearchIndex()
		_ = search.IndexMediaItem(ctx, &library.MediaItem{ID: "m2"}) // present, to be removed

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil, nil)
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

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil, nil)
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
		_ = search.IndexMediaItem(ctx, &library.MediaItem{ID: "gone"}) // stale doc lingering

		n, err := DrainSearchWriteQueue(ctx, q, items, search, nil, nil)
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

	t.Run("batches the re-read into a single GetByIDs call regardless of pending count", func(t *testing.T) {
		q := newFakeSearchQueue()
		items := &countingItemRepo{FakeMediaItemQueryRepo: libfakes.NewFakeMediaItemQueryRepo()}
		const n = 50
		for i := range n {
			id := fmt.Sprintf("m%d", i)
			q.ops[id] = library.SearchOpUpsert
			items.Add(&library.MediaItem{ID: shared.MediaItemID(id)})
		}
		search := libfakes.NewFakeSearchIndex()

		got, err := DrainSearchWriteQueue(ctx, q, items, search, nil, nil)
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		if got != n {
			t.Errorf("drained %d, want %d", got, n)
		}
		// The N+1 fix: one GetByIDs call for the whole page, not N GetByID calls.
		// Without the batching, getByIDsCalls stays 0 and getByIDCalls climbs to n.
		if items.getByIDsCalls != 1 {
			t.Errorf("GetByIDs calls = %d, want 1", items.getByIDsCalls)
		}
		if items.getByIDCalls != 0 {
			t.Errorf("GetByID calls = %d, want 0 (should use the batch call instead)", items.getByIDCalls)
		}
		if search.Indexed() != n {
			t.Errorf("indexed=%d, want %d", search.Indexed(), n)
		}
	})

	t.Run("attaches cast names to replayed upserts", func(t *testing.T) {
		q := newFakeSearchQueue()
		q.ops["m1"] = library.SearchOpUpsert
		items := libfakes.NewFakeMediaItemQueryRepo()
		items.Add(&library.MediaItem{ID: "m1", Title: "Some Movie"})
		search := libfakes.NewFakeSearchIndex()
		cast := &staticCastReader{byID: map[string][]string{"m1": {"Some Actor"}}}

		if _, err := DrainSearchWriteQueue(ctx, q, items, search, nil, cast); err != nil {
			t.Fatalf("drain: %v", err)
		}
		got, ok := search.Get("m1")
		if !ok {
			t.Fatal("m1 not indexed")
		}
		if len(got.CastNames) != 1 || got.CastNames[0] != "Some Actor" {
			t.Errorf("replayed doc lost its cast: %v", got.CastNames)
		}
	})
}

// staticCastReader satisfies reindexCastReader with a canned map, to prove the
// drain feeds cast names into the replayed documents.
type staticCastReader struct{ byID map[string][]string }

func (f *staticCastReader) CastNamesByMediaItems(_ context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	for _, id := range ids {
		if names, ok := f.byID[id]; ok {
			out[id] = names
		}
	}
	return out, nil
}
