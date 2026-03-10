package library

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gravity-zero/evey/internal/domain/library"
)

type Scheduler struct {
	schedules library.ScanScheduleRepository
	scanner   LibraryScanner
	logger    *slog.Logger
	nowFunc   func() time.Time
}

func NewScheduler(
	schedules library.ScanScheduleRepository,
	scanner LibraryScanner,
	logger *slog.Logger,
) *Scheduler {
	return &Scheduler{
		schedules: schedules,
		scanner:   scanner,
		logger:    logger,
		nowFunc:   time.Now,
	}
}

func (s *Scheduler) SetSchedule(ctx context.Context, libraryID, cronExpr string, enabled bool) (*library.ScanSchedule, error) {
	nextRun, err := NextCronTime(cronExpr, time.Now())
	if err != nil {
		return nil, fmt.Errorf("invalid cron: %w", err)
	}
	sched := &library.ScanSchedule{
		LibraryID:      libraryID,
		CronExpression: cronExpr,
		Enabled:        enabled,
		NextRunAt:      &nextRun,
	}
	if err := s.schedules.Upsert(ctx, sched); err != nil {
		return nil, fmt.Errorf("upsert schedule: %w", err)
	}
	return sched, nil
}

func (s *Scheduler) GetSchedule(ctx context.Context, libraryID string) (*library.ScanSchedule, error) {
	return s.schedules.GetByLibraryID(ctx, libraryID)
}

func (s *Scheduler) DeleteSchedule(ctx context.Context, libraryID string) error {
	return s.schedules.Delete(ctx, libraryID)
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
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
		s.logger.Info("scheduler: triggering scan", slog.String("library_id", sched.LibraryID))
		go s.runScan(sched)
	}
}

func (s *Scheduler) runScan(sched *library.ScanSchedule) {
	ctx := context.Background()
	now := s.nowFunc()
	added, updated, err := s.scanner.ScanLibrary(ctx, sched.LibraryID)
	if err != nil {
		s.logger.Error("scheduler: scan failed", slog.String("library_id", sched.LibraryID), slog.Any("err", err))
	} else {
		s.logger.Info("scheduler: scan done", slog.String("library_id", sched.LibraryID), slog.Int("added", added), slog.Int("updated", updated))
	}
	nextRun, _ := NextCronTime(sched.CronExpression, now)
	if updateErr := s.schedules.UpdateAfterRun(ctx, sched.LibraryID, now, nextRun); updateErr != nil {
		s.logger.Error("scheduler: update after run", slog.Any("err", updateErr))
	}
}

// NextCronTime computes the next run time from a standard 5-field cron expression.
// Supports: minute, hour, day-of-month, month, day-of-week.
// Fields: exact values, *, */N, comma-separated values.
func NextCronTime(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	minuteSet, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron minute: %w", err)
	}
	hourSet, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron hour: %w", err)
	}
	domSet, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-month: %w", err)
	}
	monthSet, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron month: %w", err)
	}
	dowSet, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron day-of-week: %w", err)
	}

	t := from.Truncate(time.Minute).Add(time.Minute)
	limit := from.Add(2 * 366 * 24 * time.Hour)
	for t.Before(limit) {
		if !monthSet[int(t.Month())] {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !domSet[t.Day()] || !dowSet[int(t.Weekday())] {
			t = t.Add(24 * time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !hourSet[t.Hour()] {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
			continue
		}
		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cron: no next run found within 2 years")
}

func parseCronField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)
	parts := strings.Split(field, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "*" {
			for i := min; i <= max; i++ {
				result[i] = true
			}
			continue
		}
		if strings.HasPrefix(part, "*/") {
			step, err := strconv.Atoi(part[2:])
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step %q", part)
			}
			for i := min; i <= max; i += step {
				result[i] = true
			}
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil || lo < min || hi > max || lo > hi {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			for i := lo; i <= hi; i++ {
				result[i] = true
			}
			continue
		}
		val, err := strconv.Atoi(part)
		if err != nil || val < min || val > max {
			return nil, fmt.Errorf("invalid value %q", part)
		}
		result[val] = true
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty field")
	}
	return result, nil
}
