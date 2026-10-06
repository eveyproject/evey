package grpcapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	apploadtest "github.com/evey-project/evey/internal/app/loadtest"
)

// StartLoadTest launches the built-in load test: synthetic viewers against the
// server's OWN HTTP stack, authenticated with the very bearer of the admin who
// pressed the button - a real credential exercising the real auth path. It
// refuses to run over real viewers, and while a calibration benchmark is
// measuring (two concurrent measurements would lie to each other).
func (d *adminSystemHandler) StartLoadTest(ctx context.Context, req *eveyv1.StartLoadTestRequest) (*eveyv1.LoadTestStatus, error) {
	if d.loadTest == nil {
		return nil, status.Error(codes.Unimplemented, "load test not wired")
	}
	if d.calibrationSvc != nil && d.calibrationSvc.IsBenchmarking() {
		return nil, status.Error(codes.FailedPrecondition, "a calibration benchmark is running; wait for it to finish")
	}
	cfg := apploadtest.Config{
		BaseURL:       d.loadTestBaseURL,
		PublicBaseURL: d.loadTestPublicURL,
		Scenario:      req.GetScenario(),
		MaxViewers:    int(req.GetMaxViewers()),
		Mode:          req.GetMode(),
		// Keep evicting the library from the page cache, so the run measures
		// the storage rather than the RAM (a library that fits in memory
		// otherwise overstates the viewer count, sometimes by a factor of
		// several).
		ColdCache: req.GetColdCache(),
		// A real run measures how much of its own CPU is the load generator's.
		// Without it the report charges the harness's HTTP overhead to the server
		// and an operator sizes his hardware for a cost that isn't there.
		ProfileHarnessCPU: true,
	}
	if secs := req.GetDurationSec(); secs > 0 {
		cfg.MaxDuration = time.Duration(secs) * time.Second
	}
	// Direct: no public address to prefer, so the viewers use the loopback.
	if req.GetDirect() {
		cfg.PublicBaseURL = ""
	}

	token, err := d.loadTestCredential(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cfg.Token = token
	// The run outlives this RPC by design: bind it to the server's lifetime,
	// not the HTTP request's (the runner's MaxDuration walls it regardless).
	if err := d.loadTest.Start(context.WithoutCancel(ctx), cfg); err != nil {
		switch {
		case errors.Is(err, apploadtest.ErrStreamsActive):
			return nil, status.Error(codes.FailedPrecondition, "playback sessions are active; the load test never runs over real viewers")
		case errors.Is(err, apploadtest.ErrAlreadyRunning):
			return nil, status.Error(codes.AlreadyExists, "a load test is already running")
		default:
			return nil, internalError(ctx, "start load test", err)
		}
	}
	return loadTestStatusToProto(d.loadTest.Status()), nil
}

// loadTestTokenSlack is how much longer than the run itself the synthetic
// viewers' credential must live. The run is walled by its own MaxDuration, so
// the token only has to outlast it - never the other way round.
const loadTestTokenSlack = 10 * time.Minute

// loadTestCredential mints the token the synthetic viewers authenticate with.
//
// It deliberately does NOT reuse the caller's bearer: that token is a browser
// session credential with a short life, while a load test ramps for many
// minutes. Borrowing it means the run dies mid-ramp on a 401 and reports the
// expiry as though the server had run out of capacity - a measurement that is
// worse than none. The minted token carries the SAME claims (same user, same
// permissions, same epoch), so revoking the admin's session still kills it.
//
// Falls back to the caller's bearer when no minter is wired, which keeps the
// RPC working in a test harness that never built one.
func (d *adminSystemHandler) loadTestCredential(ctx context.Context, cfg apploadtest.Config) (string, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if d.loadTestToken != nil && claims != nil {
		ttl := cfg.MaxDuration
		if ttl <= 0 {
			ttl = apploadtest.DefaultEnduranceMaxDuration // the longest a run can be
		}
		token, err := d.loadTestToken.GenerateAccessTokenWithTTL(*claims, ttl+loadTestTokenSlack)
		if err != nil {
			return "", status.Error(codes.Internal, "could not mint a load test credential")
		}
		return token, nil
	}

	token := bearerFromContext(ctx)
	if token == "" {
		return "", status.Error(codes.Unauthenticated, "no bearer token on the request")
	}
	return token, nil
}

func (d *adminSystemHandler) GetLoadTestStatus(_ context.Context, _ *eveyv1.GetLoadTestStatusRequest) (*eveyv1.LoadTestStatus, error) {
	if d.loadTest == nil {
		return nil, status.Error(codes.Unimplemented, "load test not wired")
	}
	return loadTestStatusToProto(d.loadTest.Status()), nil
}

func (d *adminSystemHandler) CancelLoadTest(_ context.Context, _ *eveyv1.CancelLoadTestRequest) (*eveyv1.LoadTestStatus, error) {
	if d.loadTest == nil {
		return nil, status.Error(codes.Unimplemented, "load test not wired")
	}
	d.loadTest.Cancel()
	return loadTestStatusToProto(d.loadTest.Status()), nil
}

func loadTestStatusToProto(s apploadtest.Status) *eveyv1.LoadTestStatus {
	out := &eveyv1.LoadTestStatus{
		Running:   s.Running,
		Phase:     s.Phase,
		Viewers:   int32(s.Viewers),
		ElapsedMs: s.Elapsed.Milliseconds(),
	}
	if r := s.LastReport; r != nil {
		out.LastReport = &eveyv1.LoadTestReport{
			Scenario:                      r.Scenario,
			Mode:                          r.Mode,
			SustainedViewers:              int32(r.SustainedViewers),
			StopReason:                    string(r.StopReason),
			StopDetail:                    r.StopDetail,
			TtffMsP50:                     int32(r.TTFFMsP50),
			TtffMsP95:                     int32(r.TTFFMsP95),
			SegmentMsP50:                  int32(r.SegmentMsP50),
			SegmentMsP95:                  int32(r.SegmentMsP95),
			SegmentSloMs:                  int32(r.SegmentSLOMs),
			Samples:                       int32(r.Samples),
			SharedCacheViewers:            int32(r.ReplayedTitleViewers),
			SkippedCandidates:             int32(r.SkippedCandidates),
			StartedAt:                     timestamppb.New(r.StartedAt),
			EndedAt:                       timestamppb.New(r.EndedAt),
			Resources:                     loadTestResourcesToProto(r.Resources),
			ResourceBaseline:              loadTestSampleToProto(r.ResourceBaseline),
			ThroughputCeilingBytesPerSec:  r.ThroughputCeilingBytesPerSec,
			DiskReadCeilingBytesPerSec:    r.DiskReadBytesPerSecCeiling,
			SlowestDiskBytesPerSec:        r.SlowestDiskBytesPerSec,
			DisksProbed:                   int32(r.DisksProbed),
			StorageCollapsesAtReaders:     int32(r.StorageCollapsesAtReaders),
			ReadAmplification:             r.ReadAmplification,
			AppReadAmplification:          r.AppReadAmplification,
			DiskCeilingAtReaders:          int32(r.DiskCeilingAtReaders), //nolint:gosec // a concurrency width, bounded by the probe's own sweep
			DiskReadAtAudienceBytesPerSec: r.DiskReadBytesPerSecAtAudience,
			HarnessCpuFraction:            r.HarnessCPUFraction,
			HarnessCpuMeasured:            r.HarnessCPUMeasured,
			MemCacheBytes:                 r.Resources.MemCacheBytesAvg,
			Tiers:                         tierCosts(r.Tiers),
			DiskReadStarved:               r.DiskReadStarved,
			ColdCache:                     r.ColdCache,
			StorageFsType:                 r.StorageFSType,
			StorageOpaque:                 r.StorageOpaque,
			Endpoint:                      r.Endpoint,
			ViaProxy:                      r.ViaProxy,
		}
		for _, b := range r.PoolBranches {
			out.LastReport.PoolBranches = append(out.LastReport.PoolBranches, &eveyv1.LoadTestPoolBranch{
				Branch:  b.Branch,
				Viewers: int32(b.Viewers), //nolint:gosec // a viewer count, bounded by the run's configured ceiling
			})
		}
		for _, d := range r.Resources.Disks {
			out.LastReport.Disks = append(out.LastReport.Disks, &eveyv1.LoadTestDiskActivity{
				Device:             d.Device,
				UtilPercentAvg:     d.UtilPctAvg,
				UtilPercentPeak:    d.UtilPctPeak,
				ReadBytesPerSecAvg: d.ReadBytesPerSecAvg,
				AwaitMsAvg:         d.AwaitMsAvg,
			})
		}
		if r.HasPerViewerCost {
			out.LastReport.PerViewerCost = loadTestResourcesToProto(r.PerViewerCost)
		}
		if len(r.ViewerMix) > 0 {
			out.LastReport.ViewerMix = make(map[string]int32, len(r.ViewerMix))
			for k, v := range r.ViewerMix {
				out.LastReport.ViewerMix[k] = int32(v)
			}
		}
		for _, m := range r.SourceMix {
			out.LastReport.SourceMix = append(out.LastReport.SourceMix, &eveyv1.LoadTestSourceMix{
				Codec: m.Codec, Height: int32(m.Height), Count: int32(m.Count),
			})
		}
		if len(r.SegmentMsP95ByType) > 0 {
			out.LastReport.SegmentMsP95ByType = make(map[string]int32, len(r.SegmentMsP95ByType))
			for k, v := range r.SegmentMsP95ByType {
				out.LastReport.SegmentMsP95ByType[k] = int32(v)
			}
		}
		if len(r.SegmentSLOMsByType) > 0 {
			out.LastReport.SegmentSloMsByType = make(map[string]int32, len(r.SegmentSLOMsByType))
			for k, v := range r.SegmentSLOMsByType {
				out.LastReport.SegmentSloMsByType[k] = int32(v)
			}
		}
	}
	return out
}

func loadTestResourcesToProto(s apploadtest.ResourceStats) *eveyv1.LoadTestResources {
	if s.Samples == 0 {
		return nil
	}
	return &eveyv1.LoadTestResources{
		CpuPercentAvg:       s.CPUPercentAvg,
		CpuPercentPeak:      s.CPUPercentPeak,
		MemUsedBytesAvg:     s.MemUsedBytesAvg,
		MemUsedBytesMax:     s.MemUsedBytesMax,
		MemLimitBytes:       s.MemLimitBytes,
		DiskReadBytesPerSec: s.DiskReadBytesPerSecAvg,
		ServedBytesPerSec:   s.ServedBytesPerSecAvg,
		Samples:             int32(s.Samples),
		GoHeapLiveBytesAvg:  s.GoHeapLiveBytesAvg,
		GoHeapLiveBytesMax:  s.GoHeapLiveBytesMax,
		GoMemLimitBytes:     s.GoMemLimitBytes,
		GoGcCpuPercentAvg:   s.GoGCCPUPercentAvg,
		GoGcCpuPercentPeak:  s.GoGCCPUPercentPeak,
	}
}

// loadTestSampleToProto carries the pre-viewer baseline, which is a single
// reading rather than a tier's worth of samples.
func loadTestSampleToProto(s apploadtest.ResourceSample) *eveyv1.LoadTestResources {
	if s.MemLimitBytes == 0 && s.CPUPercent == 0 {
		return nil
	}
	return &eveyv1.LoadTestResources{
		CpuPercentAvg:       s.CPUPercent,
		MemUsedBytesAvg:     s.MemUsedBytes,
		MemLimitBytes:       s.MemLimitBytes,
		DiskReadBytesPerSec: s.DiskReadBytesPerSec,
		ServedBytesPerSec:   s.ServedBytesPerSec,
		Samples:             1,
	}
}

// bearerFromContext extracts the raw bearer of the calling request from the
// gRPC metadata (the gateway forwards the HTTP Authorization header).
func bearerFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	return strings.TrimPrefix(vals[0], "Bearer ")
}

// tierCosts maps the run's per-audience-size table onto the wire.
func tierCosts(in []apploadtest.TierCost) []*eveyv1.LoadTestTierCost {
	out := make([]*eveyv1.LoadTestTierCost, 0, len(in))
	for _, t := range in {
		out = append(out, &eveyv1.LoadTestTierCost{
			Viewers:             int32(t.Viewers), //nolint:gosec // an audience size, bounded by the run itself
			CpuPercent:          t.CPUPercent,
			MemUsedBytes:        t.MemUsedBytes,
			DiskReadBytesPerSec: t.DiskReadBytesPerSec,
			ServedBytesPerSec:   t.ServedBytesPerSec,
			CpuPercentPerViewer: t.CPUPercentPerViewer,
			MemBytesPerViewer:   t.MemBytesPerViewer,
			Samples:             int32(t.Samples), //nolint:gosec // a sample count
			GoHeapLiveBytes:     t.GoHeapLiveBytes,
			GoGcCpuPercent:      t.GoGCCPUPercent,
		})
	}
	return out
}
