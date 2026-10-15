import { describe, it, expect, vi } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { LoadTestReportPanel } from "./LoadTestReportPanel";
import type { LoadTestReport } from "./loadTestTypes";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, opts?: Record<string, unknown>) => (opts ? `${k}(${Object.entries(opts).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : k),
  }),
}));
vi.mock("@evey/api", () => ({ formatBytes: (n: number) => `${n}B` }));

// The panel answers "how many viewers, and what gave out". A bottleneck is
// the headline verdict, the sustained tier is the bold row, a max-viewers stop
// says no bottleneck was met, and the storage/cache caveats appear only when
// they change how the number should be read.
describe("LoadTestReportPanel", () => {
  it("reads a CPU bottleneck with its tiers and caveats", () => {
    const report: LoadTestReport = {
      scenario: "balanced",
      sustainedViewers: 12,
      stopReason: "cpu_saturated",
      coldCache: true,
      diskReadStarved: true,
      storageCollapsesAtReaders: 9,
      ttffMsP50: 410.4,
      ttffMsP95: 1200.6,
      segmentMsP50: 80,
      segmentMsP95: 300,
      viewerMix: { transcode: 8, copy: 4 },
      diskReadMeasured: true,
      seed: "1792073232940793883",
      endpoint: "http://127.0.0.1:9092",
      windows: [
        { viewers: 8, phase: "ramping", measured: false, resources: { cpuPercentAvg: 61.2, machineCpuPercentAvg: 70.4, memUsedBytesAvg: 100, processRssBytesAvg: 40, diskReadBytesPerSec: 1_000_000, servedBytesPerSec: 2_000_000 }, segmentMsP95: 300, segmentMsP95Scaled: 150 },
        { viewers: 16, phase: "ramping", measured: false, broke: true, reason: "cpu_saturated", resources: { cpuPercentAvg: 97, machineCpuPercentAvg: 99 }, segmentMsP95: 900, segmentMsP95Scaled: 450 },
        { viewers: 12, phase: "narrowing", measured: true, resources: { cpuPercentAvg: 94.7, machineCpuPercentAvg: 98.1, machineIowaitPercentAvg: 2.4, memUsedBytesAvg: 140, processRssBytesAvg: 60, diskReadBytesPerSec: 1_500_000, servedBytesPerSec: 3_000_000 }, segmentMsP95: 310, segmentMsP95Scaled: 155 },
      ],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={10} />);

    expect(screen.getByTestId("load-test-verdict")).toHaveTextContent("admin.loadtest.verdict(count=12,scenario=admin.loadtest.scenarioBalancedNoun)");
    const verdict = screen.getByTestId("load-test-stop-reason");
    expect(verdict).toHaveTextContent("admin.loadtest.stopReason.cpuSaturated");
    expect(verdict.className).toContain("font-medium");
    expect(screen.queryByTestId("load-test-no-bottleneck")).toBeNull();
    expect(screen.getByTestId("load-test-cold-cache")).toBeInTheDocument();
    expect(screen.getByTestId("load-test-disk-starved")).toHaveTextContent("coldCacheStillStarvedLine");
    expect(screen.getByTestId("load-test-storage-collapse")).toHaveTextContent("count=9");

    // The path measured and the seed are said, so the run can be replayed
    // and compared with the right thing.
    expect(screen.getByTestId("load-test-endpoint")).toHaveTextContent("endpointDirectLine(endpoint=http://127.0.0.1:9092)");
    expect(screen.getByTestId("load-test-seed")).toHaveTextContent("seed=1792073232940793883");

    // One row per hold: the sustained one bold, the broken one marked with
    // its signal, the container CPU beside the machine's, the raw p95 beside
    // the one the verdict read.
    const sustained = screen.getByTestId("load-test-tier-sustained");
    expect(within(sustained).getByText("12")).toBeInTheDocument();
    expect(within(sustained).getByText("95% · 98%")).toBeInTheDocument();
    expect(within(sustained).getByText("2%")).toBeInTheDocument();
    expect(within(sustained).getByText("140B · 60B")).toBeInTheDocument();
    expect(within(sustained).getByText("12.0 Mb/s")).toBeInTheDocument();
    expect(within(sustained).getByText("24.0 Mb/s")).toBeInTheDocument();
    expect(within(sustained).getByText("310 ms")).toBeInTheDocument();
    expect(within(sustained).getByText("155 ms")).toBeInTheDocument();
    expect(within(sustained).getByText("admin.loadtest.window.held · admin.loadtest.phase.narrowing")).toBeInTheDocument();
    expect(screen.getAllByTestId("load-test-tier-sustained")).toHaveLength(1);
    const broke = screen.getByTestId("load-test-window-broke");
    expect(within(broke).getByText("16")).toBeInTheDocument();
    expect(within(broke).getByText("admin.loadtest.window.brokeCpu · admin.loadtest.phase.ramping")).toBeInTheDocument();
    expect(screen.getAllByTestId("load-test-window")).toHaveLength(1);
    expect(screen.queryByTestId("load-test-disk-unmeasured")).toBeNull();

    expect(screen.getByTestId("load-test-predicted")).toHaveTextContent("count=10,measured=8");
    expect(screen.getByTestId("load-test-viewer-mix")).toBeInTheDocument();
  });

  it("the main view says how many, what gives out first and what to do, in plain words; everything technical is folded", () => {
    const report = {
      scenario: "mkvgo",
      sustainedViewers: 101,
      rangeLow: 91,
      rangeHigh: 111,
      stopReason: "slo_breach",
      limiter: "disk",
      limiterLocation: "/mnt/disk_1",
      advice: "spread_titles",
      diskReadMeasured: true,
      seed: "42",
      endpoint: "http://127.0.0.1:9091",
      grid: [
        { viewers: 91, windows: 4, passed: 4, held: true, resources: { cpuPercentAvg: 4.4, processRssBytesAvg: 400, diskReadBytesPerSec: 50_000_000, servedBytesPerSec: 48_000_000 } },
        { viewers: 101, windows: 5, passed: 4, held: true, resources: { cpuPercentAvg: 5.1, processRssBytesAvg: 450, diskReadBytesPerSec: 60_000_000, servedBytesPerSec: 58_000_000 } },
        { viewers: 111, windows: 5, passed: 3, held: false, reason: "slo_breach", resources: { cpuPercentAvg: 5.6, processRssBytesAvg: 470, diskReadBytesPerSec: 70_000_000, servedBytesPerSec: 60_000_000 } },
      ],
      windows: [{ viewers: 101, phase: "measuring", measured: true, resources: { cpuPercentAvg: 5 }, segmentMsP95: 900, segmentMsP95Scaled: 700 }],
      diskDevices: [{ location: "/mnt/disk_1", device: "sda", resolved: "read" }],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    expect(screen.getByTestId("load-test-verdict")).toHaveTextContent("admin.loadtest.verdictRange(count=101,scenario=admin.loadtest.scenarioMkvgoNoun,low=91,high=111)");
    expect(screen.getByTestId("load-test-limiter")).toHaveTextContent("admin.loadtest.limiter.disk(where=admin.loadtest.limiter.diskWhere(location=/mnt/disk_1))");
    expect(screen.getByTestId("load-test-advice")).toHaveTextContent("admin.loadtest.advice.spreadTitles");
    const answer = screen.getByTestId("load-test-grid-answer");
    expect(within(answer).getByText("101")).toBeInTheDocument();
    expect(within(answer).getByText("5%")).toBeInTheDocument();
    expect(within(answer).getByText("450B")).toBeInTheDocument();
    expect(within(answer).getByText("480.0 Mb/s")).toBeInTheDocument();
    expect(within(answer).getByText("464.0 Mb/s")).toBeInTheDocument();
    expect(within(answer).getByText("admin.loadtest.grid.ok")).toBeInTheDocument();
    const rows = screen.getAllByTestId("load-test-grid-row");
    expect(within(rows[1]!).getByText("admin.loadtest.grid.ko")).toBeInTheDocument();
    // The main table has six columns, no technical one.
    expect(within(screen.getByTestId("load-test-grid")).getAllByRole("columnheader")).toHaveLength(6);
    const details = screen.getByTestId("load-test-details");
    for (const id of ["load-test-verdict", "load-test-limiter", "load-test-advice", "load-test-grid"]) {
      expect(details.contains(screen.getByTestId(id))).toBe(false);
    }
    for (const id of ["load-test-stop-reason", "load-test-windows", "load-test-endpoint", "load-test-seed", "load-test-grid-windows", "load-test-device"]) {
      expect(details.contains(screen.getAllByTestId(id)[0]!)).toBe(true);
    }
    expect(screen.getByTestId("load-test-grid-windows")).toHaveTextContent("viewers=111,passed=3,windows=5");
  });

  it("a run that hit the viewer ceiling says no bottleneck; an opaque mount replaces the disk lines", () => {
    const report = { scenario: "copy", sustainedViewers: 40, stopReason: "max_viewers", storageOpaque: true, storageFsType: "fuse.mergerfs", diskReadStarved: true, diskReadMeasured: false, coldCache: false } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    expect(screen.getByTestId("load-test-stop-reason").className).toContain("text-muted");
    expect(screen.getByTestId("load-test-no-bottleneck")).toBeInTheDocument();
    expect(screen.getByTestId("load-test-storage-opaque")).toHaveTextContent("storageOpaqueLine(fs=fuse.mergerfs");
    expect(screen.queryByTestId("load-test-disk-starved")).toBeNull();
    expect(screen.queryByTestId("load-test-cold-cache")).toBeNull();
    expect(screen.queryByTestId("load-test-tiers")).toBeNull();
    expect(screen.queryByTestId("load-test-windows")).toBeNull();
    expect(screen.queryByTestId("load-test-predicted")).toBeNull();
  });

  it("names the tier the cause was read on, and says when no disk could be mapped", () => {
    const report = {
      scenario: "mkvgo",
      sustainedViewers: 126,
      stopReason: "slo_breach",
      breakViewers: 131,
      breakReason: "slo_breach",
      diskReadMeasured: false,
      resources: { samples: 6, cpuPercentAvg: 9.2, machineCpuPercentAvg: 14.5, machineIowaitPercentAvg: 65.1, memUsedBytesAvg: 923, processRssBytesAvg: 462 },
      windows: [{ viewers: 126, phase: "measuring", measured: true, resources: { cpuPercentAvg: 9, servedBytesPerSec: 1_000_000 } }],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    expect(screen.getByTestId("load-test-cause")).toHaveTextContent("admin.loadtest.cause.tierBroke(tier=131)");
    expect(screen.getByTestId("load-test-machine")).toHaveTextContent("machineLine(tier=126,cpu=9,machine=15,iowait=65,mem=923B,rss=462B)");
    expect(screen.getByTestId("load-test-disk-unmeasured")).toBeInTheDocument();
    expect(screen.queryByTestId("load-test-disk-starved")).toBeNull();
    const row = screen.getByTestId("load-test-tier-sustained");
    expect(within(row).getByText("admin.loadtest.notMeasured")).toBeInTheDocument();
    expect(screen.getByTestId("load-test-app-read")).toHaveTextContent("appReadUnmeasuredLine");
  });

  it("a pooled run says where the audience sat and what each disk did, busiest first", () => {
    const report = {
      scenario: "mkvgo",
      sustainedViewers: 46,
      stopReason: "slo_breach",
      storageOpaque: true,
      storageFsType: "fuse.mergerfs",
      poolBranches: [
        { branch: "/mnt/disk_2", viewers: 41 },
        { branch: "/mnt/disk_1", viewers: 9 },
      ],
      diskReadMeasured: true,
      breakViewers: 48,
      breakDisks: [{ device: "sdc", readBytesPerSecAvg: 38 * 125000, readBytesPerSecPeak: 60 * 125000 }],
      disks: [
        { device: "sdc", utilPercentAvg: 88.6, utilPercentPeak: 97, readBytesPerSecAvg: 104 * 1048576, readBytesPerSecPeak: 150 * 1048576, awaitMsAvg: 132.4, awaitMsPeak: 410, queueDepthAvg: 7.25 },
        { device: "sdb", utilPercentAvg: 31, utilPercentPeak: 55, readBytesPerSecAvg: 30 * 1048576, awaitMsAvg: 18 },
      ],
      diskDevices: [
        { location: "/mnt/disk_2", device: "sdc", resolved: "read" },
        { location: "/mnt/disk_3", device: "", resolved: "" },
      ],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    const pool = screen.getByTestId("load-test-pool");
    const branches = within(pool).getAllByTestId("load-test-pool-branch");
    expect(branches).toHaveLength(2);
    expect(branches[0]).toHaveTextContent("branch=/mnt/disk_2");
    expect(branches[0]).toHaveTextContent("pct=82");
    expect(branches[1]).toHaveTextContent("pct=18");
    const disks = within(pool).getAllByTestId("load-test-disk");
    expect(disks).toHaveLength(2);
    expect(disks[0]).toHaveTextContent("device=sdc");
    expect(disks[0]).toHaveTextContent("util=89");
    expect(disks[0]).toHaveTextContent("peak=97");
    expect(disks[0]).toHaveTextContent("wait=132");
    expect(disks[0]).toHaveTextContent("waitPeak=410");
    expect(disks[0]).toHaveTextContent("queue=7.3");
    // The mapping says which disk each location was read from, and which
    // could not be tied to one; the capacity at the break is per disk.
    const devices = within(pool).getAllByTestId("load-test-device");
    expect(devices[0]).toHaveTextContent("deviceLine(location=/mnt/disk_2,device=sdc)");
    expect(devices[1]).toHaveTextContent("deviceUnresolvedLine(location=/mnt/disk_3)");
    expect(screen.getByTestId("load-test-break-capacity")).toHaveTextContent("tier=48,parts=admin.loadtest.breakCapacityPart(device=sdc,rate=38.0 Mb/s,peak=60.0 Mb/s)");
  });

  it("the Go runtime line says how much of the CPU the collector took, and is absent when unmeasured", () => {
    const report = {
      sustainedViewers: 29,
      stopReason: "slo_breach",
      resources: { goHeapLiveBytesAvg: 6000, goHeapLiveBytesMax: 8000, goMemLimitBytes: 9000, goGcCpuPercentAvg: 31.6, goGcCpuPercentPeak: 58.2 },
    } as LoadTestReport;
    const { unmount } = render(<LoadTestReportPanel report={report} predicted={undefined} />);
    const line = screen.getByTestId("load-test-go-runtime");
    expect(line).toHaveTextContent("heap=6000B");
    expect(line).toHaveTextContent("limit=9000B");
    expect(line).toHaveTextContent("gc=32");
    expect(line).toHaveTextContent("peak=58");
    unmount();
    render(<LoadTestReportPanel report={{ sustainedViewers: 3, stopReason: "cpu_saturated" } as LoadTestReport} predicted={undefined} />);
    expect(screen.queryByTestId("load-test-go-runtime")).toBeNull();
  });

  it("a run on plain storage shows no pool block", () => {
    render(<LoadTestReportPanel report={{ sustainedViewers: 3, stopReason: "cpu_saturated" } as LoadTestReport} predicted={undefined} />);
    expect(screen.queryByTestId("load-test-pool")).toBeNull();
  });

  it("an unknown stop reason falls back to the error verdict", () => {
    render(<LoadTestReportPanel report={{ sustainedViewers: 0 } as LoadTestReport} predicted={undefined} />);
    expect(screen.getByTestId("load-test-stop-reason")).toHaveTextContent("admin.loadtest.stopReason.error");
    expect(screen.getByTestId("load-test-verdict")).toHaveTextContent(/count=0,scenario=/);
  });

  it("names a client mix in the headline and what the server did for each client", () => {
    const report: LoadTestReport = {
      scenario: "clients:tv=1,web-chrome=1",
      sustainedViewers: 40,
      stopReason: "throughput_saturated",
      servedPaths: [
        { viewerType: "client:tv", path: "mkvgo", viewers: 20 },
        { viewerType: "client:web-chrome", path: "transcode", viewers: 4 },
      ],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    expect(screen.getByText(/scenario=admin.loadtest.scenarioClientsNoun\(mix=admin.loadtest.clientShare\(pct=50,label=admin.loadtest.client.tv\)/)).toBeInTheDocument();
    const paths = screen.getByTestId("load-test-served-paths");
    expect(paths).toHaveTextContent("label=admin.loadtest.client.tv,parts=admin.loadtest.mixPart(count=20,label=admin.loadtest.path.mkvgo)");
    expect(paths).toHaveTextContent("label=admin.loadtest.client.webChrome,parts=admin.loadtest.mixPart(count=4,label=admin.loadtest.path.transcode)");
  });
});
