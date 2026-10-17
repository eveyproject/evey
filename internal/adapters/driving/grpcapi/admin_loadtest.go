package grpcapi

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
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
		// A replayed draw: the same titles in the same order as the run that
		// reported this seed.
		Seed: req.GetSeed(),
	}
	if secs := req.GetDurationSec(); secs > 0 {
		cfg.MaxDuration = time.Duration(secs) * time.Second
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
		case errors.Is(err, apploadtest.ErrUnknownMode), errors.Is(err, apploadtest.ErrUnknownScenario):
			return nil, status.Error(codes.InvalidArgument, err.Error())
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
			ResourceBaseline:              loadTestResourcesToProto(r.ResourceBaseline),
			ThroughputCeilingBytesPerSec:  r.ThroughputCeilingBytesPerSec,
			DiskReadCeilingBytesPerSec:    r.DiskReadBytesPerSecCeiling,
			SlowestDiskBytesPerSec:        r.SlowestDiskBytesPerSec,
			DisksProbed:                   int32(r.DisksProbed),
			StorageCollapsesAtReaders:     int32(r.StorageCollapsesAtReaders),
			ReadAmplification:             r.ReadAmplification,
			AppReadAmplification:          r.AppReadAmplification,
			DiskCeilingAtReaders:          int32(r.DiskCeilingAtReaders), //nolint:gosec // a concurrency width, bounded by the probe sweep
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
			Seed:                          r.Seed,
			DiskReadMeasured:              r.DiskReadMeasured,
			AppReadMeasured:               r.AppReadMeasured,
			BreakReason:                   string(r.BreakReason),
			BreakDetail:                   r.BreakDetail,
			DiskVerdict:                   loadTestDiskVerdictToProto(r.DiskVerdict),
			RangeLow:                      int32(r.RangeLow),  //nolint:gosec // a viewer count
			RangeHigh:                     int32(r.RangeHigh), //nolint:gosec // a viewer count
			Limiter:                       r.Limiter,
			LimiterLocation:               r.LimiterLocation,
			Advice:                        r.Advice,
			Strata:                        loadTestStrataToProto(r.Strata),
			StrataByDisk:                  loadTestStrataToProto(r.StrataByDisk),
			StrataByBand:                  loadTestStrataToProto(r.StrataByBand),
		}
		for _, g := range r.Grid {
			out.LastReport.Grid = append(out.LastReport.Grid, &eveyv1.LoadTestGridTier{
				Viewers:   int32(g.Viewers), //nolint:gosec // a viewer count
				Windows:   int32(g.Windows), //nolint:gosec // a window count
				Passed:    int32(g.Passed),  //nolint:gosec // a window count
				Held:      g.Held,
				Reason:    string(g.Reason),
				Resources: loadTestResourcesToProto(g.Resources),
			})
		}
		for _, m := range r.DiskDevices {
			out.LastReport.DiskDevices = append(out.LastReport.DiskDevices, &eveyv1.LoadTestDeviceMapping{
				Location: m.Location, Device: m.Device, Resolved: m.Resolved, Disks: m.Disks,
			})
		}
		for _, w := range r.Windows {
			out.LastReport.Windows = append(out.LastReport.Windows, &eveyv1.LoadTestWindow{
				Viewers:            int32(w.Tier), //nolint:gosec // a viewer count, bounded by the configured ceiling of the run
				Phase:              w.Phase,
				Measured:           w.Measured,
				Broke:              w.Broke,
				Reason:             string(w.Reason),
				Detail:             w.Detail,
				Resources:          loadTestResourcesToProto(w.Resources),
				SegmentMsP50:       int32(w.SegmentMsP50),       //nolint:gosec // milliseconds of one fetch
				SegmentMsP95:       int32(w.SegmentMsP95),       //nolint:gosec // milliseconds of one fetch
				SegmentMsP95Scaled: int32(w.SegmentMsP95Scaled), //nolint:gosec // milliseconds of one fetch
				SegmentSamples:     int32(w.SegmentSamples),     //nolint:gosec // a sample count
				TtffMsP95:          int32(w.TTFFMsP95),          //nolint:gosec // milliseconds of one fetch
				DiskVerdict:        loadTestDiskVerdictToProto(w.DiskVerdict),
				StartedAt:          timestamppb.New(w.StartedAt),
				EndedAt:            timestamppb.New(w.EndedAt),
			})
		}
		for _, b := range r.PoolBranches {
			out.LastReport.PoolBranches = append(out.LastReport.PoolBranches, &eveyv1.LoadTestPoolBranch{
				Branch:  b.Branch,
				Viewers: int32(b.Viewers), //nolint:gosec // a viewer count, bounded by the configured ceiling of the run
			})
		}
		out.LastReport.Disks = loadTestDisksToProto(r.Resources.Disks)
		out.LastReport.BreakViewers = int32(r.BreakViewers) //nolint:gosec // a viewer count, bounded by the configured ceiling of the run
		out.LastReport.BreakDisks = loadTestDisksToProto(r.BreakResources.Disks)
		out.LastReport.BreakCpuPercentPeak = r.BreakResources.CPUPercentPeak
		out.LastReport.BreakResources = loadTestResourcesToProto(r.BreakResources)
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
				Codec: m.Codec, Height: int32(m.Height), Count: int32(m.Count), VideoRange: m.Range,
			})
		}
		for _, vt := range slices.Sorted(maps.Keys(r.PathsByType)) {
			for _, path := range slices.Sorted(maps.Keys(r.PathsByType[vt])) {
				out.LastReport.ServedPaths = append(out.LastReport.ServedPaths, &eveyv1.LoadTestServedPath{
					ViewerType: vt, Path: path, Viewers: int32(r.PathsByType[vt][path]),
				})
			}
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
		CpuPercentAvg:            s.CPUPercentAvg,
		CpuPercentPeak:           s.CPUPercentPeak,
		MachineCpuPercentAvg:     s.MachineCPUPercentAvg,
		MachineCpuPercentPeak:    s.MachineCPUPercentPeak,
		MachineIowaitPercentAvg:  s.MachineIOWaitPercentAvg,
		MachineIowaitPercentPeak: s.MachineIOWaitPercentPeak,
		MemUsedBytesAvg:          s.MemUsedBytesAvg,
		MemUsedBytesMax:          s.MemUsedBytesMax,
		ProcessRssBytesAvg:       s.ProcessRSSBytesAvg,
		ProcessRssBytesMax:       s.ProcessRSSBytesMax,
		MemLimitBytes:            s.MemLimitBytes,
		DiskReadBytesPerSec:      s.DiskReadBytesPerSecAvg,
		DiskReadMeasured:         s.DiskReadMeasured,
		AppReadBytesPerSec:       s.AppReadBytesPerSecAvg,
		AppReadMeasured:          s.AppReadMeasured,
		ServedBytesPerSec:        s.ServedBytesPerSecAvg,
		Samples:                  int32(s.Samples), //nolint:gosec // a sample count
		GoHeapLiveBytesAvg:       s.GoHeapLiveBytesAvg,
		GoHeapLiveBytesMax:       s.GoHeapLiveBytesMax,
		GoMemLimitBytes:          s.GoMemLimitBytes,
		GoGcCpuPercentAvg:        s.GoGCCPUPercentAvg,
		GoGcCpuPercentPeak:       s.GoGCCPUPercentPeak,
	}
}

// loadTestStrataToProto carries an audience split.
func loadTestStrataToProto(in []apploadtest.StratumShare) []*eveyv1.LoadTestStratum {
	out := make([]*eveyv1.LoadTestStratum, 0, len(in))
	for _, s := range in {
		out = append(out, &eveyv1.LoadTestStratum{Stratum: s.Stratum, Viewers: int32(s.Viewers)}) //nolint:gosec // a viewer count
	}
	return out
}

// loadTestDiskVerdictToProto carries a hold's disk verdict; nil when the hold
// saw no device.
func loadTestDiskVerdictToProto(v apploadtest.DiskVerdict) *eveyv1.LoadTestDiskVerdict {
	if v.Device == "" {
		return nil
	}
	return &eveyv1.LoadTestDiskVerdict{
		Device:          v.Device,
		AwaitMs:         v.AwaitMs,
		AwaitBaselineMs: v.AwaitBaselineMs,
		AwaitMsPeak:     v.AwaitMsPeak,
		QueueDepth:      v.QueueDepth,
		UtilPercent:     v.UtilPct,
		ReadBytesPerSec: v.ReadBytesPerSec,
		IowaitPercent:   v.IOWaitPct,
		Saturated:       v.Saturated,
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
			Viewers:              int32(t.Viewers), //nolint:gosec // an audience size, bounded by the run itself
			CpuPercent:           t.CPUPercent,
			MachineCpuPercent:    t.MachineCPUPercent,
			MachineIowaitPercent: t.MachineIOWaitPct,
			MemUsedBytes:         t.MemUsedBytes,
			ProcessRssBytes:      t.ProcessRSSBytes,
			DiskReadBytesPerSec:  t.DiskReadBytesPerSec,
			ServedBytesPerSec:    t.ServedBytesPerSec,
			Samples:              int32(t.Samples), //nolint:gosec // a sample count
			GoHeapLiveBytes:      t.GoHeapLiveBytes,
			GoGcCpuPercent:       t.GoGCCPUPercent,
		})
	}
	return out
}

func loadTestDisksToProto(disks []apploadtest.DiskStat) []*eveyv1.LoadTestDiskActivity {
	out := make([]*eveyv1.LoadTestDiskActivity, 0, len(disks))
	for _, d := range disks {
		out = append(out, &eveyv1.LoadTestDiskActivity{
			Device:              d.Device,
			UtilPercentAvg:      d.UtilPctAvg,
			UtilPercentPeak:     d.UtilPctPeak,
			ReadBytesPerSecAvg:  d.ReadBytesPerSecAvg,
			ReadBytesPerSecPeak: d.ReadBytesPerSecPeak,
			AwaitMsAvg:          d.AwaitMsAvg,
			AwaitMsPeak:         d.AwaitMsPeak,
			QueueDepthAvg:       d.QueueDepthAvg,
		})
	}
	return out
}
