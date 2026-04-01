package backup_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/backup"
)

func TestBackupManifest_Fields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	m := backup.BackupManifest{
		ID:         "b1",
		CreatedAt:  now,
		Version:    "1.0",
		Tables:     []string{"users", "media"},
		Size:       1024,
		DumpSHA256: "abc123",
	}

	if m.ID != "b1" {
		t.Errorf("ID = %q, want %q", m.ID, "b1")
	}
	if len(m.Tables) != 2 {
		t.Errorf("Tables len = %d, want 2", len(m.Tables))
	}
	if m.Size != 1024 {
		t.Errorf("Size = %d, want 1024", m.Size)
	}
}

func TestBackupEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{backup.BackupFailed{}, "backup.failed"},
		{backup.BackupCreated{}, "backup.created"},
		{backup.BackupRestored{}, "backup.restored"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
