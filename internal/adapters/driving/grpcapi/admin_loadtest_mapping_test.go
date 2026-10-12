package grpcapi

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
)

// tierCosts is the wire mapping of the per-audience cost table: every field
// crosses with a distinct value so a swapped pair shows.
func TestTierCosts_MapsEveryField(t *testing.T) {
	in := []apploadtest.TierCost{{
		Viewers: 12, CPUPercent: 34.5, MemUsedBytes: 67, DiskReadBytesPerSec: 89, ServedBytesPerSec: 101,
		CPUPercentPerViewer: 2.5, MemBytesPerViewer: 7, Samples: 3,
	}}
	out := tierCosts(in)
	if len(out) != 1 {
		t.Fatalf("got %d tiers", len(out))
	}
	got := out[0]
	if got.Viewers != 12 || got.CpuPercent != 34.5 || got.MemUsedBytes != 67 || got.DiskReadBytesPerSec != 89 ||
		got.ServedBytesPerSec != 101 || got.CpuPercentPerViewer != 2.5 || got.MemBytesPerViewer != 7 || got.Samples != 3 {
		t.Errorf("tier = %+v", got)
	}
	if got := tierCosts(nil); got == nil || len(got) != 0 {
		t.Errorf("no tiers must be an empty slice, got %v", got)
	}
}

// The load test runs with the CALLER's bearer, read off the gateway-forwarded
// Authorization metadata; absent metadata or header reads as no credential.
func TestBearerFromContext(t *testing.T) {
	if got := bearerFromContext(context.Background()); got != "" {
		t.Errorf("no metadata: %q", got)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-other", "1"))
	if got := bearerFromContext(ctx); got != "" {
		t.Errorf("no authorization header: %q", got)
	}
	ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer abc.def"))
	if got := bearerFromContext(ctx); got != "abc.def" {
		t.Errorf("got %q, want the bare token", got)
	}
}

// The status carries the last report only when there is one; per-viewer cost
// only when it was measured; a baseline with no reading is omitted rather than
// sent as zeros the panel would render as a real value.
func TestLoadTestStatusToProto(t *testing.T) {
	idle := loadTestStatusToProto(apploadtest.Status{Running: false, Phase: "idle"})
	if idle.Running || idle.Phase != "idle" || idle.LastReport != nil {
		t.Errorf("idle status = %+v", idle)
	}

	started := time.Date(2026, 9, 16, 1, 0, 0, 0, time.UTC)
	rep := &apploadtest.Report{
		Scenario: "balanced", Mode: "climb", SustainedViewers: 23, StopReason: "cpu_saturated", StopDetail: "92%",
		TTFFMsP50: 800, TTFFMsP95: 1900, SegmentMsP50: 120, SegmentMsP95: 480, SegmentSLOMs: 3000, Samples: 77,
		StartedAt: started, EndedAt: started.Add(4 * time.Minute),
		Resources:          apploadtest.ResourceStats{Samples: 5, CPUPercentAvg: 55, CPUPercentPeak: 92, MemUsedBytesAvg: 100, MemUsedBytesMax: 200, MemLimitBytes: 1000, DiskReadBytesPerSecAvg: 10, ServedBytesPerSecAvg: 20},
		HasPerViewerCost:   true,
		PerViewerCost:      apploadtest.ResourceStats{Samples: 5, CPUPercentAvg: 2},
		ViewerMix:          map[string]int{"mkvgo": 20, "transcode": 3},
		SegmentMsP95ByType: map[string]int{"mkvgo": 300},
		SourceMix:          []apploadtest.SourceMixEntry{{Codec: "hevc", Height: 2160, Range: "hdr10", Count: 4}},
		PathsByType:        map[string]map[string]int{"client:web-chrome": {"transcode": 2, "mkvgo": 5}, "client:tv": {"mkvgo": 9}},
	}
	got := loadTestStatusToProto(apploadtest.Status{Running: true, Phase: "climb", Viewers: 23, Elapsed: 90 * time.Second, LastReport: rep})
	if !got.Running || got.Viewers != 23 || got.ElapsedMs != 90_000 {
		t.Errorf("status = %+v", got)
	}
	r := got.LastReport
	if r == nil {
		t.Fatal("report missing")
	}
	if r.Scenario != "balanced" || r.SustainedViewers != 23 || r.StopReason != "cpu_saturated" || r.TtffMsP95 != 1900 || r.SegmentSloMs != 3000 || r.Samples != 77 {
		t.Errorf("report = %+v", r)
	}
	if r.StartedAt.AsTime() != started || r.EndedAt.AsTime() != started.Add(4*time.Minute) {
		t.Errorf("times = %v / %v", r.StartedAt.AsTime(), r.EndedAt.AsTime())
	}
	if r.Resources == nil || r.Resources.CpuPercentPeak != 92 || r.Resources.Samples != 5 {
		t.Errorf("resources = %+v", r.Resources)
	}
	if r.PerViewerCost == nil || r.PerViewerCost.CpuPercentAvg != 2 {
		t.Errorf("per-viewer cost = %+v", r.PerViewerCost)
	}
	if r.ViewerMix["mkvgo"] != 20 || r.ViewerMix["transcode"] != 3 || r.SegmentMsP95ByType["mkvgo"] != 300 {
		t.Errorf("mixes = %v / %v", r.ViewerMix, r.SegmentMsP95ByType)
	}
	if len(r.SourceMix) != 1 || r.SourceMix[0].VideoRange != "hdr10" {
		t.Errorf("source mix = %v", r.SourceMix)
	}
	// Sorted by type then path, so the admin renders a stable table.
	var paths []string
	for _, p := range r.ServedPaths {
		paths = append(paths, fmt.Sprintf("%s/%s=%d", p.ViewerType, p.Path, p.Viewers))
	}
	if want := []string{"client:tv/mkvgo=9", "client:web-chrome/mkvgo=5", "client:web-chrome/transcode=2"}; !slices.Equal(paths, want) {
		t.Errorf("served paths = %v, want %v", paths, want)
	}
	if r.ResourceBaseline != nil {
		t.Error("an empty baseline must be omitted, not sent as zeros")
	}

	rep.HasPerViewerCost = false
	if loadTestStatusToProto(apploadtest.Status{LastReport: rep}).LastReport.PerViewerCost != nil {
		t.Error("per-viewer cost must be omitted when it was not measured")
	}
	if loadTestResourcesToProto(apploadtest.ResourceStats{}) != nil {
		t.Error("resources without samples must be omitted")
	}
	if b := loadTestSampleToProto(apploadtest.ResourceSample{CPUPercent: 12, MemUsedBytes: 5, MemLimitBytes: 50}); b == nil || b.CpuPercentAvg != 12 || b.MemLimitBytes != 50 {
		t.Errorf("baseline = %+v", b)
	}
}

// A pooled mount's evidence crosses the wire intact: which branch carried how
// many viewers, and what each physical drive did. Distinct values everywhere so
// a swapped pair shows, and the order (busiest first) is the server's to decide.
func TestLoadTestStatusToProto_CarriesPoolBranchesAndDisks(t *testing.T) {
	rep := &apploadtest.Report{
		Scenario: "mkvgo",
		PoolBranches: []apploadtest.BranchShare{
			{Branch: "/mnt/disk_2", Viewers: 41},
			{Branch: "/mnt/disk_1", Viewers: 9},
		},
		Resources: apploadtest.ResourceStats{Disks: []apploadtest.DiskStat{
			{Device: "sdc", UtilPctAvg: 88.5, UtilPctPeak: 97, ReadBytesPerSecAvg: 104 << 20, AwaitMsAvg: 132.5},
			{Device: "sdb", UtilPctAvg: 31, UtilPctPeak: 55, ReadBytesPerSecAvg: 30 << 20, AwaitMsAvg: 18},
		}},
	}
	got := loadTestStatusToProto(apploadtest.Status{LastReport: rep}).LastReport
	if len(got.PoolBranches) != 2 || got.PoolBranches[0].Branch != "/mnt/disk_2" || got.PoolBranches[0].Viewers != 41 ||
		got.PoolBranches[1].Branch != "/mnt/disk_1" || got.PoolBranches[1].Viewers != 9 {
		t.Errorf("pool branches = %+v", got.PoolBranches)
	}
	if len(got.Disks) != 2 {
		t.Fatalf("disks = %+v", got.Disks)
	}
	d := got.Disks[0]
	if d.Device != "sdc" || d.UtilPercentAvg != 88.5 || d.UtilPercentPeak != 97 || d.ReadBytesPerSecAvg != 104<<20 || d.AwaitMsAvg != 132.5 {
		t.Errorf("disk 0 = %+v", d)
	}
	if none := loadTestStatusToProto(apploadtest.Status{LastReport: &apploadtest.Report{}}).LastReport; len(none.PoolBranches) != 0 || len(none.Disks) != 0 {
		t.Errorf("a run on plain storage must carry neither, got %+v / %+v", none.PoolBranches, none.Disks)
	}
}

// The server's own Go runtime figures reach the wire on the sustained tier and
// on every tier row, each with a distinct value so a swapped pair shows.
func TestLoadTestStatusToProto_CarriesTheGoRuntime(t *testing.T) {
	rep := &apploadtest.Report{
		Scenario: "mkvgo",
		Resources: apploadtest.ResourceStats{
			Samples:            4,
			GoHeapLiveBytesAvg: 6 << 30, GoHeapLiveBytesMax: 8 << 30, GoMemLimitBytes: 9 << 30,
			GoGCCPUPercentAvg: 31.5, GoGCCPUPercentPeak: 58,
		},
		Tiers: []apploadtest.TierCost{{Viewers: 30, GoHeapLiveBytes: 7 << 30, GoGCCPUPercent: 44.5}},
	}
	got := loadTestStatusToProto(apploadtest.Status{LastReport: rep}).LastReport
	r := got.Resources
	if r.GoHeapLiveBytesAvg != 6<<30 || r.GoHeapLiveBytesMax != 8<<30 || r.GoMemLimitBytes != 9<<30 ||
		r.GoGcCpuPercentAvg != 31.5 || r.GoGcCpuPercentPeak != 58 {
		t.Errorf("resources = %+v", r)
	}
	if len(got.Tiers) != 1 || got.Tiers[0].GoHeapLiveBytes != 7<<30 || got.Tiers[0].GoGcCpuPercent != 44.5 {
		t.Errorf("tiers = %+v", got.Tiers)
	}
}
