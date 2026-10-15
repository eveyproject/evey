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
		Viewers: 12, CPUPercent: 34.5, MachineCPUPercent: 40.5, MachineIOWaitPct: 9.5, MemUsedBytes: 67, ProcessRSSBytes: 45,
		DiskReadBytesPerSec: 89, ServedBytesPerSec: 101, Samples: 3,
	}}
	out := tierCosts(in)
	if len(out) != 1 {
		t.Fatalf("got %d tiers", len(out))
	}
	got := out[0]
	if got.Viewers != 12 || got.CpuPercent != 34.5 || got.MachineCpuPercent != 40.5 || got.MachineIowaitPercent != 9.5 ||
		got.MemUsedBytes != 67 || got.ProcessRssBytes != 45 || got.DiskReadBytesPerSec != 89 ||
		got.ServedBytesPerSec != 101 || got.Samples != 3 {
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
	if b := loadTestResourcesToProto(apploadtest.ResourceStats{Samples: 4, CPUPercentAvg: 12, MemUsedBytesAvg: 5, MemLimitBytes: 50}); b == nil || b.CpuPercentAvg != 12 || b.MemLimitBytes != 50 || b.Samples != 4 {
		t.Errorf("baseline = %+v", b)
	}
}

// The machine beside the container, the process beside the container total,
// the mapping, the windows and the verdict cross the wire with distinct values.
func TestLoadTestStatusToProto_CarriesMachineWindowsAndVerdict(t *testing.T) {
	rep := &apploadtest.Report{
		Scenario: "mkvgo", Seed: 1792073232940793883, DiskReadMeasured: true, AppReadMeasured: true,
		BreakViewers: 131, BreakReason: apploadtest.StopSLOBreach, BreakDetail: "p95 7178ms",
		DiskDevices: []apploadtest.DeviceMapping{{Location: "/mnt/disk_1", Device: "sda", Resolved: "read"}, {Location: "/mnt/disk_3", Device: "", Resolved: ""}},
		Resources: apploadtest.ResourceStats{Samples: 6, CPUPercentAvg: 9, MachineCPUPercentAvg: 14, MachineCPUPercentPeak: 21, MachineIOWaitPercentAvg: 65, MachineIOWaitPercentPeak: 81,
			ProcessRSSBytesAvg: 462 << 20, ProcessRSSBytesMax: 470 << 20, MemUsedBytesAvg: 923 << 20, DiskReadMeasured: true, AppReadMeasured: true, AppReadBytesPerSecAvg: 50 << 20},
		BreakResources: apploadtest.ResourceStats{Samples: 4, CPUPercentAvg: 11, CPUPercentPeak: 13,
			Disks: []apploadtest.DiskStat{{Device: "sda", UtilPctAvg: 68, UtilPctPeak: 91, ReadBytesPerSecAvg: 38 << 20, ReadBytesPerSecPeak: 60 << 20, AwaitMsAvg: 27, AwaitMsPeak: 46, QueueDepthAvg: 3.5}}},
		DiskVerdict: apploadtest.DiskVerdict{Device: "sda", AwaitMs: 27, AwaitBaselineMs: 7, AwaitMsPeak: 46, QueueDepth: 3.5, UtilPct: 68, ReadBytesPerSec: 38 << 20, IOWaitPct: 65, Saturated: true},
		Windows: []apploadtest.TierWindow{
			{Tier: 126, Phase: "measuring", Measured: true, Resources: apploadtest.ResourceStats{Samples: 6, CPUPercentAvg: 9}, SegmentMsP50: 262, SegmentMsP95: 1747, SegmentMsP95Scaled: 1200, SegmentSamples: 124, TTFFMsP95: 2079},
			{Tier: 131, Phase: "narrowing", Measured: true, Broke: true, Reason: apploadtest.StopSLOBreach, Detail: "p95 12073ms", Resources: apploadtest.ResourceStats{Samples: 4, CPUPercentAvg: 11},
				SegmentMsP95Scaled: 12073, DiskVerdict: apploadtest.DiskVerdict{Device: "sda", Saturated: true, AwaitMs: 27}},
		},
	}
	got := loadTestStatusToProto(apploadtest.Status{LastReport: rep}).LastReport
	if got.Seed != 1792073232940793883 || !got.DiskReadMeasured || !got.AppReadMeasured || got.BreakReason != "slo_breach" || got.BreakDetail != "p95 7178ms" {
		t.Errorf("headline fields = seed %d measured %v/%v break %q %q", got.Seed, got.DiskReadMeasured, got.AppReadMeasured, got.BreakReason, got.BreakDetail)
	}
	if len(got.DiskDevices) != 2 || got.DiskDevices[0].Location != "/mnt/disk_1" || got.DiskDevices[0].Device != "sda" || got.DiskDevices[0].Resolved != "read" || got.DiskDevices[1].Device != "" {
		t.Errorf("devices = %+v", got.DiskDevices)
	}
	r := got.Resources
	if r.MachineCpuPercentAvg != 14 || r.MachineCpuPercentPeak != 21 || r.MachineIowaitPercentAvg != 65 || r.MachineIowaitPercentPeak != 81 ||
		r.ProcessRssBytesAvg != 462<<20 || r.ProcessRssBytesMax != 470<<20 || !r.DiskReadMeasured || !r.AppReadMeasured || r.AppReadBytesPerSec != 50<<20 {
		t.Errorf("resources = %+v", r)
	}
	if got.BreakResources == nil || got.BreakResources.CpuPercentPeak != 13 || len(got.BreakDisks) != 1 || got.BreakDisks[0].ReadBytesPerSecPeak != 60<<20 || got.BreakDisks[0].AwaitMsPeak != 46 || got.BreakDisks[0].QueueDepthAvg != 3.5 {
		t.Errorf("break = %+v disks %+v", got.BreakResources, got.BreakDisks)
	}
	v := got.DiskVerdict
	if v == nil || v.Device != "sda" || v.AwaitMs != 27 || v.AwaitBaselineMs != 7 || v.AwaitMsPeak != 46 || v.QueueDepth != 3.5 || v.UtilPercent != 68 || v.ReadBytesPerSec != 38<<20 || v.IowaitPercent != 65 || !v.Saturated {
		t.Errorf("verdict = %+v", v)
	}
	if len(got.Windows) != 2 {
		t.Fatalf("windows = %d", len(got.Windows))
	}
	w0, w1 := got.Windows[0], got.Windows[1]
	if w0.Viewers != 126 || w0.Phase != "measuring" || !w0.Measured || w0.Broke || w0.SegmentMsP50 != 262 || w0.SegmentMsP95 != 1747 || w0.SegmentMsP95Scaled != 1200 || w0.SegmentSamples != 124 || w0.TtffMsP95 != 2079 || w0.Resources.CpuPercentAvg != 9 || w0.DiskVerdict != nil {
		t.Errorf("window 0 = %+v", w0)
	}
	if w1.Viewers != 131 || !w1.Broke || w1.Reason != "slo_breach" || w1.Detail != "p95 12073ms" || w1.SegmentMsP95Scaled != 12073 || w1.DiskVerdict == nil || !w1.DiskVerdict.Saturated {
		t.Errorf("window 1 = %+v", w1)
	}
	if loadTestDiskVerdictToProto(apploadtest.DiskVerdict{}) != nil {
		t.Error("a verdict without a device must be omitted")
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
