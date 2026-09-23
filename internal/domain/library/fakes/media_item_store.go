package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
)

// FakeMediaItemStore is the shared media item store for tests of services that
// read an item and write it back (metadata enrichment, collections). Every read
// is FakeMediaItemQueryRepo's own, on the same map that Create seeds and Update
// persists into, so a test sees what the service wrote.
type FakeMediaItemStore struct {
	*FakeMediaItemQueryRepo
	ErrUpdate error
}

func NewFakeMediaItemStore() *FakeMediaItemStore {
	return &FakeMediaItemStore{FakeMediaItemQueryRepo: NewFakeMediaItemQueryRepo()}
}

// Create seeds an item, as the scanner would have inserted it.
func (s *FakeMediaItemStore) Create(_ context.Context, item *library.MediaItem) error {
	s.Add(item)
	return nil
}

func (s *FakeMediaItemStore) Update(_ context.Context, item *library.MediaItem) error {
	if s.ErrUpdate != nil {
		return s.ErrUpdate
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[string(item.ID)]; !ok {
		return ErrNotFound
	}
	s.items[string(item.ID)] = cloneMediaItem(item)
	return nil
}

// cloneMediaItem detaches the stored record from the caller's pointer. A fake
// that hands back the same pointer it stores makes persistence untestable: a
// service that loads an item, mutates it and never calls Update is
// indistinguishable from one that persists. A real repository serialises, so
// the fake must too.
//
// Shallow at the top level plus the mutable sub-structs services actually
// reach into (File, Catalog) - deep enough to break the aliasing that hid the
// bug, without pretending to be a full deep copy.
func cloneMediaItem(item *library.MediaItem) *library.MediaItem {
	if item == nil {
		return nil
	}
	cp := *item
	if item.File != nil {
		f := *item.File
		cp.File = &f
	}
	if item.Catalog != nil {
		c := *item.Catalog
		cp.Catalog = &c
	}
	return &cp
}
