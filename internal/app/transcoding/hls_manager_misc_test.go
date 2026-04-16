package transcoding_test

import (
	"context"
	"testing"

	apptranscode "github.com/evey-project/evey/internal/app/transcoding"
	"github.com/evey-project/evey/internal/domain/transcoding/fakes"
)

// CacheDir returns the directory passed to NewHLSManager.
func TestHLSManager_CacheDir_Echoes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, &fakes.HLSProcess{})
	t.Cleanup(func() { m.CleanupAll() })
	if got := m.CacheDir(); got != dir {
		t.Fatalf("CacheDir: want %q, got %q", dir, got)
	}
}

// SetMaxConcurrent updates the field without panic; the new value takes effect
// on next restart (the sem channel is deliberately not replaced).
func TestHLSManager_SetMaxConcurrent_NoPanic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := apptranscode.NewHLSManager(context.Background(), dir, 5, &fakes.HLSProcess{})
	t.Cleanup(func() { m.CleanupAll() })

	m.SetMaxConcurrent(10)
	m.SetMaxConcurrent(1)
	// No assertion beyond "does not panic"; the internal channel semaphore
	// is intentionally not replaced (see source comment).
}
