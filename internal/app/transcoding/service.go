package transcoding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/tracing"
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
	transcoder transcoding.Transcoder
	hwDetector transcoding.HWDetector
	jobs       transcoding.TranscodeJobRepository
	profiles   transcoding.TranscodeProfileRepository
	events     shared.EventBus
	cache      transcoding.CacheStore
	// derivedPurges empty the stores ClearCache also wipes (see
	// WithDerivedCachePurge).
	derivedPurges []func(context.Context) (int, error)

	// maxConcurrent is flipped live by the admin PUT while request goroutines
	// read it in StartTranscode - guard it, and go through MaxConcurrent().
	maxConcurrentMu sync.RWMutex
	maxConcurrent   int

	// playback, when wired, drives background pre-emption: running jobs are
	// SIGSTOPped while live playback encodes and resumed once it goes idle.
	playback   PlaybackGauge
	pauseMu    sync.Mutex
	pausedJobs map[string]bool // jobIDs WE paused, so we only resume ours

	// Hardware-detection hysteresis. Detection is not a lookup: it spawns one
	// probe ffmpeg per encoder to prove the silicon really encodes, and
	// GetSystemInfo (polled by the admin panel) calls it. On 2026-07-15 those
	// probes ran repeatedly WHILE the box choked on a 4K software transcode -
	// each round failed under load ("real-encode probe failed"), polluting the
	// logs and stealing CPU at the worst moment. A GPU does not come and go:
	// the matrix is cached for hwDetectTTL, and the mutex additionally
	// serializes concurrent detections (two panel tabs = one probe run).
	// Failures are never cached - a transient under-load refusal must not
	// stick for a whole TTL.
	hwDetectMu   sync.Mutex
	hwDetectAt   time.Time
	hwDetectDevs []transcoding.HWDevice
	hwDetectTTL  time.Duration
}

type Option func(*Service)

func WithCacheStore(cs transcoding.CacheStore) Option {
	return func(s *Service) { s.cache = cs }
}

// WithDerivedCachePurge adds a store the admin cache wipe empties alongside
// the transcode cache - the extracted subtitles, which live under /data and
// not under the cache dir. One button clears everything the server derived
// from the library; before this the subtitle store was never purged at all.
// The purge returns how many entries it removed.
func WithDerivedCachePurge(purge func(context.Context) (int, error)) Option {
	return func(s *Service) {
		if purge != nil {
			s.derivedPurges = append(s.derivedPurges, purge)
		}
	}
}

// WithHWDetectTTL overrides how long a hardware-detection result is served
// from cache (see the hwDetect* fields). Zero keeps the default.
func WithHWDetectTTL(d time.Duration) Option {
	return func(s *Service) {
		if d > 0 {
			s.hwDetectTTL = d
		}
	}
}

// defaultHWDetectTTL: long enough that panel polling never storms the box with
// probe encodes, short enough that a driver fix shows up without a restart.
const defaultHWDetectTTL = 10 * time.Minute

// SetPlaybackGauge wires the live-playback gauge after construction - wire.go
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
		hwDetectTTL:   defaultHWDetectTTL,
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

	if maxConc := s.MaxConcurrent(); maxConc > 0 && running >= maxConc {
		job.Status = transcoding.Queued
		if err := s.jobs.Create(ctx, job); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, err.Error())
			return fmt.Errorf("Service.StartTranscode: %w", err)
		}
		// Refresh here too: this path returns before the success-path refresh
		// below, so without it the queue-depth gauge would not move until the
		// next start/stop - it would lag exactly the saturation it exists to
		// signal.
		s.refreshActiveGauge(ctx)
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
			slog.WarnContext(ctx, "update transcode job failed", slog.String("jobID", job.ID), slog.Any("err", updateErr))
		}
		metrics.TranscodeDuration.WithLabelValues("failed").Observe(time.Since(job.StartedAt).Seconds())
		evtFailed := transcoding.TranscodeFailed{JobID: job.ID, MediaItemID: job.MediaItemID, ErrorMessage: err.Error(), OccurredAt: time.Now()}
		if pubErr := s.events.Publish(ctx, evtFailed); pubErr != nil {
			slog.WarnContext(ctx, "publish event", slog.String("event", evtFailed.EventName()), slog.Any("err", pubErr))
		}
		s.refreshActiveGauge(ctx)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("start transcode: %w", err)
	}

	evtStarted := transcoding.TranscodeStarted{JobID: job.ID, MediaItemID: job.MediaItemID, OutputCodec: job.OutputCodec, OccurredAt: time.Now()}
	if err := s.events.Publish(ctx, evtStarted); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtStarted.EventName()), slog.Any("err", err))
	}
	s.refreshActiveGauge(ctx)
	return nil
}

// refreshActiveGauge recounts Running jobs and sets the Prom gauge.
// Called at every lifecycle hook we control (start/fail/stop) because
// natural completion isn't signaled back to this service - without
// this recount the gauge would drift upward monotonically.
func (s *Service) refreshActiveGauge(ctx context.Context) {
	active, err := s.jobs.ListActive(ctx)
	if err != nil {
		return
	}
	running, queued := 0, 0
	for _, j := range active {
		switch j.Status {
		case transcoding.Running:
			running++
		case transcoding.Queued:
			// A job lands here when StartTranscode found every concurrency slot
			// taken (running >= maxConcurrent) and persisted it Queued instead
			// of starting it. This gauge was declared and registered but never
			// fed - it read a permanent 0, which is worse than absent for a
			// saturation signal. ListActive already returns queued rows, so
			// counting them here costs nothing.
			queued++
		}
	}
	metrics.TranscodesActive.Set(float64(running))
	metrics.TranscodeQueueDepth.Set(float64(queued))
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
		slog.WarnContext(ctx, "update transcode job failed", slog.String("jobID", jobID), slog.Any("err", err))
	}
	if !job.StartedAt.IsZero() {
		metrics.TranscodeDuration.WithLabelValues("cancelled").Observe(now.Sub(job.StartedAt).Seconds())
	}
	evtCancelled := transcoding.TranscodeCancelled{JobID: jobID, MediaItemID: job.MediaItemID, OccurredAt: now}
	if err := s.events.Publish(ctx, evtCancelled); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evtCancelled.EventName()), slog.Any("err", err))
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
	s.maxConcurrentMu.Lock()
	s.maxConcurrent = n
	s.maxConcurrentMu.Unlock()
	slog.Info("transcoding: updated max concurrent", slog.Int("max", n))
}

// MaxConcurrent is the live cap. Read it through here, never the field: the
// admin panel flips it while requests are in flight.
func (s *Service) MaxConcurrent() int {
	s.maxConcurrentMu.RLock()
	defer s.maxConcurrentMu.RUnlock()
	return s.maxConcurrent
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
			slog.WarnContext(ctx, "bg-preempt: pause failed", slog.String("jobID", j.ID), slog.Any("err", err))
			continue
		}
		s.pausedJobs[j.ID] = true
		slog.InfoContext(ctx, "bg-preempt: paused background transcode for live playback", slog.String("jobID", j.ID))
	}
}

// resumePaused SIGCONTs every job we paused. A resume error means the job is
// gone (stopped/completed) - drop it from the set either way.
func (s *Service) resumePaused(ctx context.Context) {
	s.pauseMu.Lock()
	defer s.pauseMu.Unlock()
	for id := range s.pausedJobs {
		if err := s.transcoder.Resume(ctx, id); err != nil {
			slog.WarnContext(ctx, "bg-preempt: resume failed (job gone?)", slog.String("jobID", id), slog.Any("err", err))
		} else {
			slog.InfoContext(ctx, "bg-preempt: resumed background transcode", slog.String("jobID", id))
		}
		delete(s.pausedJobs, id)
	}
}

func (s *Service) DetectHardware(ctx context.Context) ([]transcoding.HWDevice, error) {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.DetectHardware")
	defer span.End()

	// Hysteresis: detection spawns one probe ffmpeg per encoder, and this is
	// called by the panel-polled GetSystemInfo. Serve the cached matrix inside
	// the TTL; the mutex also collapses concurrent callers onto one probe run.
	// See the hwDetect* field doc for the 2026-07-15 probe storm this ends.
	s.hwDetectMu.Lock()
	defer s.hwDetectMu.Unlock()
	if !s.hwDetectAt.IsZero() && time.Since(s.hwDetectAt) < s.hwDetectTTL {
		return s.hwDetectDevs, nil
	}

	devices, err := s.hwDetector.DetectDevices(ctx)
	if err != nil {
		// Never cache a failure: a probe refused under load must not stick a
		// wrong matrix for a whole TTL.
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return devices, err
	}
	s.hwDetectDevs = devices
	s.hwDetectAt = time.Now()
	return devices, nil
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

// ActiveEncodingSessions is how many viewers are being ENCODED right now - the
// ones a cache wipe cuts off. Zero when no gauge is wired.
//
// Deliberately not a field of CacheInfo: that struct is filled by the cache
// store, which reads a directory and knows nothing about who is watching.
func (s *Service) ActiveEncodingSessions() int {
	if s.playback == nil {
		return 0
	}
	return s.playback.Active()
}

// ClearCache empties the transcode cache.
//
// It does NOT stop the sessions writing into it: the store deletes the bytes out
// from under any live ffmpeg, which then dies mid-segment on ENOENT (`exit status
// 254`) instead of on a clean kill. Playback survives - the orchestrator respawns
// and re-encodes what it lost - so this is a rough edge, not data loss, and the
// admin asking for a wipe usually means it.
//
// What was missing was the WARNING. On 2026-09-04 a wipe landed during a
// playback; the resulting error burst looked exactly like an orchestrator race
// and cost an investigation to rule out every other suspect. The count below
// makes the connection explicit at the moment it happens.
//
// Deliberately NOT a refusal: the user's call (2026-09-04) is that warning is
// enough. Blocking a wipe because someone is watching would make the admin
// action unreliable, which is worse than a respawn.
func (s *Service) ClearCache(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "app.transcoding.ClearCache")
	defer span.End()
	if s.cache == nil {
		return nil
	}
	// The gauge counts live ENCODING sessions - the ones with an ffmpeg holding
	// files open in the very directory about to be removed. A direct-play viewer
	// never counts and is never disturbed by a wipe, which is why this is the
	// right thing to measure here.
	if s.playback != nil {
		if live := s.playback.Active(); live > 0 {
			slog.WarnContext(ctx, "clearing the transcode cache while playback is encoding: those sessions lose their segments and their ffmpeg will die mid-write (they respawn)",
				slog.Int("live_encoding_sessions", live))
		}
	}
	if err := s.cache.ClearCache(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}
	// The derived stores after the main one, and a failure there is logged,
	// not returned: the transcode cache IS cleared at this point, and telling
	// the operator otherwise would be false.
	for _, purge := range s.derivedPurges {
		n, perr := purge(ctx)
		if perr != nil {
			span.RecordError(perr)
			slog.WarnContext(ctx, "clear cache: derived store purge failed", slog.Any("err", perr))
			continue
		}
		slog.InfoContext(ctx, "clear cache: derived store purged", slog.Int("entries", n))
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
