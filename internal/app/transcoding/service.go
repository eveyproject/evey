package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

var ErrMaxConcurrent = errors.New("max concurrent transcodes reached")

type Service struct {
	transcoder    transcoding.Transcoder
	hwDetector    transcoding.HWDetector
	jobs          transcoding.TranscodeJobRepository
	profiles      transcoding.TranscodeProfileRepository
	events        shared.EventBus
	cache         transcoding.CacheStore
	maxConcurrent int
}

type Option func(*Service)

func WithCacheStore(cs transcoding.CacheStore) Option {
	return func(s *Service) { s.cache = cs }
}

func NewService(
	transcoder transcoding.Transcoder,
	hwDetector transcoding.HWDetector,
	jobs transcoding.TranscodeJobRepository,
	profiles transcoding.TranscodeProfileRepository,
	events shared.EventBus,
	maxConcurrent int,
	opts ...Option,
) *Service {
	s := &Service{
		transcoder:    transcoder,
		hwDetector:    hwDetector,
		jobs:          jobs,
		profiles:      profiles,
		events:        events,
		maxConcurrent: maxConcurrent,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) StartTranscode(ctx context.Context, job *transcoding.TranscodeJob) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("invalid transcode job: %w", err)
	}
	active, err := s.jobs.ListActive(ctx)
	if err != nil {
		return fmt.Errorf("list active: %w", err)
	}

	running := 0
	for _, j := range active {
		if j.Status == string(transcoding.Running) {
			running++
		}
	}

	if s.maxConcurrent > 0 && running >= s.maxConcurrent {
		job.Status = string(transcoding.Queued)
		if err := s.jobs.Create(ctx, job); err != nil {
			return fmt.Errorf("Service.StartTranscode: %w", err)
		}
		return ErrMaxConcurrent
	}

	job.Status = string(transcoding.Running)
	job.StartedAt = time.Now()
	if err := s.jobs.Create(ctx, job); err != nil {
		return fmt.Errorf("Service.StartTranscode: %w", err)
	}

	if _, err := s.transcoder.Start(ctx, job); err != nil {
		job.Status = string(transcoding.Failed)
		job.ErrorMessage = err.Error()
		if updateErr := s.jobs.Update(ctx, job); updateErr != nil {
			slog.Warn("update transcode job failed", slog.String("jobID", job.ID), slog.Any("err", updateErr))
		}
		evtFailed := transcoding.TranscodeFailed{JobID: job.ID, MediaItemID: job.MediaItemID, ErrorMessage: err.Error(), OccurredAt: time.Now()}
		if pubErr := s.events.Publish(ctx, evtFailed); pubErr != nil {
			slog.Warn("publish event", slog.String("event", evtFailed.EventName()), slog.Any("err", pubErr))
		}
		return fmt.Errorf("start transcode: %w", err)
	}

	evtStarted := transcoding.TranscodeStarted{JobID: job.ID, MediaItemID: job.MediaItemID, OutputCodec: job.OutputCodec, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.Warn("publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) StopTranscode(ctx context.Context, jobID string) error {
	if err := s.transcoder.Stop(ctx, jobID); err != nil {
		return fmt.Errorf("Service.StopTranscode: %w", err)
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return fmt.Errorf("Service.StopTranscode: %w", err)
	}
	job.Status = string(transcoding.Cancelled)
	now := time.Now()
	job.EndedAt = &now
	if err := s.jobs.Update(ctx, job); err != nil {
		slog.Warn("update transcode job failed", slog.String("jobID", jobID), slog.Any("err", err))
	}
	evtCancelled := transcoding.TranscodeCancelled{JobID: jobID, MediaItemID: job.MediaItemID, OccurredAt: now}
	if err := s.events.Publish(ctx, evtCancelled); err != nil {
		slog.Warn("publish event", slog.String("event", evtCancelled.EventName()), slog.Any("err", err))
	}
	return nil
}

func (s *Service) GetProgress(ctx context.Context, jobID string) (*transcoding.TranscodeProgress, error) {
	return s.transcoder.Progress(ctx, jobID)
}

func (s *Service) ListActiveTranscodes(ctx context.Context) ([]*transcoding.TranscodeJob, error) {
	return s.jobs.ListActive(ctx)
}

func (s *Service) ListProfiles(ctx context.Context) ([]*transcoding.TranscodeProfile, error) {
	return s.profiles.List(ctx)
}

func (s *Service) CreateProfile(ctx context.Context, p *transcoding.TranscodeProfile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid transcode profile: %w", err)
	}
	return s.profiles.Create(ctx, p)
}

func (s *Service) UpdateProfile(ctx context.Context, p *transcoding.TranscodeProfile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid transcode profile: %w", err)
	}
	return s.profiles.Update(ctx, p)
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	return s.profiles.Delete(ctx, id)
}

func (s *Service) SetMaxConcurrent(n int) {
	s.maxConcurrent = n
	slog.Info("transcoding: updated max concurrent", slog.Int("max", n))
}

func (s *Service) DetectHardware(ctx context.Context) ([]transcoding.HWDevice, error) {
	return s.hwDetector.DetectDevices(ctx)
}

func (s *Service) GetCacheInfo(ctx context.Context) (*transcoding.CacheInfo, error) {
	if s.cache == nil {
		return &transcoding.CacheInfo{}, nil
	}
	return s.cache.GetCacheInfo(ctx)
}

func (s *Service) ClearCache(ctx context.Context) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.ClearCache(ctx)
}

func (s *Service) DeleteCacheFile(ctx context.Context, filename string) error {
	if s.cache == nil {
		return errors.New("cache store not configured")
	}
	return s.cache.DeleteFile(ctx, filename)
}
