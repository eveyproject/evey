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
      tiers: [
        { viewers: 8, cpuPercent: 61.2, memUsedBytes: 100, diskReadBytesPerSec: 1_000_000, servedBytesPerSec: 2_000_000, cpuPercentPerViewer: 7.65, memBytesPerViewer: 12 },
        { viewers: 12, cpuPercent: 94.7, memUsedBytes: 140, diskReadBytesPerSec: 1_500_000, servedBytesPerSec: 3_000_000, cpuPercentPerViewer: 7.9, memBytesPerViewer: 11 },
      ],
    } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={10} />);

    expect(screen.getByText("admin.loadtest.reportHeadline(count=12,scenario=admin.loadtest.scenarioBalancedNoun)")).toBeInTheDocument();
    const verdict = screen.getByTestId("load-test-stop-reason");
    expect(verdict).toHaveTextContent("admin.loadtest.stopReason.cpuSaturated");
    expect(verdict.className).toContain("font-medium");
    expect(screen.queryByTestId("load-test-no-bottleneck")).toBeNull();
    expect(screen.getByTestId("load-test-cold-cache")).toBeInTheDocument();
    expect(screen.getByTestId("load-test-disk-starved")).toHaveTextContent("coldCacheStillStarvedLine");
    expect(screen.getByTestId("load-test-storage-collapse")).toHaveTextContent("count=9");

    const sustained = screen.getByTestId("load-test-tier-sustained");
    expect(within(sustained).getByText("12")).toBeInTheDocument();
    expect(within(sustained).getByText("95%")).toBeInTheDocument();
    expect(within(sustained).getByText("12.0 Mb/s")).toBeInTheDocument();
    expect(within(sustained).getByText("24.0 Mb/s")).toBeInTheDocument();
    expect(screen.getAllByTestId("load-test-tier-sustained")).toHaveLength(1);

    expect(screen.getByTestId("load-test-predicted")).toHaveTextContent("count=10,measured=8");
    expect(screen.getByTestId("load-test-viewer-mix")).toBeInTheDocument();
  });

  it("a run that hit the viewer ceiling says no bottleneck; an opaque mount replaces the disk lines", () => {
    const report = { scenario: "copy", sustainedViewers: 40, stopReason: "max_viewers", storageOpaque: true, storageFsType: "fuse.mergerfs", diskReadStarved: true, coldCache: false } as LoadTestReport;
    render(<LoadTestReportPanel report={report} predicted={undefined} />);
    expect(screen.getByTestId("load-test-stop-reason").className).toContain("text-muted");
    expect(screen.getByTestId("load-test-no-bottleneck")).toBeInTheDocument();
    expect(screen.getByTestId("load-test-storage-opaque")).toHaveTextContent("storageOpaqueLine(fs=fuse.mergerfs");
    expect(screen.queryByTestId("load-test-disk-starved")).toBeNull();
    expect(screen.queryByTestId("load-test-cold-cache")).toBeNull();
    expect(screen.queryByTestId("load-test-tiers")).toBeNull();
    expect(screen.queryByTestId("load-test-predicted")).toBeNull();
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
      disks: [
        { device: "sdc", utilPercentAvg: 88.6, utilPercentPeak: 97, readBytesPerSecAvg: 104 * 1048576, awaitMsAvg: 132.4 },
        { device: "sdb", utilPercentAvg: 31, utilPercentPeak: 55, readBytesPerSecAvg: 30 * 1048576, awaitMsAvg: 18 },
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
  });

  it("a run on plain storage shows no pool block", () => {
    render(<LoadTestReportPanel report={{ sustainedViewers: 3, stopReason: "cpu_saturated" } as LoadTestReport} predicted={undefined} />);
    expect(screen.queryByTestId("load-test-pool")).toBeNull();
  });

  it("an unknown stop reason falls back to the error verdict", () => {
    render(<LoadTestReportPanel report={{ sustainedViewers: 0 } as LoadTestReport} predicted={undefined} />);
    expect(screen.getByTestId("load-test-stop-reason")).toHaveTextContent("admin.loadtest.stopReason.error");
    expect(screen.getByText(/count=0,scenario=/)).toBeInTheDocument();
  });
});
