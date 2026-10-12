package mkvgo

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/mkv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDiskAvail replaces the free-space probe for the test's lifetime.
func stubDiskAvail(t *testing.T, avail uint64) {
	t.Helper()
	orig := diskAvail
	diskAvail = func(string) (uint64, error) { return avail, nil }
	t.Cleanup(func() { diskAvail = orig })
}

// The rewrite duplicates the file on its own volume before the swap: a full
// disk must be refused UPFRONT with the numbers, not discovered mid-copy
// (prod: /media2 at 93% - filling it would starve playback and neighbors).
func TestReindex_SpacePreflight(t *testing.T) {
	a := New()

	t.Run("rewrite refuses when the volume cannot hold the temp copy", func(t *testing.T) {
		p := genCuelessMKV(t)
		stubDiskAvail(t, 0)
		_, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeRewrite})
		require.Error(t, err)
		require.ErrorIs(t, err, mkv.ErrInsufficientSpace)
		assert.Contains(t, err.Error(), "available", "the error must carry the numbers")
	})

	t.Run("in-place is never blocked by the guard (no temp copy needed)", func(t *testing.T) {
		p := genCuelessKnownSizeMKV(t)
		stubDiskAvail(t, 0)
		res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{})
		require.NoError(t, err, "the surgical patch writes no sibling copy - a full disk must not block it")
		assert.Equal(t, mkv.ReindexMethodInPlace, res.Method)
	})

	t.Run("rewrite proceeds when the volume has room", func(t *testing.T) {
		p := genCuelessMKV(t)
		stubDiskAvail(t, 1<<40) // 1 TiB
		res, err := a.ReindexInPlace(context.Background(), p, mkv.ReindexOptions{Mode: mkv.ModeRewrite})
		require.NoError(t, err)
		assert.Equal(t, mkv.ReindexMethodRewrite, res.Method)
	})
}
