package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

// unreadableFile returns a file that exists on disk but whose parent folder
// cannot be read or traversed (a permission change, a NAS share remapped), and
// a sibling path that was never created.
func unreadableFile(t *testing.T) (present, missing string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	present = filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	return present, filepath.Join(t.TempDir(), "never-created.mkv")
}

// The scan prunes on the strength of these checks: a file that cannot be
// stat'ed is not a file the user deleted, and must read as present.
func TestExistenceChecks_UnreadableFileIsNotGone(t *testing.T) {
	t.Parallel()
	present, missing := unreadableFile(t)
	checks := map[string]func(string) bool{
		"video": (&Scanner{}).FileExists,
		"music": (&MusicScanner{}).Exists,
		"photo": (&PhotoScanner{}).Exists,
	}
	for name, exists := range checks {
		if !exists(present) {
			t.Errorf("%s: a file behind an unreadable folder reads as gone", name)
		}
		if exists(missing) {
			t.Errorf("%s: a file that does not exist reads as present", name)
		}
	}
}
