package backup_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/backup"
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

func TestBackupManifest_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		m       backup.BackupManifest
		wantErr bool
	}{
		{"valid", backup.BackupManifest{ID: "bk-1", Version: "1.0", Tables: []string{"users"}}, false},
		{"missing id", backup.BackupManifest{Version: "1.0", Tables: []string{"users"}}, true},
		{"missing version", backup.BackupManifest{ID: "bk-1", Tables: []string{"users"}}, true},
		{"no tables", backup.BackupManifest{ID: "bk-1", Version: "1.0"}, true},
		{"empty tables", backup.BackupManifest{ID: "bk-1", Version: "1.0", Tables: []string{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewBackupManifest(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps CreatedAt + clones tables", func(t *testing.T) {
		t.Parallel()
		tables := []string{"users", "media"}
		m, err := backup.NewBackupManifest("bk-1", "1.0", tables,
			backup.WithBackupSize(4096),
			backup.WithBackupSHA256("abc"),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if m.CreatedAt.IsZero() || m.Size != 4096 || m.DumpSHA256 != "abc" {
			t.Errorf("fields not wired: %+v", m)
		}
		tables[0] = "mutated"
		if m.Tables[0] != "users" {
			t.Error("Tables was not cloned")
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := backup.NewBackupManifest("", "1.0", []string{"t"}); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty tables rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := backup.NewBackupManifest("bk-1", "1.0", nil); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBackupEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{backup.BackupFailed{}, "backup.failed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
