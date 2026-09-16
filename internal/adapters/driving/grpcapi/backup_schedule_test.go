package grpcapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appbackup "github.com/evey-project/evey/internal/app/backup"
	domainbackup "github.com/evey-project/evey/internal/domain/backup"
)

// fakeScheduleRepoForHandler is an in-memory ScheduleRepository for handler tests.
type fakeScheduleRepoForHandler struct {
	sched domainbackup.BackupSchedule
	err   error
}

func newFakeHandlerScheduleRepo() *fakeScheduleRepoForHandler {
	return &fakeScheduleRepoForHandler{
		sched: domainbackup.BackupSchedule{
			CronExpression: "0 3 * * *",
			Enabled:        false,
			RetentionDays:  30,
			UpdatedAt:      time.Now(),
		},
	}
}

func (r *fakeScheduleRepoForHandler) Get(_ context.Context) (*domainbackup.BackupSchedule, error) {
	if r.err != nil {
		return nil, r.err
	}
	cp := r.sched
	return &cp, nil
}

func (r *fakeScheduleRepoForHandler) Update(_ context.Context, s *domainbackup.BackupSchedule) error {
	if r.err != nil {
		return r.err
	}
	r.sched.CronExpression = s.CronExpression
	r.sched.Enabled = s.Enabled
	r.sched.RetentionDays = s.RetentionDays
	r.sched.NextRunAt = s.NextRunAt
	return nil
}

func (r *fakeScheduleRepoForHandler) AdvanceNextRun(_ context.Context, nextRun time.Time) error {
	if r.err != nil {
		return r.err
	}
	r.sched.NextRunAt = &nextRun
	return nil
}

func (r *fakeScheduleRepoForHandler) RecordRunResult(_ context.Context, lastRun time.Time, status, errMsg string) error {
	if r.err != nil {
		return r.err
	}
	r.sched.LastRunAt = &lastRun
	r.sched.LastStatus = status
	r.sched.LastError = errMsg
	return nil
}

// fakeRunnerForHandler is a BackupRunner for handler tests.
type fakeRunnerForHandler struct {
	createErr error
	manifest  *domainbackup.BackupManifest
}

func newFakeRunnerForHandler() *fakeRunnerForHandler {
	m, _ := domainbackup.NewBackupManifest("bk-handler-test", "dev", []string{"users"})
	return &fakeRunnerForHandler{manifest: m}
}

func (f *fakeRunnerForHandler) CreateBackup(_ context.Context) (*domainbackup.BackupManifest, error) {
	return f.manifest, f.createErr
}

func (f *fakeRunnerForHandler) CleanupOldBackups(_ context.Context, _ int) error {
	return nil
}

func newTestBackupScheduler(repo *fakeScheduleRepoForHandler, runner *fakeRunnerForHandler) *appbackup.Scheduler {
	return appbackup.NewSchedulerForTest(repo, runner, slog.Default(), time.Now)
}

func TestBackupScheduleHandler_GetBackupSchedule(t *testing.T) {
	repo := newFakeHandlerScheduleRepo()
	runner := newFakeRunnerForHandler()
	scheduler := newTestBackupScheduler(repo, runner)
	h := &backupScheduleHandler{scheduler: scheduler}

	resp, err := h.GetBackupSchedule(context.Background(), &eveyv1.GetBackupScheduleRequest{})
	if err != nil {
		t.Fatalf("GetBackupSchedule: %v", err)
	}
	if resp.CronExpression != "0 3 * * *" {
		t.Errorf("expected cron '0 3 * * *', got %q", resp.CronExpression)
	}
	if resp.Enabled {
		t.Error("expected enabled=false")
	}
	if resp.RetentionDays != 30 {
		t.Errorf("expected retention_days=30, got %d", resp.RetentionDays)
	}
}

func TestBackupScheduleHandler_UpdateBackupSchedule_PartialUpdate(t *testing.T) {
	repo := newFakeHandlerScheduleRepo()
	runner := newFakeRunnerForHandler()
	scheduler := newTestBackupScheduler(repo, runner)
	h := &backupScheduleHandler{scheduler: scheduler}

	newCron := "0 */6 * * *"
	enabled := true
	resp, err := h.UpdateBackupSchedule(context.Background(), &eveyv1.UpdateBackupScheduleRequest{
		CronExpression: &newCron,
		Enabled:        &enabled,
	})
	if err != nil {
		t.Fatalf("UpdateBackupSchedule: %v", err)
	}
	if resp.CronExpression != "0 */6 * * *" {
		t.Errorf("cron mismatch: %q", resp.CronExpression)
	}
	if !resp.Enabled {
		t.Error("expected enabled=true")
	}
	// retention_days not updated, should remain 30
	if resp.RetentionDays != 30 {
		t.Errorf("expected retention_days=30, got %d", resp.RetentionDays)
	}
}

func TestBackupScheduleHandler_UpdateBackupSchedule_InvalidCron(t *testing.T) {
	repo := newFakeHandlerScheduleRepo()
	runner := newFakeRunnerForHandler()
	scheduler := newTestBackupScheduler(repo, runner)
	h := &backupScheduleHandler{scheduler: scheduler}

	bad := "not-a-cron"
	_, err := h.UpdateBackupSchedule(context.Background(), &eveyv1.UpdateBackupScheduleRequest{
		CronExpression: &bad,
	})
	if err == nil {
		t.Error("expected error for invalid cron")
	}
}

func TestBackupScheduleHandler_RunBackupNow(t *testing.T) {
	repo := newFakeHandlerScheduleRepo()
	runner := newFakeRunnerForHandler()
	scheduler := newTestBackupScheduler(repo, runner)
	h := &backupScheduleHandler{scheduler: scheduler}

	resp, err := h.RunBackupNow(context.Background(), &eveyv1.RunBackupNowRequest{})
	if err != nil {
		t.Fatalf("RunBackupNow: %v", err)
	}
	if resp.BackupId != "bk-handler-test" {
		t.Errorf("expected backup_id 'bk-handler-test', got %q", resp.BackupId)
	}
}

// A failed backup's cause (pg_dump stderr, a filesystem path) belongs in the
// server log, not on the wire: the RPC answers a sanitized internal error.
func TestBackupScheduleHandler_RunBackupNow_FailureIsSanitized(t *testing.T) {
	repo := newFakeHandlerScheduleRepo()
	runner := newFakeRunnerForHandler()
	runner.createErr = errors.New("pg_dump: /var/lib/evey/backups/full.sql: no space left on device")
	h := &backupScheduleHandler{scheduler: newTestBackupScheduler(repo, runner)}

	_, err := h.RunBackupNow(context.Background(), &eveyv1.RunBackupNowRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
	if msg := status.Convert(err).Message(); strings.Contains(msg, "pg_dump") || strings.Contains(msg, "/var/lib") {
		t.Errorf("internal detail leaked on the wire: %q", msg)
	}
}
