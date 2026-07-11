package fakes

import (
	"context"
	"sync"
)

type FakeArtworkDownloader struct {
	mu        sync.Mutex
	Downloads map[string]string
	Err       error
}

func NewFakeArtworkDownloader() *FakeArtworkDownloader {
	return &FakeArtworkDownloader{Downloads: make(map[string]string)}
}

func (d *FakeArtworkDownloader) Download(_ context.Context, url, destPath string) error {
	if d.Err != nil {
		return d.Err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Downloads[url] = destPath
	return nil
}
