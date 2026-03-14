package fakes

import (
	"context"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type FakePersonRepository struct {
	mu    sync.RWMutex
	items map[string]*library.Person
	Err   error
}

func NewFakePersonRepository() *FakePersonRepository {
	return &FakePersonRepository{items: make(map[string]*library.Person)}
}

func (r *FakePersonRepository) Create(_ context.Context, person *library.Person) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[person.ID] = person
	return nil
}

func (r *FakePersonRepository) GetByID(_ context.Context, id string) (*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	p, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return p, nil
}

func (r *FakePersonRepository) GetByName(_ context.Context, name string) (*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	for _, p := range r.items {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, ErrNotFound
}

func (r *FakePersonRepository) List(_ context.Context) ([]*library.Person, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make([]*library.Person, 0, len(r.items))
	for _, p := range r.items {
		result = append(result, p)
	}
	return result, nil
}
