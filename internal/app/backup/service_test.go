package backup

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/backup/fakes"
)

func setup() (*Service, *fakes.FakeBackupStore, *fakes.FakeDatabaseDumper) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "0.1.0")
	return svc, store, dumper
}

func TestCreateBackup(t *testing.T) {
	svc, store, _ := setup()
	ctx := context.Background()

	m, err := svc.CreateBackup(ctx)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if m.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if m.Version != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %s", m.Version)
	}
	if len(m.Tables) == 0 {
		t.Fatal("expected tables list")
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(list))
	}
}

func TestListBackups(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	svc.CreateBackup(ctx)
	svc.CreateBackup(ctx)

	list, err := svc.ListBackups(ctx)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(list))
	}
}

func TestGetBackup(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	got, rc, err := svc.GetBackup(ctx, m.ID)
	if err != nil {
		t.Fatalf("GetBackup: %v", err)
	}
	defer rc.Close()
	if got.ID != m.ID {
		t.Fatalf("expected ID %s, got %s", m.ID, got.ID)
	}
	data, _ := io.ReadAll(rc)
	if len(data) == 0 {
		t.Fatal("expected non-empty data")
	}
}

func TestGetBackupNotFound(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	_, _, err := svc.GetBackup(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestDeleteBackup(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	m, _ := svc.CreateBackup(ctx)
	if err := svc.DeleteBackup(ctx, m.ID); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	list, _ := svc.ListBackups(ctx)
	if len(list) != 0 {
		t.Fatalf("expected 0 backups after delete, got %d", len(list))
	}
}

func TestDeleteBackupNotFound(t *testing.T) {
	svc, _, _ := setup()
	ctx := context.Background()

	err := svc.DeleteBackup(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent backup")
	}
}

func TestRestoreBackup(t *testing.T) {
	svc, _, dumper := setup()
	ctx := context.Background()

	input := []byte("-- restore data\n")
	err := svc.RestoreBackup(ctx, bytes.NewReader(input))
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if !bytes.Equal(dumper.Restored, input) {
		t.Fatalf("expected restored data to match input")
	}
}
