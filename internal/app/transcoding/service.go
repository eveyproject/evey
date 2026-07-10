package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/pkg/metrics"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

var ErrMaxConcurrent = errors.New("max concurrent transcodes reached")

// PlaybackGauge reports how many live sessions are currently ENCODING video --
// the CPU-contended playback (a direct-play/copy stream never counts). Satisfied
// by the VideoSessionLimiter. >0 means a viewer's transcode is running, so
// background jobs should yield.
type PlaybackGauge interface{ Active() int }

type Service struct {
	transcoder    transcoding.Transcoder
	hwDetector    transcoding.HWDetector
	jobs          transcoding.TranscodeJobRepository
	profiles      transcoding.TranscodeProfileRepository
	events        shared.EventBus
	cache         transcoding.CacheStore
	maxConcurrent int

	// playback, when wired, drives background pre-emption: running jobs are
	// SIGSTOPped while live playback encodes and resumed once it goes idle.
	playback   PlaybackGauge
	pauseMu    sync.Mutex
	pausedJobs map[string]bool // jobIDs WE paused, so we only resume ours
}

type Option func(*Service)

func WithCacheStore(cs transcoding.CacheStore) Option {
	return func(s *Service) { s.cache = cs }
}

// WithPlaybackPreemption wires the live-playback gauge that pauses background
// transcodes while viewers are watching. Call SuperviseBackgroundPreemption to
// run the loop.
func WithPlaybackPreemption(g PlaybackGauge) Option {
	return func(s *Service) { s.playback = g }
}

// SetPlaybackGauge wires the live-playback gauge after construction — wire.go
// builds the session limiter after this service. Pair with
// SuperviseBackgroundPreemption.
func (s *Service) SetPlaybackGauge(g PlaybackGauge) { s.playback = g }

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
		pausedJobs:    make(map[string]bool),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) StartTranscode(ctx context.Context, job *transcoding.TranscodeJob) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.StartTranscode")
	defer span.End()
	span.SetAttributes(
		attribute.String("job.id", job.ID),
		attribute.String("media_item.id", string(job.MediaItemID)),
	)
	if err := job.Validate(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("invalid transcode job: %w", err)
	}
	active, err := s.jobs.ListActive(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("list active: %w", err)
	}

	running := 0
	for _, j := range active {
		if j.Status == transcoding.Running {
			running++
		}
	}

	if s.maxConcurrent > 0 && running >= s.maxConcurrent {
		job.Status = transcoding.Queued
		if err := s.jobs.Create(ctx, job); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return fmt.Errorf("Service.StartTranscode: %w", err)
		}
		span.RecordError(ErrMaxConcurrent)
		span.SetStatus(otelcodes.Error, ErrMaxConcurrent.Error())
		return ErrMaxConcurrent
	}

	job.Status = transcoding.Running
	job.StartedAt = time.Now()
	if err := s.jobs.Create(ctx, job); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("Service.StartTranscode: %w", err)
	}

	if _, err := s.transcoder.Start(ctx, job); err != nil {
		job.Status = transcoding.Failed
		job.ErrorMessage = err.Error()
		if updateErr := s.jobs.Update(ctx, job); updateErr != nil {
			slog.Warn("update transcode job failed", slog.String("jobID", job.ID), slog.Any("err", updateErr))
		}
		metrics.TranscodeDuration.WithLabelValues("failed").Observe(time.Since(job.StartedAt).Seconds())
		evtFailed := transcoding.TranscodeFailed{JobID: job.ID, MediaItemID: job.MediaItemID, ErrorMessage: err.Error(), OccurredAt: time.Now()}
		if pubErr := s.events.Publish(ctx, evtFailed); pubErr != nil {
			slog.Warn("publish event", slog.String("event", evtFailed.EventName()), slog.Any("err", pubErr))
		}
		s.refreshActiveGauge(ctx)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("start transcode: %w", err)
	}

	evtStarted := transcoding.TranscodeStarted{JobID: job.ID, MediaItemID: job.MediaItemID, OutputCodec: job.OutputCodec, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.Warn("publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	s.refreshActiveGauge(ctx)
	return nil
}

// refreshActiveGauge recounts Running jobs and sets the Prom gauge.
// Called at every lifecycle hook we control (start/fail/stop) because
// natural completion isn't signaled back to this service — without
// this recount the gauge would drift upward monotonically.
func (s *Service) refreshActiveGauge(ctx context.Context) {
	active, err := s.jobs.ListActive(ctx)
	if err != nil {
		return
	}
	running := 0
	for _, j := range active {
		if j.Status == transcoding.Running {
			running++
		}
	}
	metrics.TranscodesActive.Set(float64(running))
}

func (s *Service) StopTranscode(ctx context.Context, jobID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.StopTranscode")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", jobID))
	if err := s.transcoder.Stop(ctx, jobID); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("Service.StopTranscode: %w", err)
	}
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("Service.StopTranscode: %w", err)
	}
	job.Status = transcoding.Cancelled
	now := time.Now()
	job.EndedAt = &now
	if err := s.jobs.Update(ctx, job); err != nil {
		slog.Warn("update transcode job failed", slog.String("jobID", jobID), slog.Any("err", err))
	}
	if !job.StartedAt.IsZero() {
		metrics.TranscodeDuration.WithLabelValues("cancelled").Observe(now.Sub(job.StartedAt).Seconds())
	}
	evtCancelled := transcoding.TranscodeCancelled{JobID: jobID, MediaItemID: job.MediaItemID, OccurredAt: now}
	if err := s.events.Publish(ctx, evtCancelled); err != nil {
		slog.Warn("publish event", slog.String("event", evtCancelled.EventName()), slog.Any("err", err))
	}
	s.refreshActiveGauge(ctx)
	return nil
}

func (s *Service) GetProgress(ctx context.Context, jobID string) (*transcoding.TranscodeProgress, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.GetProgress")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", jobID))
	progress, err := s.transcoder.Progress(ctx, jobID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return progress, err
}

// GetJob returns the persisted job row. Used by gRPC handlers that
// need the MediaItemID for ownership/library-access checks before
// touching jobID-addressable operations (Stop, GetStatus).
func (s *Service) GetJob(ctx context.Context, jobID string) (*transcoding.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.GetJob")
	defer span.End()
	span.SetAttributes(attribute.String("job.id", jobID))
	job, err := s.jobs.GetByID(ctx, jobID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return job, err
}

func (s *Service) ListActiveTranscodes(ctx context.Context) ([]*transcoding.TranscodeJob, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.ListActiveTranscodes")
	defer span.End()
	jobs, err := s.jobs.ListActive(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return jobs, err
}

func (s *Service) ListProfiles(ctx context.Context) ([]*transcoding.TranscodeProfile, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.ListProfiles")
	defer span.End()
	profiles, err := s.profiles.List(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return profiles, err
}

func (s *Service) CreateProfile(ctx context.Context, p *transcoding.TranscodeProfile) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.CreateProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", p.ID))
	if err := p.Validate(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("invalid transcode profile: %w", err)
	}
	if err := s.profiles.Create(ctx, p); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("create transcode profile %q: %w", p.ID, err)
	}
	return nil
}

func (s *Service) UpdateProfile(ctx context.Context, p *transcoding.TranscodeProfile) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.UpdateProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", p.ID))
	if err := p.Validate(); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("invalid transcode profile: %w", err)
	}
	if err := s.profiles.Update(ctx, p); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("update transcode profile %q: %w", p.ID, err)
	}
	return nil
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.DeleteProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", id))
	if err := s.profiles.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("delete transcode profile %q: %w", id, err)
	}
	return nil
}

func (s *Service) SetMaxConcurrent(n int) {
	s.maxConcurrent = n
	slog.Info("transcoding: updated max concurrent", slog.Int("max", n))
}

// SuperviseBackgroundPreemption blocks until ctx is cancelled. While live
// playback is encoding video (playback.Active() > 0) it SIGSTOPs every running
// background transcode so watching never competes for CPU; once playback has
// been idle for a short grace period (so a quick seek doesn't thrash) it SIGCONTs
// them. Background jobs are transient, so pausing them is invisible except that
// they finish a little later. No-op when no gauge was wired.
func (s *Service) SuperviseBackgroundPreemption(ctx context.Context) {
	if s.playback == nil {
		return
	}
	const tick = 2 * time.Second
	const resumeGrace = 12 * time.Second
	t := time.NewTicker(tick)
	defer t.Stop()

	var idleSince time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.playback.Active() > 0 {
				idleSince = time.Time{}
				s.pauseRunning(ctx)
				continue
			}
			if idleSince.IsZero() {
				idleSince = time.Now()
			}
			if time.Since(idleSince) >= resumeGrace {
				s.resumePaused(ctx)
			}
		}
	}
}

// pauseRunning SIGSTOPs every Running job we haven't already paused.
func (s *Service) pauseRunning(ctx context.Context) {
	active, err := s.jobs.ListActive(ctx)
	if err != nil {
		return
	}
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	for _, j := range active {
		if j.Status != transcoding.Running || s.pausedJobs[j.ID] {
			continue
		}
		if err := s.transcoder.Pause(ctx, j.ID); err != nil {
			slog.Warn("bg-preempt: pause failed", slog.String("jobID", j.ID), slog.Any("err", err))
			continue
		}
		s.pausedJobs[j.ID] = true
		slog.Info("bg-preempt: paused background transcode for live playback", slog.String("jobID", j.ID))
	}
}

// resumePaused SIGCONTs every job we paused. A resume error means the job is
// gone (stopped/completed) — drop it from the set either way.
func (s *Service) resumePaused(ctx context.Context) {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	for id := range s.pausedJobs {
		if err := s.transcoder.Resume(ctx, id); err != nil {
			slog.Warn("bg-preempt: resume failed (job gone?)", slog.String("jobID", id), slog.Any("err", err))
		} else {
			slog.Info("bg-preempt: resumed background transcode", slog.String("jobID", id))
		}
		delete(s.pausedJobs, id)
	}
}

func (s *Service) DetectHardware(ctx context.Context) ([]transcoding.HWDevice, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.DetectHardware")
	defer span.End()
	devices, err := s.hwDetector.DetectDevices(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return devices, err
}

func (s *Service) GetCacheInfo(ctx context.Context) (*transcoding.CacheInfo, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.GetCacheInfo")
	defer span.End()
	if s.cache == nil {
		return &transcoding.CacheInfo{}, nil
	}
	info, err := s.cache.GetCacheInfo(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
	}
	return info, err
}

func (s *Service) ClearCache(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.ClearCache")
	defer span.End()
	if s.cache == nil {
		return nil
	}
	if err := s.cache.ClearCache(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	return nil
}

func (s *Service) DeleteCacheFile(ctx context.Context, filename string) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.DeleteCacheFile")
	defer span.End()
	span.SetAttributes(attribute.String("cache.filename", filename))
	if s.cache == nil {
		err := errors.New("cache store not configured")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	if err := s.cache.DeleteFile(ctx, filename); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	return nil
}
