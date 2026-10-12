package fakes

import (
	"context"
	"sync"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

type FakeCacheStore struct {
	mu    sync.RWMutex
	files map[string]transcoding.CacheFileInfo
	Err   error
}

func NewFakeCacheStore() *FakeCacheStore {
	return &FakeCacheStore{files: make(map[string]transcoding.CacheFileInfo)}
}

func (s *FakeCacheStore) AddFile(f transcoding.CacheFileInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[f.Name] = f
}

func (s *FakeCacheStore) GetCacheInfo(_ context.Context) (*transcoding.CacheInfo, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	info := &transcoding.CacheInfo{}
	for _, f := range s.files {
		info.Files = append(info.Files, f)
		info.TotalSize += f.Size
		info.FileCount++
	}
	return info, nil
}

func (s *FakeCacheStore) ClearCache(_ context.Context) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = make(map[string]transcoding.CacheFileInfo)
	return nil
}

func (s *FakeCacheStore) DeleteFile(_ context.Context, filename string) error {
	if s.Err != nil {
		return s.Err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.files[filename]; !ok {
		return transcoding.ErrCacheFileNotFound
	}
	delete(s.files, filename)
	return nil
}
