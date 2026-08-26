package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/photo"
)

type FakePhotoScanner struct {
	Files      []photo.ScannedImageFile
	WalkErr    error
	// ExistingPaths overrides Exists when non-nil; otherwise a path exists
	// when the walk would report it (present in Files) or has data.
	ExistingPaths map[string]bool
	FileData   map[string][]byte
	ReadErr    error
	Dimensions map[string][2]int
	Hashes     map[string]string
	HashErr    error
}

func NewFakePhotoScanner() *FakePhotoScanner {
	return &FakePhotoScanner{
		FileData:   make(map[string][]byte),
		Dimensions: make(map[string][2]int),
		Hashes:     make(map[string]string),
	}
}

func (f *FakePhotoScanner) WalkImageFiles(ctx context.Context, _ string, fn func(photo.ScannedImageFile) error) error {
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

func (f *FakePhotoScanner) ReadFile(path string) ([]byte, error) {
	if f.ReadErr != nil {
		return nil, f.ReadErr
	}
	data, ok := f.FileData[path]
	if !ok {
		return nil, nil
	}
	return data, nil
}

func (f *FakePhotoScanner) ImageDimensions(path string) (int, int) {
	d, ok := f.Dimensions[path]
	if !ok {
		return 0, 0
	}
	return d[0], d[1]
}

func (f *FakePhotoScanner) ComputeHash(path string) (string, error) {
	if f.HashErr != nil {
		return "", f.HashErr
	}
	return f.Hashes[path], nil
}

func (f *FakePhotoScanner) Exists(path string) bool {
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
