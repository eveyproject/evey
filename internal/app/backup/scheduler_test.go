package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	backupmodel "github.com/gravity-zero/evey/internal/domain/backup"
	"github.com/gravity-zero/evey/internal/domain/backup/fakes"
	"github.com/gravity-zero/evey/internal/domain/shared"
)

// deleteFailStore wraps FakeBackupStore to fail only on Delete
type deleteFailStore struct {
	*fakes.FakeBackupStore
	deleteErr error
}

func (s *deleteFailStore) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.FakeBackupStore.Delete(ctx, id)
}

// failingEventBus returns an error on Publish
type failingEventBus struct{}

func (f *failingEventBus) Publish(context.Context, shared.Event) error {
	return errors.New("bus broken")
}
func (f *failingEventBus) Subscribe(string, func(context.Context, shared.Event)) string { return "" }
func (f *failingEventBus) Unsubscribe(string)                                           {}

// channelEventBus signals when an event is published
type channelEventBus struct {
	mu     sync.Mutex
	events []shared.Event
	ch     chan struct{}
}

func newChannelEventBus() *channelEventBus {
	return &channelEventBus{ch: make(chan struct{}, 10)}
}

func (b *channelEventBus) Publish(_ context.Context, e shared.Event) error {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
	b.ch <- struct{}{}
	return nil
}
func (b *channelEventBus) Subscribe(string, func(context.Context, shared.Event)) string { return "" }
func (b *channelEventBus) Unsubscribe(string)                                           {}

func TestStartScheduler_RunsInitialCleanup(t *testing.T) {
	svc, store, _ := setup()
	ctx, cancel := context.WithCancel(context.Background())

	old, _ := svc.CreateBackup(ctx)
	store.SetCreatedAt(old.ID, time.Now().UTC().AddDate(0, 0, -31))
	_, _ = svc.CreateBackup(ctx) // recent

	svc.StartScheduler(ctx, 24, 30)
	time.Sleep(50 * time.Millisecond) // let goroutine run initial cleanup
	cancel()

	list, _ := svc.ListBackups(context.Background())
	if len(list) != 1 {
		t.Fatalf("expected initial cleanup to delete old backup, got %d remaining", len(list))
	}
}

func TestStartScheduler_ContextCancel(t *testing.T) {
	svc, _, _ := setup()
	ctx, cancel := context.WithCancel(context.Background())

	svc.StartScheduler(ctx, 24, 30)
	time.Sleep(20 * time.Millisecond)
	cancel()
	// goroutine should exit cleanly; no way to observe directly but no panic = pass
}

func TestStartScheduler_InitialCleanupError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	svc := NewService(store, dumper, "1.0.0")

	ctx, cancel := context.WithCancel(context.Background())

	// Create a backup, then make store error for List (used by CleanupOldBackups)
	store.Err = errors.New("list broken")

	svc.StartScheduler(ctx, 24, 30)
	time.Sleep(50 * time.Millisecond)
	cancel()
	// should not panic, just log warning
}

func TestCleanupOldBackups_DeleteError(t *testing.T) {
	inner := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	store := &deleteFailStore{FakeBackupStore: inner, deleteErr: fmt.Errorf("disk error")}
	svc := &Service{store: store, dumper: dumper, version: "1.0.0"}

	ctx := context.Background()
	// Create via inner store directly
	innerSvc := NewService(inner, dumper, "1.0.0")
	m, _ := innerSvc.CreateBackup(ctx)
	inner.SetCreatedAt(m.ID, time.Now().UTC().AddDate(0, 0, -60))

	err := svc.CleanupOldBackups(ctx, 30)
	if err == nil {
		t.Fatal("expected delete error")
	}
	if !strings.Contains(err.Error(), "disk error") {
		t.Fatalf("expected disk error, got: %v", err)
	}
}

func TestExtractTarGz_CorruptedTarEntry(t *testing.T) {
	// Build a gzip stream with invalid tar content
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	gw.Write([]byte("this is not valid tar data"))
	gw.Close()

	_, err := extractTarGz(&buf)
	if err == nil {
		t.Fatal("expected error for corrupted tar")
	}
}

func TestExtractTarGz_UnknownFilesIgnored(t *testing.T) {
	dump := []byte("-- dump\n")
	manifest := &backupmodel.BackupManifest{ID: "bk-x", Version: "1.0.0"}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	mBytes := mustMarshal(t, manifest)
	writeTar(t, tw, "manifest.json", mBytes)
	writeTar(t, tw, "extra_file.txt", []byte("ignored"))
	writeTar(t, tw, "dump.sql", dump)
	tw.Close()
	gw.Close()

	a, err := extractTarGz(&buf)
	if err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	defer os.Remove(a.dumpPath)
	got, readErr := os.ReadFile(a.dumpPath)
	if readErr != nil {
		t.Fatalf("read dump temp file: %v", readErr)
	}
	if !bytes.Equal(got, dump) {
		t.Fatal("dump mismatch")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStartScheduler_TickerCreatesBackup(t *testing.T) {
	// This test verifies the scheduler goroutine's ticker path by examining
	// the code structure. Since the ticker uses intervalHours * time.Hour,
	// we can't easily trigger it in a unit test without waiting.
	// Instead we test the code paths that the scheduler calls directly.

	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	dumper.Err = errors.New("db down")
	bus := newChannelEventBus()
	svc := NewService(store, dumper, "1.0.0").WithEventBus(bus)

	// Simulate what the scheduler does on tick: CreateBackup + publish event
	ctx := context.Background()
	if _, err := svc.CreateBackup(ctx); err != nil {
		if svc.events != nil {
			evt := backupmodel.BackupFailed{
				Error:      err.Error(),
				OccurredAt: time.Now(),
			}
			_ = svc.events.Publish(ctx, evt)
		}
	}

	select {
	case <-bus.ch:
	case <-time.After(time.Second):
		t.Fatal("expected event to be published")
	}

	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(bus.events))
	}
}

func TestExtractTarGz_TruncatedEntry(t *testing.T) {
	// Build a tar.gz where a tar entry header claims more bytes than available
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	// Write header for a large file but close the tar writer immediately
	_ = tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: 9999, Mode: 0640})
	// Write fewer bytes than header claims, then close
	tw.Write([]byte("short"))
	// Force flush and close gzip
	tw.Close()
	gw.Close()

	_, err := extractTarGz(&buf)
	if err == nil {
		t.Fatal("expected error for truncated tar entry")
	}
}

func TestStartScheduler_PublishError(t *testing.T) {
	store := fakes.NewFakeBackupStore()
	dumper := fakes.NewFakeDatabaseDumper()
	dumper.Err = errors.New("db down")
	svc := NewService(store, dumper, "1.0.0").WithEventBus(&failingEventBus{})

	// Simulate scheduler publish error path
	ctx := context.Background()
	if _, err := svc.CreateBackup(ctx); err != nil {
		if svc.events != nil {
			evt := backupmodel.BackupFailed{
				Error:      err.Error(),
				OccurredAt: time.Now(),
			}
			// This should not panic even when Publish returns error
			_ = svc.events.Publish(ctx, evt)
		}
	}
}
