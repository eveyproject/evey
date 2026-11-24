package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func touch(t *testing.T, p string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestLibraryRootOf_LongestRootAndNoEscape(t *testing.T) {
	roots := []string{"/media", "/media/films", "/"}
	assert.Equal(t, "/media/films", libraryRootOf(roots, "/media/films/Dune/Dune.mkv"))
	assert.Equal(t, "/media", libraryRootOf(roots, "/media/other/x.mkv"))
	// "/" is never a usable root, and a sibling with the same prefix is not inside.
	assert.Equal(t, "", libraryRootOf([]string{"/"}, "/x.mkv"))
	assert.Equal(t, "", libraryRootOf([]string{"/media"}, "/media2/x.mkv"))
}

// The video and its subtitles go to <root>/.evey-trash/<same path>; another
// copy of the same film in the same folder stays where it is.
func TestMoveToTrash_MovesVideoAndCompanionsOnly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Dune")
	video := filepath.Join(dir, "Dune.mkv")
	touch(t, video)
	touch(t, filepath.Join(dir, "Dune.fr.srt"))
	touch(t, filepath.Join(dir, "Dune.nfo"))
	touch(t, filepath.Join(dir, "Dune.2021.mkv")) // another copy: must stay

	dest, err := moveToTrash(root, video)
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(root, TrashDirName, "Dune", "Dune.mkv"), dest)
	assert.True(t, exists(dest))
	assert.False(t, exists(video))
	assert.True(t, exists(filepath.Join(root, TrashDirName, "Dune", "Dune.fr.srt")))
	assert.True(t, exists(filepath.Join(root, TrashDirName, "Dune", "Dune.nfo")))
	assert.True(t, exists(filepath.Join(dir, "Dune.2021.mkv")), "a sibling copy is not a companion")
}

// The same name trashed twice keeps both: the second gets a timestamp.
func TestMoveToTrash_NeverOverwrites(t *testing.T) {
	root := t.TempDir()
	video := filepath.Join(root, "Film.mkv")
	touch(t, filepath.Join(root, TrashDirName, "Film.mkv"))
	touch(t, video)

	dest, err := moveToTrash(root, video)
	require.NoError(t, err)
	assert.NotEqual(t, filepath.Join(root, TrashDirName, "Film.mkv"), dest)
	assert.True(t, exists(dest))
	assert.True(t, exists(filepath.Join(root, TrashDirName, "Film.mkv")))
}

// A file already gone is an error, and nothing is created in its place.
func TestMoveToTrash_MissingFileFails(t *testing.T) {
	root := t.TempDir()
	_, err := moveToTrash(root, filepath.Join(root, "gone.mkv"))
	require.Error(t, err)
	assert.True(t, IsTrashError(err))
}

// Trash then restore puts the video and its subtitles back where they were.
func TestRestoreTarget_RoundTrip(t *testing.T) {
	root := t.TempDir()
	video := filepath.Join(root, "Dune", "Dune.mkv")
	touch(t, video)
	touch(t, filepath.Join(root, "Dune", "Dune.fr.srt"))
	trashed, err := moveToTrash(root, video)
	require.NoError(t, err)

	libs := []*library.Library{{ID: "lib1", Paths: []string{root}}}
	dest, libID, err := restoreTarget(libs, trashed)
	require.NoError(t, err)
	assert.Equal(t, video, dest)
	assert.Equal(t, "lib1", libID)

	require.NoError(t, os.Rename(trashed, dest))
	moveCompanions(trashed, filepath.Dir(dest))
	assert.True(t, exists(video))
	assert.True(t, exists(filepath.Join(root, "Dune", "Dune.fr.srt")))
}

// Restore only ever moves a file OUT of a library trash, never anything else,
// and never over a file that took its place.
func TestRestoreTarget_Refusals(t *testing.T) {
	root := t.TempDir()
	libs := []*library.Library{{ID: "lib1", Paths: []string{root}}}
	for _, p := range []string{
		filepath.Join(root, "Dune.mkv"),                  // not in the trash
		"/etc/passwd",                                    // outside every library
		filepath.Join(root, TrashDirName),                // the trash itself
		filepath.Join(root, TrashDirName, "..", "x.mkv"), // not clean
		"relative/.evey-trash/x.mkv",                     // not absolute
	} {
		_, _, err := restoreTarget(libs, p)
		assert.ErrorIs(t, err, ErrRestoreNotInTrash, p)
	}
	touch(t, filepath.Join(root, "Taken.mkv"))
	_, _, err := restoreTarget(libs, filepath.Join(root, TrashDirName, "Taken.mkv"))
	assert.ErrorIs(t, err, ErrRestoreTaken)
}
