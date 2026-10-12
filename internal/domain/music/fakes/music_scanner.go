package fakes

import (
	"bytes"
	"context"

	"github.com/eveyproject/evey/internal/domain/music"
)

type FakeMusicScanner struct {
	Files   []music.ScannedAudioFile
	WalkErr error
	// ExistingPaths overrides Exists when non-nil; otherwise a path exists
	// when the walk would report it (present in Files) or has data.
	ExistingPaths map[string]bool
	FileData      map[string][]byte
	OpenErr       error
	Hashes        map[string]string
	HashErr       error
}

func NewFakeMusicScanner() *FakeMusicScanner {
	return &FakeMusicScanner{
		FileData: make(map[string][]byte),
		Hashes:   make(map[string]string),
	}
}

func (f *FakeMusicScanner) WalkAudioFiles(ctx context.Context, _ string, fn func(music.ScannedAudioFile) error) error {
	if f.WalkErr != nil {
		return f.WalkErr
	}
	for _, file := range f.Files {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(file); err != nil {
			return err
		}
	}
	return nil
}

type nopReadSeekCloser struct {
	*bytes.Reader
}

func (n *nopReadSeekCloser) Close() error { return nil }

func (f *FakeMusicScanner) OpenFile(path string) (music.ReadSeekCloser, error) {
	if f.OpenErr != nil {
		return nil, f.OpenErr
	}
	data, ok := f.FileData[path]
	if !ok {
		return nil, &fakeNotFoundErr{path: path}
	}
	return &nopReadSeekCloser{bytes.NewReader(data)}, nil
}

type fakeNotFoundErr struct{ path string }

func (e *fakeNotFoundErr) Error() string { return "file not found: " + e.path }

func (f *FakeMusicScanner) ComputeHash(path string) (string, error) {
	if f.HashErr != nil {
		return "", f.HashErr
	}
	return f.Hashes[path], nil
}

func (f *FakeMusicScanner) Exists(path string) bool {
	if f.ExistingPaths != nil {
		return f.ExistingPaths[path]
	}
	for _, file := range f.Files {
		if file.Path == path {
			return true
		}
	}
	_, ok := f.FileData[path]
	return ok
}
