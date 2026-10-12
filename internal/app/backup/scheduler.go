package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eveyproject/evey/internal/domain/backup"
	"github.com/eveyproject/evey/internal/pkg/cron"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ErrBackupInProgress is returned when a backup is requested while another one
// is already running. A pg_dump can outlive the cron interval on a weak NAS, and
// a manual RunNow can land during a scheduled run - two concurrent dumps double
// the disk/memory pressure on exactly the hardware that can least afford it.
var ErrBackupInProgress = errors.New("a backup is already running")

// BackupRunner is the subset of Service methods required by the Scheduler.
// Exposed as an interface so tests can inject a fake without the full Service.
type BackupRunner interface {
	CreateBackup(ctx context.Context) (*backup.BackupManifest, error)
	CleanupOldBackups(ctx context.Context, retentionDays int) error
}

// Scheduler polls the singleton backup_schedule row every minute and triggers
// CreateBackup when enabled && now >= next_run_at.
type Scheduler struct {
	schedRepo backup.ScheduleRepository
	runner    BackupRunner
	logger    *slog.Logger
	nowFunc   func() time.Time
	runWG     sync.WaitGroup
	running   atomic.Bool
}

// NewScheduler builds a production Scheduler backed by the real wall clock.
func NewScheduler(schedRepo backup.ScheduleRepository, svc *Service, logger *slog.Logger) *Scheduler {
	return newSchedulerWithClock(schedRepo, svc, logger, time.Now)
}

// NewSchedulerForTest constructs a Scheduler with an injected clock, enabling
// deterministic testing without relying on wall-clock time.
func NewSchedulerForTest(schedRepo backup.ScheduleRepository, runner BackupRunner, logger *slog.Logger, nowFunc func() time.Time) *Scheduler {
	return &Scheduler{
		schedRepo: schedRepo,
		runner:    runner,
		logger:    logger,
		nowFunc:   nowFunc,
	}
}

func newSchedulerWithClock(schedRepo backup.ScheduleRepository, runner BackupRunner, logger *slog.Logger, nowFunc func() time.Time) *Scheduler {
	return NewSchedulerForTest(schedRepo, runner, logger, nowFunc)
}

// GetSchedule returns the current backup schedule row.
func (s *Scheduler) GetSchedule(ctx context.Context) (*backup.BackupSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.scheduler.GetSchedule")
	defer span.End()
	return s.schedRepo.Get(ctx)
}

// UpdateSchedule validates the cron expression, recomputes next_run_at, and
// persists the updated schedule row.
func (s *Scheduler) UpdateSchedule(ctx context.Context, cronExpr string, enabled bool, retentionDays int) (*backup.BackupSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.scheduler.UpdateSchedule")
	defer span.End()
	if retentionDays <= 0 {
		return nil, fmt.Errorf("retention_days must be > 0")
	}
	nextRun, err := cron.NextTime(cronExpr, s.nowFunc())
	if err != nil {
		return nil, fmt.Errorf("invalid cron: %w", err)
	}
	sched := &backup.BackupSchedule{
		CronExpression: cronExpr,
		Enabled:        enabled,
		RetentionDays:  retentionDays,
		NextRunAt:      &nextRun,
	}
	if err := s.schedRepo.Update(ctx, sched); err != nil {
		return nil, fmt.Errorf("update schedule: %w", err)
	}
	return sched, nil
}

// RunNow triggers an immediate backup outside the cron schedule.
func (s *Scheduler) RunNow(ctx context.Context) (*backup.BackupManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.scheduler.RunNow")
	defer span.End()
	sched, err := s.schedRepo.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return s.execBackup(ctx, sched)
}

// Run is the long-running goroutine: polls the schedule every minute and
// executes a backup when enabled && now >= next_run_at.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.runWG.Wait()
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// WaitForRuns blocks until every backup goroutine started by tick() has finished.
// Intended for deterministic tests; production callers do not need to invoke it.
func (s *Scheduler) WaitForRuns() {
	s.runWG.Wait()
}

// Tick exposes the private tick method for deterministic unit tests.
func (s *Scheduler) Tick(ctx context.Context) {
	s.tick(ctx)
}

func (s *Scheduler) tick(ctx context.Context) {
	sched, err := s.schedRepo.Get(ctx)
	if err != nil {
		s.logger.Warn("backup scheduler: get schedule", slog.Any("err", err))
		return
	}
	if !sched.Enabled {
		return
	}
	now := s.nowFunc()
	if sched.NextRunAt == nil || now.Before(*sched.NextRunAt) {
		return
	}
	// Advance NextRunAt at trigger time, not after the backup completes. A
	// pg_dump can outlive the one-minute tick (weak NAS, big base); when the
	// advance only happened on completion, every tick behind a slow run saw
	// the stale NextRunAt and stacked another backup goroutine. Standard cron
	// semantics, same as the library scan scheduler.
	nextRun, cronErr := cron.NextTime(sched.CronExpression, now)
	if cronErr != nil {
		s.logger.Warn("backup scheduler: invalid cron expression", slog.Any("err", cronErr))
		nextRun = now.Add(24 * time.Hour)
	}
	if advErr := s.schedRepo.AdvanceNextRun(ctx, nextRun); advErr != nil {
		// Without the advance the next tick WOULD re-fire: skip this run rather
		// than reintroduce the pileup for the price of one missed slot.
		s.logger.Error("backup scheduler: advance next run", slog.Any("err", advErr))
		return
	}
	s.logger.Info("backup scheduler: triggering backup")
	s.runWG.Add(1)
	go s.runBackup(ctx, sched)
}

func (s *Scheduler) runBackup(ctx context.Context, sched *backup.BackupSchedule) {
	defer s.runWG.Done()
	// A panic in a bare goroutine takes down the whole process; the scheduler
	// must survive a buggy backup run.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("backup scheduler: backup panicked", slog.Any("panic", r))
		}
	}()
	_, err := s.execBackup(ctx, sched)
	if err != nil {
		s.logger.Error("backup scheduler: backup failed", slog.Any("err", err))
	}
}

func (s *Scheduler) execBackup(ctx context.Context, sched *backup.BackupSchedule) (*backup.BackupManifest, error) {
	// Serialize runs: the tick advance stops same-slot stacking, but a dump that
	// outlives the cron interval, or a RunNow during a scheduled run, would still
	// start a second concurrent pg_dump. One at a time - the loser is refused,
	// not queued (a stale backup is worthless once a fresh one is running).
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrBackupInProgress
	}
	defer s.running.Store(false)
	now := s.nowFunc()
	m, err := s.runner.CreateBackup(ctx)
	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	} else {
		if cleanErr := s.runner.CleanupOldBackups(ctx, sched.RetentionDays); cleanErr != nil {
			s.logger.Warn("backup scheduler: cleanup failed", slog.Any("err", cleanErr))
		}
	}
	// next_run_at is NOT touched here: the cron schedule advanced when the run
	// fired (tick), and a manual RunNow must not eat the next scheduled slot.
	if updateErr := s.schedRepo.RecordRunResult(ctx, now, status, errMsg); updateErr != nil {
		s.logger.Error("backup scheduler: record run result", slog.Any("err", updateErr))
	}
	return m, err
}
