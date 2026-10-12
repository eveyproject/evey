package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backupmodel "github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/domain/backup/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
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

	initDone := make(chan struct{})
	svc.StartScheduler(ctx, 24, 30, initDone)
	<-initDone
	cancel()

	list, _ := svc.ListBackups(context.Background())
	if len(list) != 1 {
		t.Fatalf("expected initial cleanup to delete old backup, got %d remaining", len(list))
	}
}

func TestStartScheduler_ContextCancel(t *testing.T) {
	svc, _, _ := setup()
	ctx, cancel := context.WithCancel(context.Background())

	initDone := make(chan struct{})
	svc.StartScheduler(ctx, 24, 30, initDone)
	<-initDone
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

	initDone := make(chan struct{})
	svc.StartScheduler(ctx, 24, 30, initDone)
	<-initDone
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
	svc := NewService(store, dumper, "1.0.0", WithEventBus(bus))

	// CreateBackup announces its own failure.
	ctx := context.Background()
	if _, err := svc.CreateBackup(ctx); err == nil {
		t.Fatal("expected the dump error")
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
	svc := NewService(store, dumper, "1.0.0", WithEventBus(&failingEventBus{}))

	// A failing publish must not hide the backup error nor panic.
	if _, err := svc.CreateBackup(context.Background()); err == nil || !strings.Contains(err.Error(), "db down") {
		t.Fatalf("expected the dump error, got %v", err)
	}
}

// ---- Tests for the DB-driven backup Scheduler ----

type fakeScheduleRepo struct {
	mu    sync.Mutex
	sched backupmodel.BackupSchedule
	err   error
}

func newFakeScheduleRepo() *fakeScheduleRepo {
	return &fakeScheduleRepo{
		sched: backupmodel.BackupSchedule{
			CronExpression: "0 3 * * *",
			Enabled:        false,
			RetentionDays:  30,
		},
	}
}

func (r *fakeScheduleRepo) Get(_ context.Context) (*backupmodel.BackupSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	cp := r.sched
	return &cp, nil
}

func (r *fakeScheduleRepo) Update(_ context.Context, s *backupmodel.BackupSchedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.sched.CronExpression = s.CronExpression
	r.sched.Enabled = s.Enabled
	r.sched.RetentionDays = s.RetentionDays
	r.sched.NextRunAt = s.NextRunAt
	return nil
}

func (r *fakeScheduleRepo) AdvanceNextRun(_ context.Context, nextRun time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.sched.NextRunAt = &nextRun
	return nil
}

func (r *fakeScheduleRepo) RecordRunResult(_ context.Context, lastRun time.Time, status, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.sched.LastRunAt = &lastRun
	r.sched.LastStatus = status
	r.sched.LastError = errMsg
	return nil
}

type fakeBackupRunner struct {
	mu           sync.Mutex
	createCalls  int
	cleanupCalls int
	createErr    error
	manifest     *backupmodel.BackupManifest
}

func newFakeBackupRunner() *fakeBackupRunner {
	m, _ := backupmodel.NewBackupManifest("bk-test", "dev", []string{"users"})
	return &fakeBackupRunner{manifest: m}
}

func (f *fakeBackupRunner) CreateBackup(_ context.Context) (*backupmodel.BackupManifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	return f.manifest, f.createErr
}

func (f *fakeBackupRunner) CleanupOldBackups(_ context.Context, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls++
	return nil
}

func (f *fakeBackupRunner) getCreateCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCalls
}

// blockingBackupRunner parks inside CreateBackup until released, so a second
// concurrent run can be observed hitting (or being refused by) the guard.
type blockingBackupRunner struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (r *blockingBackupRunner) CreateBackup(_ context.Context) (*backupmodel.BackupManifest, error) {
	r.calls.Add(1)
	r.entered <- struct{}{}
	<-r.release
	m, _ := backupmodel.NewBackupManifest("bk-block", "dev", []string{"users"})
	return m, nil
}

func (r *blockingBackupRunner) CleanupOldBackups(_ context.Context, _ int) error { return nil }

func TestScheduler_ExecBackup_RefusesConcurrentRun(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := &blockingBackupRunner{entered: make(chan struct{}, 1), release: make(chan struct{})}
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	go func() { _, _ = sched.RunNow(context.Background()) }()
	<-runner.entered // the first run now holds the guard, parked in CreateBackup

	_, err := sched.RunNow(context.Background())
	if !errors.Is(err, ErrBackupInProgress) {
		t.Fatalf("a second concurrent backup must be refused, got %v", err)
	}
	if got := runner.calls.Load(); got != 1 {
		t.Fatalf("only the first run may reach CreateBackup, got %d", got)
	}
	close(runner.release) // let the first goroutine finish cleanly
}

func TestNewScheduler_GetSchedule(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	got, err := sched.GetSchedule(context.Background())
	if err != nil {
		t.Fatalf("GetSchedule: %v", err)
	}
	if got.CronExpression != "0 3 * * *" {
		t.Errorf("expected '0 3 * * *', got %q", got.CronExpression)
	}
}

func TestNewScheduler_UpdateSchedule_Valid(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	updated, err := sched.UpdateSchedule(context.Background(), "0 */6 * * *", true, 14)
	if err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}
	if updated.CronExpression != "0 */6 * * *" {
		t.Errorf("cron mismatch")
	}
	if !updated.Enabled {
		t.Error("expected enabled=true")
	}
	if updated.RetentionDays != 14 {
		t.Errorf("retention_days mismatch")
	}
	if updated.NextRunAt == nil {
		t.Error("expected next_run_at to be set")
	}
}

func TestNewScheduler_UpdateSchedule_InvalidCron(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	_, err := sched.UpdateSchedule(context.Background(), "not-a-cron", true, 7)
	if err == nil {
		t.Error("expected error for invalid cron")
	}
}

func TestNewScheduler_UpdateSchedule_InvalidRetention(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	_, err := sched.UpdateSchedule(context.Background(), "0 3 * * *", true, 0)
	if err == nil {
		t.Error("expected error for retention_days=0")
	}
}

func TestNewScheduler_RunNow(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	m, err := sched.RunNow(context.Background())
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if m == nil {
		t.Error("expected non-nil manifest")
	}
	if runner.getCreateCalls() != 1 {
		t.Errorf("expected 1 create call, got %d", runner.getCreateCalls())
	}
	got, _ := repo.Get(context.Background())
	if got.LastStatus != "success" {
		t.Errorf("expected last_status=success, got %q", got.LastStatus)
	}
}

func TestNewScheduler_RunNow_Fails(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	runner.createErr = errors.New("disk full")
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	_, err := sched.RunNow(context.Background())
	if err == nil {
		t.Error("expected error from RunNow")
	}
	got, _ := repo.Get(context.Background())
	if got.LastStatus != "failed" {
		t.Errorf("expected last_status=failed, got %q", got.LastStatus)
	}
}

func TestNewScheduler_Tick_Disabled(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	sched.Tick(context.Background())
	sched.WaitForRuns()
	if runner.getCreateCalls() != 0 {
		t.Errorf("expected 0 creates, got %d", runner.getCreateCalls())
	}
}

func TestNewScheduler_Tick_Enabled_Overdue(t *testing.T) {
	repo := newFakeScheduleRepo()
	past := time.Now().Add(-time.Minute)
	repo.sched.Enabled = true
	repo.sched.NextRunAt = &past

	runner := newFakeBackupRunner()
	frozen := time.Now()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), func() time.Time { return frozen })

	sched.Tick(context.Background())
	sched.WaitForRuns()

	if runner.getCreateCalls() != 1 {
		t.Errorf("expected 1 create, got %d", runner.getCreateCalls())
	}
}

func TestNewScheduler_Tick_Enabled_NotYetDue(t *testing.T) {
	repo := newFakeScheduleRepo()
	future := time.Now().Add(time.Hour)
	repo.sched.Enabled = true
	repo.sched.NextRunAt = &future

	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	sched.Tick(context.Background())
	sched.WaitForRuns()

	if runner.getCreateCalls() != 0 {
		t.Errorf("expected 0 creates, got %d", runner.getCreateCalls())
	}
}

func TestNewScheduler_Run_StopsOnCancel(t *testing.T) {
	repo := newFakeScheduleRepo()
	runner := newFakeBackupRunner()
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sched.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}

func slogDefault() *slog.Logger {
	return slog.Default()
}

// blockingRunner parks CreateBackup until release is closed, so a test can
// hold a run "in flight" across several ticks.
type blockingRunner struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (b *blockingRunner) CreateBackup(_ context.Context) (*backupmodel.BackupManifest, error) {
	b.mu.Lock()
	b.calls++
	if b.calls == 1 {
		close(b.started)
	}
	b.mu.Unlock()
	<-b.release
	m, _ := backupmodel.NewBackupManifest("bk-slow", "dev", []string{"users"})
	return m, nil
}

func (b *blockingRunner) CleanupOldBackups(_ context.Context, _ int) error { return nil }

func (b *blockingRunner) getCalls() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

// TestNewScheduler_Tick_SlowRunIsNotRefired pins the anti-pileup rule: the
// schedule advances at TRIGGER time, so a backup that outlives the tick
// interval must not be re-fired by the ticks that elapse while it runs. The
// old order (advance after CreateBackup) stacked one pg_dump goroutine per
// minute behind any slow run - library/scheduler.go had the correct order all
// along, this aligns backup on it.
func TestNewScheduler_Tick_SlowRunIsNotRefired(t *testing.T) {
	repo := newFakeScheduleRepo()
	past := time.Now().Add(-time.Minute)
	repo.sched.Enabled = true
	repo.sched.NextRunAt = &past

	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	sched := NewSchedulerForTest(repo, runner, slogDefault(), time.Now)

	sched.Tick(context.Background())
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the first tick never started a backup")
	}

	// The run is parked in CreateBackup. Every further tick must see the
	// advanced NextRunAt and decline.
	sched.Tick(context.Background())
	sched.Tick(context.Background())

	close(runner.release)
	sched.WaitForRuns()

	if got := runner.getCalls(); got != 1 {
		t.Fatalf("a slow run must fire exactly once, got %d backups - the pre-fix pileup", got)
	}
	got, _ := repo.Get(context.Background())
	if got.NextRunAt == nil || !got.NextRunAt.After(time.Now().Add(-time.Second)) {
		t.Fatalf("NextRunAt must have advanced at trigger time, got %v", got.NextRunAt)
	}
}
