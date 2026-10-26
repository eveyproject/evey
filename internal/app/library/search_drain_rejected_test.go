package library

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type rejectedDocs struct{ ids []string }

func (e *rejectedDocs) Error() string         { return "documents rejected" }
func (e *rejectedDocs) RejectedIDs() []string { return e.ids }

// rejectingIndex indexes every document except the ones it refuses, like the
// Meilisearch adapter once it bisected a failed batch.
type rejectingIndex struct {
	*libfakes.FakeSearchIndex
	refuse map[string]bool
}

func (r *rejectingIndex) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	var bad []string
	var good []*library.MediaItem
	for _, it := range items {
		if r.refuse[string(it.ID)] {
			bad = append(bad, string(it.ID))
		} else {
			good = append(good, it)
		}
	}
	_ = r.FakeSearchIndex.IndexMediaItemsBatch(ctx, good)
	if len(bad) > 0 {
		return &rejectedDocs{ids: bad}
	}
	return nil
}

type attemptsQueue struct {
	*fakeSearchQueue
	attempts map[string]int
	dead     map[string]bool
}

func (q *attemptsQueue) List(ctx context.Context, limit int) ([]library.PendingSearchWrite, error) {
	all, _ := q.fakeSearchQueue.List(ctx, limit)
	out := all[:0]
	for _, p := range all {
		if !q.dead[p.MediaItemID] {
			out = append(out, p)
		}
	}
	return out, nil
}

func (q *attemptsQueue) RecordFailure(_ context.Context, id string, maxAttempts int) (bool, error) {
	q.attempts[id]++
	if q.attempts[id] >= maxAttempts {
		q.dead[id] = true
	}
	return q.dead[id], nil
}

func TestDrainSearchWriteQueue_RejectedDocumentIsDeadLetteredNotBlocking(t *testing.T) {
	ctx := context.Background()
	q := &attemptsQueue{fakeSearchQueue: newFakeSearchQueue(), attempts: map[string]int{}, dead: map[string]bool{}}
	items := libfakes.NewFakeMediaItemQueryRepo()
	for _, id := range []string{"good1", "poison", "good2"} {
		q.ops[id] = library.SearchOpUpsert
		items.Add(&library.MediaItem{ID: shared.MediaItemID(id), Title: "Title " + id})
	}
	search := &rejectingIndex{FakeSearchIndex: libfakes.NewFakeSearchIndex(), refuse: map[string]bool{"poison": true}}

	if _, err := DrainSearchWriteQueue(ctx, q, items, search, nil, nil); err != nil {
		t.Fatalf("a refused document must not fail the whole drain: %v", err)
	}
	if _, ok := q.ops["good1"]; ok {
		t.Error("good1 was indexed but is still queued")
	}
	if _, ok := q.ops["good2"]; ok {
		t.Error("good2 was indexed but is still queued")
	}
	if q.attempts["poison"] != 1 {
		t.Fatalf("poison attempts = %d after one drain, want 1", q.attempts["poison"])
	}
	for range maxSearchWriteAttempts {
		_, _ = DrainSearchWriteQueue(ctx, q, items, search, nil, nil)
	}
	if !q.dead["poison"] {
		t.Fatalf("poison not dead-lettered after %d attempts", q.attempts["poison"])
	}
	if q.attempts["poison"] != maxSearchWriteAttempts {
		t.Fatalf("poison retried %d times, want %d", q.attempts["poison"], maxSearchWriteAttempts)
	}
}
