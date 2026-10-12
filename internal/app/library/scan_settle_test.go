package library

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/eveyproject/evey/internal/domain/library"
)

func TestSplitSettled_LeavesFilesStillBeingCopied(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 49, 48, 0, time.UTC)
	files := []library.ScannedFile{
		{Path: "/s/copying.mkv", ModTime: now.Add(-3 * time.Second)},
		{Path: "/s/done.mkv", ModTime: now.Add(-10 * time.Minute)},
		{Path: "/s/skewed.mkv", ModTime: now.Add(48 * time.Hour)},
		{Path: "/s/unknown.mkv"},
	}

	s := &ScanService{}
	got, n := s.splitSettled(files, now)
	assert.Len(t, got, 4, "no window configured: nothing waits")
	assert.Zero(t, n)

	s.WithFileSettle(time.Minute)
	got, n = s.splitSettled(files, now)
	assert.Equal(t, 1, n)
	paths := make([]string, 0, len(got))
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	assert.Equal(t, []string{"/s/done.mkv", "/s/skewed.mkv", "/s/unknown.mkv"}, paths)
}
