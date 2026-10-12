package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/eveyproject/evey/internal/domain/shared"
)

var ErrNotFound = shared.ErrNotFound

type FakeContainerReader struct {
	Containers map[string]*mkv.Container
	Err        error
}

func NewFakeContainerReader() *FakeContainerReader {
	return &FakeContainerReader{Containers: make(map[string]*mkv.Container)}
}

func (r *FakeContainerReader) Open(_ context.Context, path string) (*mkv.Container, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	c, ok := r.Containers[path]
	if !ok {
		return nil, ErrNotFound
	}
	return c, nil
}

func (r *FakeContainerReader) ReadTracks(_ context.Context, path string) ([]mkv.Track, error) {
	c, ok := r.Containers[path]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Tracks, nil
}

func (r *FakeContainerReader) ReadChapters(_ context.Context, path string) ([]mkv.Chapter, error) {
	c, ok := r.Containers[path]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Chapters, nil
}

func (r *FakeContainerReader) ReadAttachments(_ context.Context, path string) ([]mkv.Attachment, error) {
	c, ok := r.Containers[path]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Attachments, nil
}

func (r *FakeContainerReader) ReadTags(_ context.Context, path string) ([]mkv.Tag, error) {
	c, ok := r.Containers[path]
	if !ok {
		return nil, ErrNotFound
	}
	return c.Tags, nil
}
