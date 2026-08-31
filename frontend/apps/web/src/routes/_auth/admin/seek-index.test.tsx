import React from "react";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockScan = vi.fn();
const mockJob = vi.fn();
const startScanMutate = vi.fn();
const repairOneMutate = vi.fn();
const rollbackOneMutate = vi.fn();
const repairLibMutate = vi.fn();
const rollbackLibMutate = vi.fn();
const deleteBackupMutate = vi.fn();
const purgeLibMutate = vi.fn();
const remuxAudioOneMutate = vi.fn();
const remuxAudioLibMutate = vi.fn();
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
  useAdminMediaServiceReindexMediaSeekIndex: () => ({ mutate: repairOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceRollbackMediaSeekIndex: () => ({ mutate: rollbackOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceReindexLibrarySeekIndex: () => ({ mutate: repairLibMutate, isPending: false }),
  useAdminMediaServiceRollbackLibrarySeekIndex: () => ({ mutate: rollbackLibMutate, isPending: false }),
  useAdminMediaServiceDeleteSeekIndexBackup: () => ({ mutate: deleteBackupMutate, isPending: false, variables: undefined }),
  useAdminMediaServicePurgeSeekIndexBackups: () => ({ mutate: purgeLibMutate, isPending: false }),
  useAdminMediaServiceRemuxAudioMedia: () => ({ mutate: remuxAudioOneMutate, isPending: false }),
  useAdminMediaServiceRemuxAudioLibrary: () => ({ mutate: remuxAudioLibMutate, isPending: false }),
  useAdminMediaServiceAckSeekIndexMedia: () => ({ mutate: ackOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceAckSeekIndexGroup: () => ({ mutate: ackGroupMutate, isPending: false }),
  useAdminMediaServiceFixContainerMedia: () => ({ mutate: fixContainerMutate, isPending: false, variables: undefined }),
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
  const mod = await import("./seek-index.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("SeekIndexPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockScan.mockReturnValue(notScanned);
    mockJob.mockReturnValue(idleJob);
  });

  it("renders heading, read-only note and scan button before any scan", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Seek Index Repair")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-ro-note")).toHaveTextContent(/read-only/i);
    expect(screen.getByTestId("seek-index-scan-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-repair-all-btn")).not.toBeInTheDocument();
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
    // repairable, mi-7 (truncated) needs the operator, mi-6 (short audio) is
    // information. The truncated file is NOT counted as "missing index": a
    // rebuild would be wasted until the source is re-acquired.
    expect(screen.getByTestId("seek-index-repairable-count")).toHaveTextContent("2");
    expect(screen.getByTestId("seek-index-manual-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-informational-count")).toHaveTextContent("1");
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
    // missingIndex=2 but one of them is truncated (skipped by the batch):
    // 1 repairable + 1 delayed audio.
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (2)");
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
          { mediaId: "mi-8", title: "Movie G", path: "/media/g.mkv", hasIndex: false, hasBackup: false, sizeBytes: "4096", container: "mkv", seekCoarse: true },
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
    // Its own badge, NOT the repairable "Missing" one.
    expect(within(row).getByText("Coarse seeking")).toBeInTheDocument();
    expect(within(row).queryByText("Missing")).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-btn"]')).toBeNull();
    expect(row.querySelector('[data-testid="seek-index-repair-resync-btn"]')).toBeNull();
    // Unchanged by the extra flagged file: the batch cannot act on it.
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (2)");
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

  it("a delayed-audio file shows the badge with the probed seconds", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Audio +0.9s")).toBeInTheDocument();
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

  it("unchecking the attention filter reveals every scanned file", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(6);
    expect(screen.getByText("Movie C")).toBeInTheDocument();
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
    expect(screen.getByTestId("seek-index-informational-count")).toHaveTextContent("0");
    expect(screen.getByTestId("seek-index-acked-count")).toHaveTextContent("1");
    expect(screen.queryByText("Movie E")).not.toBeInTheDocument();
    // Untick the filter: the row is back, marked seen, with the way back.
    fireEvent.click(screen.getByRole("checkbox"));
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
    // ...while the incomplete download can only be acknowledged.
    const truncatedRow = screen.getByText("Movie F").closest("tr")!;
    expect(within(truncatedRow).getByTestId("seek-index-verdict")).toHaveTextContent("Your call");
    expect(within(truncatedRow).getByTestId("seek-index-ack-btn")).toBeInTheDocument();
  });

  it("the group button acknowledges every pending file of that verdict in scope", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-ack-manual-btn"));
    expect(ackGroupMutate).toHaveBeenCalledWith({ data: { verdict: "manual" } });
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
    expect(screen.getByTestId("seek-index-repair-all-btn")).toHaveTextContent("Repair all (3)");
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
    expect(screen.getByTestId("seek-index-confirm-text")).toHaveTextContent("Repair 2 file(s)");
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
