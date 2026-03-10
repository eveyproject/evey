package transcoding

import (
	"context"
	"errors"
	"fmt"
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

func NewService(
	transcoder transcoding.Transcoder,
	hwDetector transcoding.HWDetector,
	jobs transcoding.TranscodeJobRepository,
	profiles transcoding.TranscodeProfileRepository,
	events shared.EventBus,
	maxConcurrent int,
) *Service {
	return &Service{
		transcoder:    transcoder,
		hwDetector:    hwDetector,
		jobs:          jobs,
		profiles:      profiles,
		events:        events,
		maxConcurrent: maxConcurrent,
	}
}

func (s *Service) WithCacheStore(cs transcoding.CacheStore) {
	s.cache = cs
}

func (s *Service) StartTranscode(ctx context.Context, job *transcoding.TranscodeJob) error {
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
			return err
		}
		return ErrMaxConcurrent
	}

	job.Status = string(transcoding.Running)
	job.StartedAt = time.Now()
	if err := s.jobs.Create(ctx, job); err != nil {
		return err
	}

	if _, err := s.transcoder.Start(ctx, job); err != nil {
		job.Status = string(transcoding.Failed)
		job.ErrorMessage = err.Error()
		s.jobs.Update(ctx, job)
		s.events.Publish(ctx, transcoding.TranscodeFailed{JobID: job.ID, MediaItemID: job.MediaItemID, ErrorMessage: err.Error(), OccurredAt: time.Now()})
		return fmt.Errorf("start transcode: %w", err)
	}

	s.events.Publish(ctx, transcoding.TranscodeStarted{JobID: job.ID, MediaItemID: job.MediaItemID, OutputCodec: job.OutputCodec, OccurredAt: time.Now()})
	return nil
}

func (s *Service) StopTranscode(ctx context.Context, jobID string) error {
	if err := s.transcoder.Stop(ctx, jobID); err != nil {
		return err
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		return err
	}
	job.Status = string(transcoding.Cancelled)
	now := time.Now()
	job.EndedAt = &now
	s.jobs.Update(ctx, job)
	s.events.Publish(ctx, transcoding.TranscodeCancelled{JobID: jobID, MediaItemID: job.MediaItemID, OccurredAt: now})
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
		return fmt.Errorf("cache store not configured")
	}
	return s.cache.DeleteFile(ctx, filename)
}
