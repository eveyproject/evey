package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	backupfakes "github.com/eveyproject/evey/internal/domain/backup/fakes"
)

func newTestBackupHandler() *backupHandler {
	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	svc := appbackup.NewService(store, dumper, "test")
	return &backupHandler{svc: svc}
}

func TestBackupCreateBackup(t *testing.T) {
	h := newTestBackupHandler()
	resp, err := h.CreateBackup(context.Background(), &eveyv1.CreateBackupRequest{})
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if resp.Id == "" {
		t.Error("expected non-empty backup id")
	}
	if resp.Version != "test" {
		t.Errorf("expected version 'test', got %q", resp.Version)
	}
	if len(resp.Tables) == 0 {
		t.Error("expected non-empty tables list")
	}
}

func TestBackupListBackups_Empty(t *testing.T) {
	h := newTestBackupHandler()
	resp, err := h.ListBackups(context.Background(), &eveyv1.ListBackupsRequest{})
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(resp.Backups) != 0 {
		t.Errorf("expected 0 backups, got %d", len(resp.Backups))
	}
}

func TestBackupListBackups_WithData(t *testing.T) {
	h := newTestBackupHandler()
	_, err := h.CreateBackup(context.Background(), &eveyv1.CreateBackupRequest{})
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	resp, err := h.ListBackups(context.Background(), &eveyv1.ListBackupsRequest{})
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(resp.Backups) != 1 {
		t.Errorf("expected 1 backup, got %d", len(resp.Backups))
	}
}

func TestBackupDeleteBackup_NotFound(t *testing.T) {
	h := newTestBackupHandler()
	_, err := h.DeleteBackup(context.Background(), &eveyv1.DeleteBackupRequest{Id: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent backup")
	}
}

func TestBackupDeleteBackup_Success(t *testing.T) {
	h := newTestBackupHandler()
	created, err := h.CreateBackup(context.Background(), &eveyv1.CreateBackupRequest{})
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	_, err = h.DeleteBackup(context.Background(), &eveyv1.DeleteBackupRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	list, err := h.ListBackups(context.Background(), &eveyv1.ListBackupsRequest{})
	if err != nil {
		t.Fatalf("ListBackups after delete: %v", err)
	}
	if len(list.Backups) != 0 {
		t.Errorf("expected 0 backups after delete, got %d", len(list.Backups))
	}
}

func TestBackupCreateBackup_DumperError(t *testing.T) {
	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	dumper.Err = errors.New("dump failed")
	svc := appbackup.NewService(store, dumper, "test")
	h := &backupHandler{svc: svc}

	_, err := h.CreateBackup(context.Background(), &eveyv1.CreateBackupRequest{})
	if err == nil {
		t.Error("expected error when dumper fails")
	}
}
