package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTrash_RealPool runs the trash and its undo on a real pooled filesystem,
// the production case: a mergerfs pool whose library folder exists on two
// branches, the file on one, the trash folder landing on the other. Set
// EVEY_TRASH_POOL to the pool's library root, with films/Dune.mkv and
// films/Dune.fr.srt on the first branch and films/ on the second.
func TestTrash_RealPool(t *testing.T) {
	root := os.Getenv("EVEY_TRASH_POOL")
	if root == "" {
		t.Skip("EVEY_TRASH_POOL not set: needs a mergerfs mount")
	}
	video := filepath.Join(root, "films", "Dune.mkv")
	want, err := os.ReadFile(video)
	require.NoError(t, err)
	trashed, err := moveToTrash(root, video)
	require.NoError(t, err)
	t.Logf("trashed to %s", trashed)
	assert.Equal(t, filepath.Join(root, TrashDirName, "films", "Dune.mkv"), trashed)
	assert.False(t, exists(video))
	got, err := os.ReadFile(trashed)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.True(t, exists(filepath.Join(root, TrashDirName, "films", "Dune.fr.srt")))

	dest, _, err := restoreTarget([]*library.Library{{ID: "l", Paths: []string{root}}}, trashed)
	require.NoError(t, err)
	require.NoError(t, moveFile(trashed, dest))
	moveCompanions(trashed, filepath.Dir(dest))
	assert.True(t, exists(video))
	assert.True(t, exists(filepath.Join(root, "films", "Dune.fr.srt")))
}
