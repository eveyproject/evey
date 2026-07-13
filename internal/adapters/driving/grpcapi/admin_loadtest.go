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
	token := bearerFromContext(ctx)
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "no bearer token on the request")
	}

	cfg := apploadtest.Config{
		BaseURL:    d.loadTestBaseURL,
		Token:      token,
		Scenario:   req.GetScenario(),
		MaxViewers: int(req.GetMaxViewers()),
		Mode:       req.GetMode(),
	}
	if secs := req.GetDurationSec(); secs > 0 {
		cfg.MaxDuration = time.Duration(secs) * time.Second
	}
	// The run outlives this RPC by design: bind it to the server's lifetime,
	// not the HTTP request's (the runner's MaxDuration walls it regardless).
	if err := d.loadTest.Start(context.WithoutCancel(ctx), cfg); err != nil {
		switch {
		case errors.Is(err, apploadtest.ErrStreamsActive):
			return nil, status.Error(codes.FailedPrecondition, "playback sessions are active; the load test never runs over real viewers")
		case errors.Is(err, apploadtest.ErrAlreadyRunning):
			return nil, status.Error(codes.AlreadyExists, "a load test is already running")
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return loadTestStatusToProto(d.loadTest.Status()), nil
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
			Scenario:           r.Scenario,
			Mode:               r.Mode,
			SustainedViewers:   int32(r.SustainedViewers),
			StopReason:         string(r.StopReason),
			StopDetail:         r.StopDetail,
			TtffMsP50:          int32(r.TTFFMsP50),
			TtffMsP95:          int32(r.TTFFMsP95),
			SegmentMsP50:       int32(r.SegmentMsP50),
			SegmentMsP95:       int32(r.SegmentMsP95),
			SegmentSloMs:       int32(r.SegmentSLOMs),
			Samples:            int32(r.Samples),
			SharedCacheViewers: int32(r.SharedCacheViewers),
			SkippedCandidates:  int32(r.SkippedCandidates),
			StartedAt:          timestamppb.New(r.StartedAt),
			EndedAt:            timestamppb.New(r.EndedAt),
			Resources:          loadTestResourcesToProto(r.Resources),
			ResourceBaseline:   loadTestSampleToProto(r.ResourceBaseline),
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
