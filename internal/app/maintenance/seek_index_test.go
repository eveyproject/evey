package maintenance_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	appmaintenance "github.com/evey-project/evey/internal/app/maintenance"
	"github.com/evey-project/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReindexer is a state-machine fake of mkv.Reindexer: HasSeekIndex/HasBackup
// answer from canned maps keyed by path, and the mutating calls record what they
// were asked to do. ReindexInPlace can be gated on a channel to hold a batch
// open deterministically (for the concurrency-guard test).
type fakeReindexer struct {
	mu        sync.Mutex
	hasIndex  map[string]bool
	hasBackup map[string]bool
	reindexed []string
	restored  []string
	gate      chan struct{} // when non-nil, ReindexInPlace blocks until it's closed
}

func (f *fakeReindexer) HasSeekIndex(_ context.Context, p string) (bool, error) {
	return f.hasIndex[p], nil
}
func (f *fakeReindexer) HasBackup(_ context.Context, p string) (bool, error) {
	return f.hasBackup[p], nil
}
func (f *fakeReindexer) ReindexInPlace(_ context.Context, p string, _ mkv.ReindexOptions) (mkv.ReindexResult, error) {
	if f.gate != nil {
		<-f.gate
	}
	f.mu.Lock()
	f.reindexed = append(f.reindexed, p)
	f.mu.Unlock()
	return mkv.ReindexResult{BackupPath: p + ".mkvgobak", SrcBytes: 100, DstBytes: 90}, nil
}
func (f *fakeReindexer) RestoreBackup(_ context.Context, p string) error {
	f.mu.Lock()
	f.restored = append(f.restored, p)
	f.mu.Unlock()
	return nil
}

type fakeCatalog struct {
	files []appmaintenance.MediaRef
}

func (c fakeCatalog) ListMediaFiles(_ context.Context, _ string) ([]appmaintenance.MediaRef, error) {
	return c.files, nil
}
func (c fakeCatalog) GetMediaFile(_ context.Context, mediaID string) (appmaintenance.MediaRef, error) {
	for _, f := range c.files {
		if f.MediaID == mediaID {
			return f, nil
		}
	}
	return appmaintenance.MediaRef{}, os.ErrNotExist
}

// Scan inspects only EBML files that exist on disk, and rolls up the missing-
// index / has-backup counts correctly while skipping non-EBML and absent files.
func TestSeekIndexService_Scan(t *testing.T) {
	dir := t.TempDir()
	indexed := filepath.Join(dir, "indexed.mkv")
	missing := filepath.Join(dir, "missing.mkv") // exists on disk, no Cues, has backup
	notEBML := filepath.Join(dir, "movie.mp4")   // not an EBML container → skipped
	absent := filepath.Join(dir, "ghost.mkv")    // recorded in DB but not on disk → skipped
	for _, p := range []string{indexed, missing, notEBML} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}

	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{indexed: true, missing: false},
		hasBackup: map[string]bool{missing: true},
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Title: "Indexed", Path: indexed},
		{MediaID: "m2", Title: "Missing", Path: missing},
		{MediaID: "m3", Title: "Mp4", Path: notEBML},
		{MediaID: "m4", Title: "Ghost", Path: absent},
		{MediaID: "m5", Title: "NoFile", Path: ""},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	res, err := svc.Scan(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total, "only the two on-disk EBML files are counted")
	assert.Equal(t, 1, res.MissingIndex, "exactly the un-indexed file")
	assert.Equal(t, 1, res.WithBackup, "exactly the file with a .mkvgobak")
	require.Len(t, res.Items, 2)
	byID := map[string]appmaintenance.SeekStatus{}
	for _, it := range res.Items {
		byID[it.MediaID] = it
	}
	assert.True(t, byID["m1"].HasIndex)
	assert.Equal(t, "mkv", byID["m1"].Container)
	assert.False(t, byID["m2"].HasIndex)
	assert.True(t, byID["m2"].HasBackup)
}

func TestSeekIndexService_ReindexMedia_KeepsBackup(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.mkv")
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, hasBackup: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	res, err := svc.ReindexMedia(context.Background(), "m1")
	require.NoError(t, err)
	assert.False(t, res.Skipped)
	assert.Equal(t, []string{p}, reidx.reindexed, "reindexed the resolved path")

	// A media id with no file is a clean error, not a panic.
	_, err = svc.ReindexMedia(context.Background(), "missing-id")
	require.Error(t, err)
}

func TestSeekIndexService_RollbackMedia(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.mkv")
	reidx := &fakeReindexer{hasIndex: map[string]bool{}, hasBackup: map[string]bool{}}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{{MediaID: "m1", Path: p}}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	require.NoError(t, svc.RollbackMedia(context.Background(), "m1"))
	assert.Equal(t, []string{p}, reidx.restored)
}

// ReindexLibrary queues only un-indexed files and refuses a second concurrent
// batch for the same scope while the first is still running.
func TestSeekIndexService_ReindexLibrary_QueuesAndGuardsConcurrency(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.mkv") // un-indexed → queued
	b := filepath.Join(dir, "b.mkv") // indexed → skipped
	c := filepath.Join(dir, "c.mkv") // un-indexed → queued
	for _, p := range []string{a, b, c} {
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	gate := make(chan struct{})
	reidx := &fakeReindexer{
		hasIndex:  map[string]bool{a: false, b: true, c: false},
		hasBackup: map[string]bool{},
		gate:      gate, // hold the background batch open
	}
	cat := fakeCatalog{files: []appmaintenance.MediaRef{
		{MediaID: "m1", Path: a}, {MediaID: "m2", Path: b}, {MediaID: "m3", Path: c},
	}}
	svc := appmaintenance.NewSeekIndexService(reidx, cat, nil)

	queued, err := svc.ReindexLibrary(context.Background(), "lib1")
	require.NoError(t, err)
	assert.Equal(t, 2, queued, "only the two un-indexed files are queued")

	// The first batch is blocked on the gate → a second request is refused.
	_, err = svc.ReindexLibrary(context.Background(), "lib1")
	require.Error(t, err, "a second batch for the same scope must be refused while one runs")

	close(gate) // let the background batch drain so the goroutine exits cleanly
}
