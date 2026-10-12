package fakes

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
)

type FakeFileScanner struct {
	Results map[string][]library.ScannedFile
	// ParsedTitles lets a test pre-seed the title the scanner would
	// derive from a given file path - used by the orphan title-heal pass.
	// Keys are file paths, values are the parsed title. Missing keys fall
	// back to "" (which the heal pass treats as "no signal, skip item").
	ParsedTitles map[string]string
	// ExistingPaths marks which paths FileExists reports as present. Default
	// (nil) → false, so a fingerprint match is treated as a genuine move (old
	// file gone); a test sets a path true to simulate a size+mtime collision.
	ExistingPaths map[string]bool
	Err           error
}

func NewFakeFileScanner() *FakeFileScanner {
	return &FakeFileScanner{
		Results:      make(map[string][]library.ScannedFile),
		ParsedTitles: make(map[string]string),
	}
}

func (s *FakeFileScanner) Scan(_ context.Context, path string) ([]library.ScannedFile, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Results[path], nil
}

func (s *FakeFileScanner) ParseTitle(path string) string {
	return s.ParsedTitles[path]
}

func (s *FakeFileScanner) FileExists(path string) bool {
	return s.ExistingPaths[path]
}
