import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockScan = vi.fn();
const mockJob = vi.fn();
const mockJournal = vi.fn(() => ({ data: { entries: [] }, isLoading: false }));
const startScanMutate = vi.fn();
const repairOneMutate = vi.fn();
const rollbackOneMutate = vi.fn();
const repairLibMutate = vi.fn();
const rollbackLibMutate = vi.fn();
const deleteBackupMutate = vi.fn();
const purgeLibMutate = vi.fn();
const ackOneMutate = vi.fn();
const ackGroupMutate = vi.fn();
const fixContainerMutate = vi.fn();
const invalidateQueries = vi.fn();
const mockToast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-1", name: "Films" }] } }),
  useAdminMediaServiceScanSeekIndex: (...args: unknown[]) => mockScan(...args),
  useAdminMediaServiceGetSeekIndexJob: (...args: unknown[]) => mockJob(...args),
  useAdminMediaServiceStartSeekIndexScan: () => ({ mutate: startScanMutate, isPending: false }),
  useAdminMediaServiceCancelSeekIndexJob: () => ({ mutate: vi.fn(), isPending: false, isSuccess: false }),
  useAdminMediaServiceReindexMediaSeekIndex: () => ({ mutate: repairOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceRollbackMediaSeekIndex: () => ({ mutate: rollbackOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceReindexLibrarySeekIndex: () => ({ mutate: repairLibMutate, isPending: false }),
  useAdminMediaServiceRollbackLibrarySeekIndex: () => ({ mutate: rollbackLibMutate, isPending: false }),
  useAdminMediaServiceDeleteSeekIndexBackup: () => ({ mutate: deleteBackupMutate, isPending: false, variables: undefined }),
  useAdminMediaServicePurgeSeekIndexBackups: () => ({ mutate: purgeLibMutate, isPending: false }),
  useAdminMediaServiceAckSeekIndexMedia: () => ({ mutate: ackOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceAckSeekIndexGroup: () => ({ mutate: ackGroupMutate, isPending: false }),
  useAdminMediaServiceFixContainerMedia: () => ({ mutate: fixContainerMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceReinterleaveMedia: () => ({ mutate: vi.fn(), isPending: false, variables: undefined }),
  useAdminMediaServiceListMediaRepairs: (...args: unknown[]) => mockJournal(...args),
}));

vi.mock("@evey/store", () => ({ toast: mockToast }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries }),
}));

const scanned = {
  data: {
    items: [
      { mediaId: "mi-1", title: "Movie A", path: "/media/a.mkv", hasIndex: false, hasBackup: false, sizeBytes: "1073741824", container: "mkv" },
      { mediaId: "mi-2", title: "Movie B", path: "/media/b.mkv", hasIndex: true, hasBackup: true, sizeBytes: "2147483648", container: "mkv" },
      { mediaId: "mi-3", title: "Movie C", path: "/media/c.webm", hasIndex: true, hasBackup: false, sizeBytes: "1024", container: "webm" },
      { mediaId: "mi-4", title: "Movie D", path: "/media/d.mkv", hasIndex: true, hasBackup: false, sizeBytes: "4096", container: "mkv", audioDelaySec: 0.9 },
      { mediaId: "mi-6", title: "Movie E", path: "/media/e.mkv", hasIndex: true, hasBackup: false, sizeBytes: "8192", container: "mkv", audioShortfallSec: 249.6 },
      { mediaId: "mi-7", title: "Movie F", path: "/media/f.mkv", hasIndex: false, hasBackup: false, sizeBytes: "2219689920", container: "mkv", truncated: true, missingTailBytes: "41080408" },
    ],
    total: 6,
    missingIndex: 2,
    withBackup: 1,
    audioDelayed: 1,
    audioShort: 1,
    truncated: 1,
    scannedAtMs: "1765584000000",
  },
  isError: false,
  error: null,
};

const notScanned = { data: { items: [], scannedAtMs: "0" }, isError: false, error: null };

const idleJob = { data: { running: false }, queryKey: ["job"] };
const runningRepairJob = {
  data: {
    running: true,
    kind: "reindex",
    phase: "processing",
    done: 3,
    failed: 1,
    total: 10,
    currentPath: "/media/a.mkv",
  },
  queryKey: ["job"],
};
const finishedWithFailures = {
  data: {
    running: false,
    last: {
      kind: "reindex",
      done: 5,
      failed: 2,
      total: 5,
      readOnly: true,
      failures: [
        { path: "/media/x.mkv", error: "open /media/x.mkv: read-only file system", readOnly: true },
      ],
    },
  },
  queryKey: ["job"],
};

async function getPage() {
  const mod = await import("./media-health.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("SeekIndexPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockScan.mockReturnValue(notScanned);
    mockJob.mockReturnValue(idleJob);
  });

  it("renders heading, the two verdicts and the scan button before any scan", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Media Health")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-intro-repairable")).toHaveTextContent(/Repairable here/i);
    expect(screen.getByTestId("seek-index-intro-informational")).toHaveTextContent(/Information/i);
    expect(screen.getByTestId("seek-index-scan-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-repair-all-btn")).not.toBeInTheDocument();
  });

  // The read-only caveat is troubleshooting, not a headline: it moved into the
  // header's info bubble when the nine-line paragraph was cut down. It must
  // still be REACHABLE - a warning nobody can find is a warning that is gone.
  it("keeps the read-only caveat one click away, in the header info bubble", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.queryByTestId("seek-index-ro-note")).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /More about the two verdicts/i }));
    expect(screen.getByTestId("seek-index-ro-note")).toHaveTextContent(/read-only/i);
  });

  it("clicking Scan starts a background scan job", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-scan-btn"));
    expect(startScanMutate).toHaveBeenCalledWith({ data: {} });
  });

  it("after a scan, shows rollups, scan date and only files needing attention", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    // The three verdict cards: mi-1 (no index) + mi-4 (delayed audio) are
    // repairable; mi-7 (truncated) and mi-6 (short audio) are information.
    // The truncated file is NOT counted as "missing index": a rebuild would
    // be wasted until the source is re-acquired, so it is a remark instead.
    // mi-4 (audio offset) is INFORMATION now, not a repair: an offset is how a
    // dub is aligned and every player honours it.
    expect(screen.getByTestId("seek-index-repairable-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-informational-count")).toHaveTextContent("3");
    expect(screen.getByTestId("seek-index-missing-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-audio-delay-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-truncated-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-scanned-at")).toBeInTheDocument();
    // mi-3 is healthy (indexed, no backup, aligned audio) → hidden by the
    // default filter; mi-4's delayed audio, mi-6's short audio and mi-7's
    // truncation keep them visible.
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(5);
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Movie D")).toBeInTheDocument();
    expect(screen.queryByText("Movie C")).not.toBeInTheDocument();
  });

  it("a short-audio file (track ends before the declared duration) shows its card and badge", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    // Its own rollup card (the file is structurally healthy, so no other
    // counter would ever surface it)...
    expect(screen.getByTestId("seek-index-audio-short-count")).toHaveTextContent("1");
    // ...and the per-row badge carrying the measured gap.
    expect(screen.getByText("Audio -250s")).toBeInTheDocument();
    expect(screen.getByText("Movie E")).toBeInTheDocument();
  });

  it("a truncated file shows the missing-tail magnitude and offers NO repair", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    // The magnitude is what routes the decision (a 9 KB stub vs a missing
    // final scene) - the bare "incomplete" badge hid it.
    expect(screen.getByText("Incomplete - missing tail ~39.2 MB")).toBeInTheDocument();
    // No repair can bring absent bytes back: offering the buttons promised a
    // guaranteed spurious failure (the batch already skips these files).
    const row = screen.getByText("Movie F").closest("tr")!;
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-resync-btn"]')).toBeNull();
  });

  it("Repair all advertises only what the batch will actually repair", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    // missingIndex=2 but one of them is truncated (skipped by the batch), and
    // the audio offset is not a repair at all: one file left to repair.
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (1)");
  });

  // A file rebuilt once and still too coarse to seek is NOT a repair candidate:
  // its cues cannot beat the encode's own keyframes. It used to show as
  // "Missing" with repair buttons, and every "Repair all" queued it for a job
  // that skipped it - the loop that left 134 prod files flagged after a
  // 138-file run.
  it("a coarse-index file gets its own badge, no repair button and no place in the repair count", async () => {
    const coarse = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          { mediaId: "mi-8", title: "Movie G", path: "/media/g.mkv", hasIndex: false, hasBackup: false, sizeBytes: "4096", container: "mkv", seekCoarse: true, seekCoarseDetail: "the video cues leave a 44s hole - a seek there lands that far from its target" },
        ],
        total: 7,
        seekCoarse: 1,
      },
    };
    mockScan.mockReturnValue(coarse);
    const Page = await getPage();
    render(React.createElement(Page));

    expect(screen.getByTestId("seek-index-coarse-count")).toHaveTextContent("1");
    const row = screen.getByText("Movie G").closest("tr")!;
    // The hint carries mkvgo's own account of the hole, not a generic story
    // about keyframes (live, the hole was usually the final credits).
    expect(within(row).getByTestId("seek-index-coarse-hint").getAttribute("title")).toContain("the video cues leave a 44s hole");
    // Its own badge, NOT the repairable "Missing" one.
    expect(within(row).getByText("Coarse seeking")).toBeInTheDocument();
    expect(within(row).queryByText("Missing")).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-resync-btn"]')).toBeNull();
    // Unchanged by the extra flagged file: the batch cannot act on it.
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (1)");
  });

  // A repair walked this file, its source refused to parse, and the server
  // recorded that. The row must say so and route to the ONE repair that can
  // help - offering the strict one again would offer the button that just
  // failed (2026-09-07 prod batch: 3 failures out of 4 files).
  it("a file the strict repair refused shows why and offers only the resync", async () => {
    const refused = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          { mediaId: "mi-9", title: "Movie H", path: "/media/h.mkv", hasIndex: false, hasBackup: false, sizeBytes: "4096", container: "mkv", resyncRequired: true, verdict: "repairable" },
        ],
        total: 7,
      },
    };
    mockScan.mockReturnValue(refused);
    const Page = await getPage();
    render(React.createElement(Page));

    const row = screen.getByText("Movie H").closest("tr")!;
    expect(within(row).getByTestId("seek-index-resync-required-hint")).toBeInTheDocument();
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-resync-btn"]')).not.toBeNull();
    expect(screen.getByTestId("seek-index-resync-required-count")).toHaveTextContent("1");
  });

  // The resync checkbox states a byte total. A file whose corruption was only
  // found by a rewrite has no such figure, so it is counted in the consent but
  // called out apart - a total that covered half the files would be the wrong
  // kind of precise.
  it("says plainly that the byte figure does not cover the repair-discovered files", async () => {
    const refused = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          { mediaId: "mi-9", title: "Movie H", path: "/media/h.mkv", hasIndex: false, hasBackup: false, sizeBytes: "4096", container: "mkv", resyncRequired: true, verdict: "repairable" },
        ],
        total: 7,
      },
    };
    mockScan.mockReturnValue(refused);
    const Page = await getPage();
    render(React.createElement(Page));

    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    expect(screen.getByTestId("seek-index-resync-required-note")).toHaveTextContent(/1 the normal repair refused/i);
  });

  // "138/138 processed" read identically whether the job repaired 138 files or
  // skipped 133 of them. The skipped count is the whole difference.
  it("a job that mostly skipped says so instead of reading as a clean sweep", async () => {
    mockScan.mockReturnValue(scanned);
    mockJob.mockReturnValue({
      data: { running: false, last: { kind: "reindex", done: 138, failed: 1, skipped: 133, total: 138 } },
      queryKey: ["job"],
    });
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByTestId("seek-index-skipped-count")).toHaveTextContent("133 skipped");
  });

  it("an audio offset is stated as a fact, not flagged as a defect", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Audio at +0.9s")).toBeInTheDocument();
  });

  it("tracks measured to disagree get the warning, and say by how much", async () => {
    const disagree = {
      ...scanned,
      data: {
        ...scanned.data,
        items: scanned.data.items.map((it: { mediaId: string }) =>
          it.mediaId === "mi-4"
            ? { ...it, audioDelaySec: 12, audioTracksLagSec: 11.94, audioTracksChecked: true }
            : it,
        ),
      },
    };
    mockScan.mockReturnValue(disagree);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Audio tracks 11.9s apart")).toBeInTheDocument();
    // and it is said, never repaired behind the operator's back
    expect(screen.getByTestId("seek-index-tracks-disagree-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-repairable-count")).toHaveTextContent("1");
  });

  it("a track pulled to 0 by an earlier repair is named, as a remark", async () => {
    const zeroed = {
      ...scanned,
      data: {
        ...scanned.data,
        items: scanned.data.items.map((it: { mediaId: string }) =>
          it.mediaId === "mi-4" ? { ...it, audioDelaySec: 0, audioTrackZeroed: true } : it,
        ),
      },
    };
    mockScan.mockReturnValue(zeroed);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Audio track pulled to 0")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-track-zeroed-hint")).toHaveAttribute("title", expect.stringContaining("earlier audio repair"));
    // nothing to repair here: the amount removed is not in the file any more
    expect(screen.getByTestId("seek-index-repairable-count")).toHaveTextContent("1");
  });

  it("a wrong-container file shows the badge and a wrong-container rollup count", async () => {
    const wc = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          {
            mediaId: "mi-5",
            title: "Movie WC",
            path: "/media/wc.mkv",
            hasIndex: true,
            hasBackup: false,
            sizeBytes: "5242880",
            container: "mkv",
            wrongContainer: true,
          },
        ],
        total: 5,
        wrongContainer: 1,
      },
    };
    mockScan.mockReturnValue(wc);
    const Page = await getPage();
    render(React.createElement(Page));
    // a wrong-container file is NOT missing an index (nothing to rebuild): it
    // must surface its own badge (plus the rollup card, so >= 2 occurrences),
    // never a repair button.
    expect(screen.getByText("Wrong container")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-wrong-container-count")).toHaveTextContent("1");
    // Repairable (the extension is the defect), but the server did not
    // advertise the rename: no button promises what it cannot do.
    const row = screen.getByText("Movie WC").closest("tr")!;
    expect(within(row).getByTestId("seek-index-verdict")).toHaveTextContent("Repairable");
    expect(row.querySelector('[data-testid="seek-index-fix-container-btn"]')).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
  });

  it("a wrong-container file offers the rename when the server can do it", async () => {
    const wc = {
      ...scanned,
      data: {
        ...scanned.data,
        containerFixAvailable: true,
        items: [
          { mediaId: "mi-5", title: "Movie E", path: "/media/e.mkv", hasIndex: true, hasBackup: false, sizeBytes: "5242880", container: "mkv", wrongContainer: true },
        ],
      },
    };
    mockScan.mockReturnValue(wc);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-fix-container-btn"));
    expect(fixContainerMutate).toHaveBeenCalledWith({ mediaId: "mi-5" });
    // ...and it counts in "Repair all", which renames it in the same run.
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (1)");
  });

  it("a trailing-junk file shows a rollup count and its own badge, never a damaged badge", async () => {
    const tj = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          {
            mediaId: "mi-8",
            title: "Movie TJ",
            path: "/media/tj.mkv",
            hasIndex: true,
            hasBackup: false,
            sizeBytes: "6291456",
            container: "mkv",
            trailingJunk: true,
            // complete media: the surplus tail is NOT damage - damagedBytes stays 0
            damagedBytes: "0",
          },
        ],
        total: 5,
        trailingJunk: 1,
      },
    };
    mockScan.mockReturnValue(tj);
    const Page = await getPage();
    render(React.createElement(Page));
    // The rollup card is always visible; the trailing-junk file itself is low
    // priority (media is fine) so its row sits behind the attention filter, like
    // a damaged-but-seekable file.
    expect(screen.getByTestId("seek-index-trailing-junk-count")).toHaveTextContent("1");
    // Information, not a problem: the row shows, says so, and can be acknowledged.
    const row = screen.getByText("Movie TJ").closest("tr")!;
    expect(within(row).getByTestId("seek-index-verdict")).toHaveTextContent("Info");
    expect(within(row).getByText("Trailing bytes")).toBeInTheDocument();
    fireEvent.click(within(row).getByTestId("seek-index-ack-btn"));
    expect(ackOneMutate).toHaveBeenCalledWith({ data: { mediaId: "mi-8", acked: true } });
    // it must never be mislabelled as corrupt
    expect(screen.queryByText(/damaged/i)).not.toBeInTheDocument();
  });

  it("a cached response with scannedAtMs 0 is treated as no scan yet", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.queryByTestId("seek-index-row")).not.toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-repair-all-btn")).not.toBeInTheDocument();
  });

  it("the 'All' view reveals every scanned file", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-view-all"));
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(6);
    expect(screen.getByText("Movie C")).toBeInTheDocument();
  });

  it("view chips carry live counts and narrow the table to one verdict", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByTestId("seek-index-view-attention")).toHaveTextContent("Awaiting (5)");
    expect(screen.getByTestId("seek-index-view-repairable")).toHaveTextContent("Repairable (1)");
    expect(screen.getByTestId("seek-index-view-informational")).toHaveTextContent("Info (3)");
    expect(screen.queryByTestId("seek-index-view-manual")).toBeNull();
    expect(screen.getByTestId("seek-index-view-backups")).toHaveTextContent("Backups (1)");
    expect(screen.getByTestId("seek-index-view-all")).toHaveTextContent("All (6)");
    fireEvent.click(screen.getByTestId("seek-index-view-repairable"));
    const rows = screen.getAllByTestId("seek-index-row");
    expect(rows).toHaveLength(1);
    expect(within(rows[0]).getByText("Movie A")).toBeInTheDocument();
  });

  it("default order puts what needs doing first: repairable, then your call, then info, then backups", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    const titles = screen.getAllByTestId("seek-index-row").map((r) => within(r).getAllByText(/Movie/)[0].textContent);
    expect(titles).toEqual(["Movie A", "Movie F", "Movie E", "Movie D", "Movie B"]);
  });

  it("search narrows by title or path and says what the view holds", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.change(screen.getByTestId("seek-index-search"), { target: { value: "d.mkv" } });
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(1);
    expect(screen.getByText("Movie D")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-page-info")).toHaveTextContent("1-1 of 1");
    fireEvent.change(screen.getByTestId("seek-index-search"), { target: { value: "nothing-like-this" } });
    expect(screen.queryAllByTestId("seek-index-row")).toHaveLength(0);
    expect(screen.getByText("Nothing in this view.")).toBeInTheDocument();
  });

  it("a big library is paged, 50 rows at a time", async () => {
    const many = {
      ...scanned,
      data: {
        ...scanned.data,
        items: Array.from({ length: 120 }, (_, i) => ({
          mediaId: `big-${i}`,
          title: `Big ${String(i).padStart(3, "0")}`,
          path: `/media/big-${i}.mkv`,
          hasIndex: false,
          hasBackup: false,
          sizeBytes: String(1000 - i),
          container: "mkv",
        })),
        total: 120,
      },
    };
    mockScan.mockReturnValue(many);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(50);
    expect(screen.getByTestId("seek-index-page-info")).toHaveTextContent("1-50 of 120");
    expect(screen.getByText("Big 000")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next/i }));
    expect(screen.getByTestId("seek-index-page-info")).toHaveTextContent("51-100 of 120");
    expect(screen.getByText("Big 050")).toBeInTheDocument();
  });

  it("row repair button calls the single-file mutation", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-btn"));
    // The per-file repair uses the safe default strategy ("auto": patch when
    // possible, rewrite otherwise) with a backup kept.
    expect(repairOneMutate).toHaveBeenCalledWith({
      mediaId: "mi-1",
      data: { mode: "auto" },
    });
  });

  it("row rollback button calls the single-file rollback mutation", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-rollback-btn"));
    expect(rollbackOneMutate).toHaveBeenCalledWith({ mediaId: "mi-2" });
  });

  it("repair-all opens the confirm modal and confirming queues the batch", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    // Default: keep backups (space is fine), embark the audio repair, no
    // deep-verify (a backup exists), no rolling window.
    expect(repairLibMutate).toHaveBeenCalledWith({
      data: { mode: "auto", deepVerify: false },
    });
    expect(rollbackLibMutate).not.toHaveBeenCalled();
  });

  it("a nearly-full volume pre-declines backups and shows the advice", async () => {
    mockScan.mockReturnValue({
      ...scanned,
      data: {
        ...scanned.data,
        volumes: [
          {
            root: "/media2/series",
            // 1 GiB free vs 500 GiB of candidates: backups would overflow,
            // and even the largest single temp copy (2 GiB) does not fit.
            freeBytes: String(1 * 1024 ** 3),
            missingBytes: String(500 * 1024 ** 3),
            largestMissingBytes: String(2 * 1024 ** 3),
            missingFiles: 200,
          },
        ],
      },
    });
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    // Evey pre-set the safe choice itself: backups off -> rolling window
    // drained at the end, deep-verify enforced in exchange.
    expect(screen.getByTestId("seek-index-keep-backups")).not.toBeChecked();
    expect(screen.getByTestId("seek-index-volumes")).toHaveTextContent("backups discouraged");
    expect(screen.getByTestId("seek-index-volumes")).toHaveTextContent("too full for the largest file");
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(repairLibMutate).toHaveBeenCalledWith({
      data: { mode: "auto", deepVerify: true, backupWindow: 5 },
    });
  });

  it("declining the backup forces deep-verify and lands in the batch request", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("seek-index-keep-backups"));
    // The rollback copy is gone, so the batch enforces the full-read proof
    // and a rolling window that drains at the end.
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(repairLibMutate).toHaveBeenCalledWith({
      data: { mode: "auto", deepVerify: true, backupWindow: 5 },
    });
  });

  it("rollback-all confirm queues the rollback batch", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-rollback-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(rollbackLibMutate).toHaveBeenCalledWith({ data: {} });
    expect(repairLibMutate).not.toHaveBeenCalled();
  });

  it("an acknowledged file leaves the attention list and its pile, and can be flagged again", async () => {
    const acked = {
      ...scanned,
      data: {
        ...scanned.data,
        items: scanned.data.items.map((it) =>
          it.mediaId === "mi-6" ? { ...it, ackedAtMs: "1765584000000" } : it,
        ),
      },
    };
    mockScan.mockReturnValue(acked);
    const Page = await getPage();
    render(React.createElement(Page));
    // Movie D's audio offset and Movie F's truncation are information too,
    // and neither is acknowledged.
    expect(screen.getByTestId("seek-index-informational-count")).toHaveTextContent("2");
    expect(screen.getByTestId("seek-index-acked-count")).toHaveTextContent("1");
    expect(screen.queryByText("Movie E")).not.toBeInTheDocument();
    // The "Seen" view: the row is back, marked seen, with the way back.
    fireEvent.click(screen.getByTestId("seek-index-view-acked"));
    const row = screen.getByText("Movie E").closest("tr")!;
    expect(within(row).getByTestId("seek-index-acked-badge")).toBeInTheDocument();
    fireEvent.click(within(row).getByTestId("seek-index-unack-btn"));
    expect(ackOneMutate).toHaveBeenCalledWith({ data: { mediaId: "mi-6", acked: false } });
  });

  it("a repairable file cannot be acknowledged - it gets repaired", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    const row = screen.getByText("Movie A").closest("tr")!;
    expect(row.querySelector('[data-testid="seek-index-ack-btn"]')).toBeNull();
    expect(within(row).getByTestId("seek-index-verdict")).toHaveTextContent("Repairable");
    // ...while the incomplete download is only ever mentioned, and waved
    // through when the operator has seen it.
    const truncatedRow = screen.getByText("Movie F").closest("tr")!;
    expect(within(truncatedRow).getByTestId("seek-index-verdict")).toHaveTextContent("Info");
    expect(within(truncatedRow).getByTestId("seek-index-ack-btn")).toBeInTheDocument();
  });

  it("the group button acknowledges every pending file of that verdict in scope", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    // One group, because there is one thing to wave through.
    expect(screen.queryByTestId("seek-index-ack-manual-btn")).toBeNull();
    fireEvent.click(screen.getByTestId("seek-index-ack-informational-btn"));
    expect(ackGroupMutate).toHaveBeenCalledWith({ data: { verdict: "informational" } });
  });

  it("damaged files need explicit consent: asked with the bytes at stake, sent as resyncDamaged", async () => {
    const dmg = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          { mediaId: "mi-9", title: "Movie H", path: "/media/h.mkv", hasIndex: true, hasBackup: false, sizeBytes: "4096", container: "mkv", damagedBytes: "2097152" },
        ],
      },
    };
    mockScan.mockReturnValue(dmg);
    const Page = await getPage();
    render(React.createElement(Page));
    // Counted as repairable (the resync IS the repair)...
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (2)");
    const row = screen.getByText("Movie H").closest("tr")!;
    expect(within(row).getByTestId("seek-index-repair-resync-btn")).toBeInTheDocument();
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    // ...but the loss is spelled out and consented to, on by default.
    const consent = screen.getByTestId("seek-index-resync-damaged");
    expect(consent).toBeChecked();
    expect(screen.getByRole("dialog")).toHaveTextContent("2.0 MB lost");
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(repairLibMutate).toHaveBeenCalledWith({
      data: { mode: "auto", deepVerify: false, resyncDamaged: true },
    });
  });

  it("declining the damaged resync shrinks the promise to what the job will do", async () => {
    const dmg = {
      ...scanned,
      data: {
        ...scanned.data,
        items: [
          ...scanned.data.items,
          { mediaId: "mi-9", title: "Movie H", path: "/media/h.mkv", hasIndex: true, hasBackup: false, sizeBytes: "4096", container: "mkv", damagedBytes: "2097152" },
        ],
      },
    };
    mockScan.mockReturnValue(dmg);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("seek-index-resync-damaged"));
    // Declining the damaged file leaves the single genuine repair.
    expect(screen.getByTestId("seek-index-confirm-text")).toHaveTextContent("Repair 1 file");
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(repairLibMutate).toHaveBeenCalledWith({
      data: { mode: "auto", deepVerify: false },
    });
  });

  it("while a job runs, shows the progress card and disables actions", async () => {
    mockScan.mockReturnValue(scanned);
    mockJob.mockReturnValue(runningRepairJob);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByTestId("seek-index-job-card")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-job-progress")).toHaveTextContent("3/10");
    expect(screen.getByText("a.mkv")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-repair-all-btn")).toBeDisabled();
    expect(screen.getByTestId("seek-index-repair-btn")).toBeDisabled();
  });

  it("when idle, shows the last job summary with failures and read-only hint", async () => {
    mockJob.mockReturnValue(finishedWithFailures);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByTestId("seek-index-last-job")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-ro-hint")).toBeInTheDocument();
    expect(screen.getAllByText(/x\.mkv/).length).toBeGreaterThan(0);
  });
  it("row delete-backup button deletes the single backup synchronously", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-delete-backup-btn"));
    expect(deleteBackupMutate).toHaveBeenCalledWith({ data: { mediaId: "mi-2" } });
  });

  it("purge-all opens the confirm modal and confirming queues the purge job", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-purge-all-btn"));
    fireEvent.click(screen.getByTestId("seek-index-confirm-batch-btn"));
    expect(purgeLibMutate).toHaveBeenCalledWith({ data: {} });
    expect(repairLibMutate).not.toHaveBeenCalled();
    expect(rollbackLibMutate).not.toHaveBeenCalled();
  });

});

describe("repair journal", () => {
  it("opens on demand and lists what the repairs touched, evidence included", async () => {
    mockScan.mockReturnValue(scanned);
    mockJournal.mockReturnValue({
      isLoading: false,
      data: {
        entries: [
          { atMs: "1757800000000", op: "reindex", outcome: "applied", job: "reindex", evidenceMethod: "declared-defect", path: "/media/a.mkv", mediaId: "mi-1" },
          { atMs: "1757800001000", op: "audio_retime", outcome: "rolled_back", job: "", evidenceMethod: "operator", path: "/media/d.mkv", mediaId: "mi-4", error: "the rewrite damaged an audio track" },
        ],
      },
    });
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.queryByTestId("repair-journal-entry")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("repair-journal-toggle"));
    expect(screen.getAllByTestId("repair-journal-entry")).toHaveLength(2);
    expect(screen.getByText("a.mkv")).toBeInTheDocument();
    expect(screen.getByText("Acted on what the file declared about itself, without measuring.")).toBeInTheDocument();
    expect(screen.getByText("Judged by a person.")).toBeInTheDocument();
    expect(screen.getByText("the rewrite damaged an audio track")).toBeInTheDocument();
    // the operator view keeps only what a person did on one file
    fireEvent.change(screen.getByTestId("repair-journal-job"), { target: { value: "operator" } });
    expect(screen.getAllByTestId("repair-journal-entry")).toHaveLength(1);
  });
});
