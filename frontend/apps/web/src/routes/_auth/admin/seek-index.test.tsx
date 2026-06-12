import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockScan = vi.fn();
const mockRefetch = vi.fn();
const repairOneMutate = vi.fn();
const rollbackOneMutate = vi.fn();
const repairLibMutate = vi.fn();
const rollbackLibMutate = vi.fn();
const mockToast = { success: vi.fn(), error: vi.fn(), info: vi.fn() };

vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-1", name: "Films" }] } }),
  useAdminMediaServiceScanSeekIndex: (...args: unknown[]) => mockScan(...args),
  useAdminMediaServiceReindexMediaSeekIndex: () => ({ mutate: repairOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceRollbackMediaSeekIndex: () => ({ mutate: rollbackOneMutate, isPending: false, variables: undefined }),
  useAdminMediaServiceReindexLibrarySeekIndex: () => ({ mutate: repairLibMutate, isPending: false }),
  useAdminMediaServiceRollbackLibrarySeekIndex: () => ({ mutate: rollbackLibMutate, isPending: false }),
}));

vi.mock("@evey/store", () => ({ toast: mockToast }));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

const scanned = {
  data: {
    items: [
      { mediaId: "mi-1", title: "Movie A", path: "/media/a.mkv", hasIndex: false, hasBackup: false, sizeBytes: "1073741824", container: "mkv" },
      { mediaId: "mi-2", title: "Movie B", path: "/media/b.mkv", hasIndex: true, hasBackup: true, sizeBytes: "2147483648", container: "mkv" },
      { mediaId: "mi-3", title: "Movie C", path: "/media/c.webm", hasIndex: true, hasBackup: false, sizeBytes: "1024", container: "webm" },
    ],
    total: 3,
    missingIndex: 1,
    withBackup: 1,
  },
  isFetching: false,
  isError: false,
  error: null,
  refetch: mockRefetch,
};

const notScanned = { data: undefined, isFetching: false, isError: false, error: null, refetch: mockRefetch };

async function getPage() {
  const mod = await import("./seek-index.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("SeekIndexPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockScan.mockReturnValue(notScanned);
  });

  it("renders heading, read-only note and scan button before any scan", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByText("Seek Index Repair")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-ro-note")).toHaveTextContent(/read-only/i);
    expect(screen.getByTestId("seek-index-scan-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-repair-all-btn")).not.toBeInTheDocument();
  });

  it("clicking Scan triggers a refetch", async () => {
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByTestId("seek-index-scan-btn"));
    expect(mockRefetch).toHaveBeenCalled();
  });

  it("after a scan, shows rollups and only files needing attention", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    expect(screen.getByTestId("seek-index-missing-count")).toHaveTextContent("1");
    // mi-3 is healthy (indexed, no backup) → hidden by the default filter.
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(2);
    expect(screen.getByText("Movie A")).toBeInTheDocument();
    expect(screen.queryByText("Movie C")).not.toBeInTheDocument();
  });

  it("unchecking the attention filter reveals every scanned file", async () => {
    mockScan.mockReturnValue(scanned);
    const Page = await getPage();
    render(React.createElement(Page));
    fireEvent.click(screen.getByRole("checkbox"));
    expect(screen.getAllByTestId("seek-index-row")).toHaveLength(3);
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
});
