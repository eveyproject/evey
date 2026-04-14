package fakes

import (
	"context"

	"github.com/evey-project/evey/internal/domain/library"
)

type FakeFileScanner struct {
	Results map[string][]library.ScannedFile
	Err     error
}

func NewFakeFileScanner() *FakeFileScanner {
	return &FakeFileScanner{Results: make(map[string][]library.ScannedFile)}
}

func (s *FakeFileScanner) Scan(_ context.Context, path string) ([]library.ScannedFile, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Results[path], nil
}
