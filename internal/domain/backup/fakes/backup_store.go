package fakes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/gravity-zero/evey/internal/domain/backup"
)

type FakeBackupStore struct {
	mu        sync.RWMutex
	manifests map[string]*backup.BackupManifest
	data      map[string][]byte
	Err       error
}

func NewFakeBackupStore() *FakeBackupStore {
	return &FakeBackupStore{
		manifests: make(map[string]*backup.BackupManifest),
		data:      make(map[string][]byte),
	}
}

func (s *FakeBackupStore) Create(_ context.Context, m *backup.BackupManifest, data io.Reader) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.Size = int64(len(b))
	s.manifests[m.ID] = m
	s.data[m.ID] = b
	return nil
}

func (s *FakeBackupStore) List(_ context.Context) ([]*backup.BackupManifest, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Err != nil {
		return nil, s.Err
	}
	var result []*backup.BackupManifest
	for _, m := range s.manifests {
		result = append(result, m)
	}
	return result, nil
}

func (s *FakeBackupStore) Get(_ context.Context, id string) (*backup.BackupManifest, io.ReadCloser, error) {
	if s.Err != nil {
		return nil, nil, s.Err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Err != nil {
		return nil, nil, s.Err
	}
	m, ok := s.manifests[id]
	if !ok {
		return nil, nil, fmt.Errorf("backup not found")
	}
	return m, io.NopCloser(bytes.NewReader(s.data[id])), nil
}

func (s *FakeBackupStore) Delete(_ context.Context, id string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	if _, ok := s.manifests[id]; !ok {
		return fmt.Errorf("backup not found")
	}
	delete(s.manifests, id)
	delete(s.data, id)
	return nil
}

// SetCreatedAt backdates a manifest for testing retention logic.
func (s *FakeBackupStore) SetCreatedAt(id string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.manifests[id]; ok {
		m.CreatedAt = t
	}
}
