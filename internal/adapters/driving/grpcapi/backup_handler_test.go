package grpcapi

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appbackup "github.com/eveyproject/evey/internal/app/backup"
	domainbackup "github.com/eveyproject/evey/internal/domain/backup"
	backupfakes "github.com/eveyproject/evey/internal/domain/backup/fakes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// A backup asked while a restore runs is Aborted (409 through the gateway),
// not Internal: nothing failed, the database is busy.
func TestBackupCreateBackup_WhileARestoreRuns(t *testing.T) {
	store := backupfakes.NewFakeBackupStore()
	dumper := backupfakes.NewFakeDatabaseDumper()
	svc := appbackup.NewService(store, dumper, "test")
	h := &backupHandler{svc: svc}
	ctx := context.Background()

	archive := bytes.NewReader(validBackupArchive(t))
	dumper.RestoreStarted = make(chan struct{})
	dumper.RestoreRelease = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- svc.RestoreBackup(ctx, archive) }()
	select {
	case <-dumper.RestoreStarted:
	case err := <-done:
		t.Fatalf("the restore ended before reaching the database: %v", err)
	}

	_, err := h.CreateBackup(ctx, &eveyv1.CreateBackupRequest{})
	close(dumper.RestoreRelease)
	if code := status.Code(err); code != codes.Aborted {
		t.Fatalf("CreateBackup during a restore: %v, want Aborted", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the running restore: %v", err)
	}
}

// validBackupArchive is the smallest archive RestoreBackup accepts: a
// manifest whose checksum matches the fake dump.
func validBackupArchive(t *testing.T) []byte {
	t.Helper()
	dump := []byte("-- fake dump\n")
	sum := sha256.Sum256(dump)
	manifest, err := json.Marshal(&domainbackup.BackupManifest{
		ID: "bk-test", CreatedAt: time.Now().UTC(), Version: "1.0.0",
		Tables: appbackup.BackupTables, DumpSHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for name, data := range map[string][]byte{"manifest.json": manifest, "dump.sql": dump} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Size: int64(len(data)), Mode: 0o640}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}
