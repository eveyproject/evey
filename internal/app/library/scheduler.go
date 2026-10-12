package library

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/cron"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type Scheduler struct {
	schedules library.ScanScheduleRepository
	scanner   LibraryScanner
	logger    *slog.Logger
	clock     Clock
	nowFunc   func() time.Time
	scanWG    sync.WaitGroup
}

// NewScheduler builds a Scheduler with the real wall clock. Backward-compatible
// constructor used by production wiring and existing call sites.
func NewScheduler(
	schedules library.ScanScheduleRepository,
	scanner LibraryScanner,
	logger *slog.Logger,
) *Scheduler {
	return NewSchedulerWithClock(schedules, scanner, logger, RealClock())
}

// NewSchedulerWithClock builds a Scheduler with an injected Clock so tests can
// drive ticks and time deterministically.
func NewSchedulerWithClock(
	schedules library.ScanScheduleRepository,
	scanner LibraryScanner,
	logger *slog.Logger,
	clock Clock,
) *Scheduler {
	if clock == nil {
		clock = RealClock()
	}
	return &Scheduler{
		schedules: schedules,
		scanner:   scanner,
		logger:    logger,
		clock:     clock,
		nowFunc:   clock.Now,
	}
}

func (s *Scheduler) SetSchedule(ctx context.Context, libraryID, cronExpr string, enabled bool) (*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.scheduler.SetSchedule")
	defer span.End()
	nextRun, err := NextCronTime(cronExpr, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron: %w", err)
	}
	sched, err := library.NewScanSchedule(shared.LibraryID(libraryID), cronExpr, enabled, &nextRun)
	if err != nil {
		return nil, fmt.Errorf("build scan schedule: %w", err)
	}
	if err := s.schedules.Upsert(ctx, sched); err != nil {
		return nil, fmt.Errorf("upsert schedule: %w", err)
	}
	return sched, nil
}

func (s *Scheduler) GetSchedule(ctx context.Context, libraryID string) (*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.scheduler.GetSchedule")
	defer span.End()
	return s.schedules.GetByLibraryID(ctx, libraryID)
}

func (s *Scheduler) DeleteSchedule(ctx context.Context, libraryID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.library.scheduler.DeleteSchedule")
	defer span.End()
	return s.schedules.Delete(ctx, libraryID)
}

func (s *Scheduler) ListSchedules(ctx context.Context) ([]*library.ScanSchedule, error) {
	ctx, span := tracing.StartSpan(ctx, "app.library.scheduler.ListSchedules")
	defer span.End()
	return s.schedules.List(ctx)
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := s.clock.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			s.tick(ctx)
		}
	}
}

// WaitForScans blocks until every scan goroutine started by tick() has
// finished. Intended for deterministic tests; production callers do not need
// to invoke it.
func (s *Scheduler) WaitForScans() {
	s.scanWG.Wait()
}

func (s *Scheduler) tick(ctx context.Context) {
	schedules, err := s.schedules.ListEnabled(ctx)
	if err != nil {
		s.logger.Warn("scheduler: list enabled", slog.Any("err", err))
		return
	}
	now := s.nowFunc()
	for _, sched := range schedules {
		if sched.NextRunAt == nil || now.Before(*sched.NextRunAt) {
			continue
		}
		// Advance NextRunAt at trigger time, not after the scan completes. A scan
		// can run longer than the tick interval (common on a weak NAS); if
		// NextRunAt only advanced on completion, the next tick would re-fire it -
		// the scan lock rejects the duplicate but logs a spurious "scan failed"
		// every long scan. Standard cron semantics: the schedule advances when it
		// fires, independent of the run's outcome.
		nextRun, cronErr := NextCronTime(sched.CronExpression, now)
		if cronErr != nil {
			s.logger.Warn("scheduler: invalid cron expression", slog.String("library_id", string(sched.LibraryID)), slog.Any("err", cronErr))
			continue
		}
		if updErr := s.schedules.UpdateAfterRun(ctx, string(sched.LibraryID), now, nextRun); updErr != nil {
			s.logger.Error("scheduler: update after run", slog.Any("err", updErr))
			continue
		}
		s.logger.Info("scheduler: triggering scan", slog.String("library_id", string(sched.LibraryID)))
		s.scanWG.Add(1)
		go s.runScan(ctx, sched)
	}
}

func (s *Scheduler) runScan(ctx context.Context, sched *library.ScanSchedule) {
	defer s.scanWG.Done()
	// A panic in a bare goroutine takes down the whole process; the scheduler
	// must survive a buggy scan of one library.
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("scheduler: scan panicked", slog.String("library_id", string(sched.LibraryID)), slog.Any("panic", r))
		}
	}()
	added, updated, err := s.scanner.ScanLibrary(ctx, string(sched.LibraryID))
	if err != nil {
		s.logger.Error("scheduler: scan failed", slog.String("library_id", string(sched.LibraryID)), slog.Any("err", err))
	} else {
		s.logger.Info("scheduler: scan done", slog.String("library_id", string(sched.LibraryID)), slog.Int("added", added), slog.Int("updated", updated))
	}
}

// NextCronTime computes the next run time from a standard 5-field cron expression.
// Delegates to the shared internal/pkg/cron package; kept here for backward compatibility
// with existing call sites and tests in this package.
func NextCronTime(expr string, from time.Time) (time.Time, error) {
	return cron.NextTime(expr, from)
}
