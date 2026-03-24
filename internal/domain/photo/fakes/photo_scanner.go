package fakes

import (
	"context"

	"github.com/gravity-zero/evey/internal/domain/photo"
)

type FakePhotoScanner struct {
	Files      []photo.ScannedImageFile
	WalkErr    error
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

func (f *FakePhotoScanner) WalkImageFiles(_ context.Context, _ string, fn func(photo.ScannedImageFile) error) error {
	if f.WalkErr != nil {
		return f.WalkErr
	}
	for _, file := range f.Files {
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
