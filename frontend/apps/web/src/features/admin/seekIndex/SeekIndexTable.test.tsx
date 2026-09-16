import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";
import { SeekIndexTable } from "./SeekIndexTable";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children, ...p }: any) => React.createElement("span", { "data-testid": p["data-testid"] }, children) }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-testid": p["data-testid"] }, children),
}));
vi.mock("@/components/ui/EmptyState", () => ({ EmptyState: ({ title }: any) => React.createElement("div", { "data-testid": "empty" }, title) }));
vi.mock("@/components/ui/Pagination", () => ({ Pagination: () => null }));
vi.mock("./RepairHistoryPanel", () => ({ RepairHistoryPanel: ({ mediaId }: any) => React.createElement("div", null, `history:${mediaId}`) }));
vi.mock("./SyncPreviewPanel", () => ({ SyncPreviewPanel: ({ item }: any) => React.createElement("div", null, `preview:${item.mediaId}`) }));

const mutation = () => ({ mutate: vi.fn(), isPending: false, variables: undefined });

function mount(items: any[], over: Record<string, unknown> = {}) {
  const props = {
    items, visible: items, paged: items, page: 1, setPage: vi.fn(), totalPages: 1, busy: false, containerFixAvailable: true,
    repairOne: mutation(), rollbackOne: mutation(), deleteBackup: mutation(), fixContainerOne: mutation(), ackOne: mutation(),
    ...over,
  };
  render(<SeekIndexTable {...(props as any)} />);
  return props;
}

beforeEach(() => vi.clearAllMocks());

// Each row offers exactly the actions its verdict allows: a missing index
// repairs (auto) or resyncs, a damaged stretch resyncs only, a wrong
// container gets the container fix, an informational verdict is acknowledged
// (never repaired), and only a file with a backup can be rolled back or have
// its backup deleted. Busy freezes the repairs but not the acknowledgment.
describe("SeekIndexTable", () => {
  it("maps each verdict to its actions", () => {
    const rows = [
      { mediaId: "missing", path: "/m/a.mkv", title: "A", container: "mkv", sizeBytes: "10", verdict: "repairable", hasIndex: false },
      { mediaId: "damaged", path: "/m/b.mkv", title: "B", container: "mkv", verdict: "repairable", hasIndex: true, damagedBytes: "4096" },
      { mediaId: "wrongc", path: "/m/c.mkv", title: "C", container: "mkv", verdict: "repairable", hasIndex: false, wrongContainer: true },
      { mediaId: "info", path: "/m/d.mkv", title: "D", container: "mkv", verdict: "informational", hasIndex: true, hasBackup: true, audioDelaySec: 0.5 },
      { mediaId: "acked", path: "/m/e.mkv", title: "E", container: "mkv", verdict: "informational", hasIndex: true, ackedAtMs: "1758000000000" },
    ];
    const p = mount(rows);
    const byId = Object.fromEntries(screen.getAllByTestId("seek-index-row").map((r) => [within(r).getByText(/^[A-E]$/).textContent, r]));

    within(byId.A).getByTestId("seek-index-repair-btn").click();
    expect(p.repairOne.mutate).toHaveBeenCalledWith({ mediaId: "missing", data: { mode: "auto" } });
    expect(within(byId.A).queryByTestId("seek-index-fix-container-btn")).toBeNull();
    expect(within(byId.A).queryByTestId("seek-index-ack-btn")).toBeNull();

    expect(within(byId.B).queryByTestId("seek-index-repair-btn")).toBeNull();
    within(byId.B).getByTestId("seek-index-repair-resync-btn").click();
    expect(p.repairOne.mutate).toHaveBeenCalledWith({ mediaId: "damaged", data: { resync: true } });

    expect(within(byId.C).queryByTestId("seek-index-repair-btn")).toBeNull();
    within(byId.C).getByTestId("seek-index-fix-container-btn").click();
    expect(p.fixContainerOne.mutate).toHaveBeenCalledWith({ mediaId: "wrongc" });

    expect(within(byId.D).queryByTestId("seek-index-repair-btn")).toBeNull();
    within(byId.D).getByTestId("seek-index-ack-btn").click();
    expect(p.ackOne.mutate).toHaveBeenCalledWith({ data: { mediaId: "info", acked: true } });
    within(byId.D).getByTestId("seek-index-rollback-btn").click();
    expect(p.rollbackOne.mutate).toHaveBeenCalledWith({ mediaId: "info" });
    within(byId.D).getByTestId("seek-index-delete-backup-btn").click();
    expect(p.deleteBackup.mutate).toHaveBeenCalledWith({ data: { mediaId: "info" } });
    expect(within(byId.A).queryByTestId("seek-index-rollback-btn")).toBeNull();

    expect(within(byId.E).getByTestId("seek-index-acked-badge")).toBeInTheDocument();
    within(byId.E).getByTestId("seek-index-unack-btn").click();
    expect(p.ackOne.mutate).toHaveBeenCalledWith({ data: { mediaId: "acked", acked: false } });
  });

  it("busy freezes the repairs but not the acknowledgment; one journal and one preview at a time", () => {
    const rows = [
      { mediaId: "m1", path: "/m/a.mkv", title: "A", container: "mkv", verdict: "repairable", hasIndex: false },
      { mediaId: "m2", path: "/m/b.mkv", title: "B", container: "mkv", verdict: "informational", hasIndex: true },
    ];
    mount(rows, { busy: true });
    const [a, b] = screen.getAllByTestId("seek-index-row");
    expect((within(a).getByTestId("seek-index-repair-btn") as HTMLButtonElement).disabled).toBe(true);
    expect((within(b).getByTestId("seek-index-ack-btn") as HTMLButtonElement).disabled).toBe(false);

    fireEvent.click(within(a).getByTestId("seek-index-history-btn"));
    expect(screen.getByText("history:m1")).toBeInTheDocument();
    fireEvent.click(within(b).getByTestId("seek-index-history-btn"));
    expect(screen.queryByText("history:m1")).toBeNull();
    expect(screen.getByText("history:m2")).toBeInTheDocument();
    fireEvent.click(within(b).getByTestId("seek-index-history-btn"));
    expect(screen.queryByText("history:m2")).toBeNull();
    fireEvent.click(within(a).getByTestId("seek-index-sync-preview-btn"));
    expect(screen.getByText("preview:m1")).toBeInTheDocument();
  });

  it("says when the library has no EBML file and when the view is empty", () => {
    mount([]);
    expect(screen.getByTestId("empty")).toHaveTextContent("admin.seekIndex.noEbml");
    mount([{ mediaId: "m1", path: "/m/a.mkv", verdict: "healthy", hasIndex: true }], { visible: [], paged: [] });
    expect(screen.getAllByTestId("empty").at(-1)).toHaveTextContent("admin.seekIndex.noneInView");
  });
});
