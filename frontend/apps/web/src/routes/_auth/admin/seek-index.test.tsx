import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
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
const invalidateQueries = vi.fn();
const mockToast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };

vi.mock("@evey/api", () => ({
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
    ],
    total: 4,
    missingIndex: 1,
    withBackup: 1,
    audioDelayed: 1,
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
    expect(screen.getByTestId("seek-index-missing-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-audio-delay-count")).toHaveTextContent("1");
    expect(screen.getByTestId("seek-index-scanned-at")).toBeInTheDocument();
    // mi-3 is healthy (indexed, no backup, aligned audio) → hidden by the
    // default filter; mi-4's delayed audio keeps it visible.
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(3);
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.getByText("Movie D")).toBeInTheDocument();
    expect(screen.queryByText("Movie C")).not.toBeInTheDocument();
  });

  it("a delayed-audio file shows the badge with the probed seconds", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Audio +0.9s")).toBeInTheDocument();
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
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(4);
    expect(screen.getByText("Movie C")).toBeInTheDocument();
  });

  it("row repair button calls the single-file mutation", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-repair-btn"));
    expect(repairOneMutate).toHaveBeenCalledWith({ mediaId: "mi-1" });
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
    expect(repairLibMutate).toHaveBeenCalledWith({ data: {} });
    expect(rollbackLibMutate).not.toHaveBeenCalled();
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
