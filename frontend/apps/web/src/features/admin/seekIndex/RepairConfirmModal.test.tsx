import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { RepairConfirmModal } from "./RepairConfirmModal";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.entries(o).map(([a, b]) => `${a}=${String(b)}`).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children, title }: any) => (open ? React.createElement("div", { role: "dialog", "aria-label": title }, children) : null),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading }: any) => React.createElement("button", { onClick, disabled: loading }, children),
}));
vi.mock("@/components/ui/Checkbox", () => ({
  Checkbox: ({ label, checked, onChange, ...p }: any) =>
    React.createElement("label", null, React.createElement("input", { type: "checkbox", checked, onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.checked), "data-testid": p["data-testid"] }), label),
}));

const mutation = () => ({ mutate: vi.fn(), isPending: false });

function mount(over: Record<string, unknown> = {}) {
  const props = {
    confirmBatch: "repair" as const,
    setConfirmBatch: vi.fn(),
    withBackup: 3,
    repairable: 10,
    repairVolumes: [] as unknown[],
    breakdown: { missing: 6, damaged: 3, damagedBytes: 8192, resyncRequired: 1, delayedAudio: 0, tracksDisagree: 0, wrongContainer: 0 },
    resyncDamaged: true,
    setResyncDamaged: vi.fn(),
    keepBackups: true,
    setKeepBackups: vi.fn(),
    repairLib: mutation(),
    rollbackLib: mutation(),
    purgeLib: mutation(),
    libraryId: "lib-1",
    ...over,
  };
  render(<RepairConfirmModal {...(props as any)} />);
  return props;
}

beforeEach(() => vi.clearAllMocks());

// The dialog never promises more than the job will do: declining the lossy
// resync drops those files from the count; "keep backups" off switches the
// job to a rolling window plus deep-verify; each volume line says when the
// backups or the temp copy would not fit; rollback and purge send only the
// library scope.
describe("RepairConfirmModal", () => {
  it("counts the resync files only when consented, and sends the matching job", () => {
    const p = mount();
    expect(screen.getByTestId("seek-index-confirm-text")).toHaveTextContent("repairAllConfirm(count=10)");
    expect(screen.getByText(/resyncDamaged\(count=4,bytes=8.0 KB\)/)).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-resync-required-note")).toHaveTextContent("count=1");
    fireEvent.click(screen.getByText("common.confirm"));
    expect(p.repairLib.mutate).toHaveBeenCalledWith({ data: { libraryId: "lib-1", mode: "auto", deepVerify: false, resyncDamaged: true } });
    expect(p.setConfirmBatch).toHaveBeenCalledWith(null);

    vi.clearAllMocks();
    const q = mount({ resyncDamaged: false, keepBackups: false, libraryId: "" });
    expect(screen.getAllByTestId("seek-index-confirm-text").at(-1)).toHaveTextContent("repairAllConfirm(count=6)");
    expect(screen.getByText("admin.seekIndex.keepBackupsOffHint(n=5)")).toBeInTheDocument();
    fireEvent.click(screen.getAllByText("common.confirm").at(-1)!);
    expect(q.repairLib.mutate).toHaveBeenCalledWith({ data: { mode: "auto", deepVerify: true, backupWindow: 5 } });
  });

  it("flags a volume where the backups or the temp copy would not fit", () => {
    const gib = 1024 ** 3;
    mount({
      repairVolumes: [
        { root: "/media/a", freeBytes: String(100 * gib), missingBytes: String(10 * gib), largestMissingBytes: String(4 * gib), missingFiles: 3 },
        { root: "/media/b", freeBytes: String(5 * gib), missingBytes: String(20 * gib), largestMissingBytes: String(1 * gib), missingFiles: 9 },
        { root: "/media/c", freeBytes: String(2 * gib), missingBytes: String(30 * gib), largestMissingBytes: String(8 * gib), missingFiles: 2 },
      ],
    });
    const lines = screen.getAllByRole("listitem");
    expect(lines[0]).not.toHaveTextContent("volumeBackupsRisky");
    expect(lines[1]).toHaveTextContent("volumeBackupsRisky");
    expect(lines[1]).not.toHaveTextContent("volumeTempRisky");
    expect(lines[2]).toHaveTextContent("volumeTempRisky(largest=8.0 GB)");
  });

  it("rollback and purge confirm on the backup count and send only the scope", () => {
    const r = mount({ confirmBatch: "rollback" });
    expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe("admin.seekIndex.rollbackAllConfirmTitle");
    expect(screen.getByTestId("seek-index-confirm-text")).toHaveTextContent("rollbackAllConfirm(count=3)");
    expect(screen.queryByTestId("seek-index-keep-backups")).toBeNull();
    fireEvent.click(screen.getByText("common.confirm"));
    expect(r.rollbackLib.mutate).toHaveBeenCalledWith({ data: { libraryId: "lib-1" } });

    vi.clearAllMocks();
    const p = mount({ confirmBatch: "purge", libraryId: "" });
    fireEvent.click(screen.getAllByText("common.confirm").at(-1)!);
    expect(p.purgeLib.mutate).toHaveBeenCalledWith({ data: {} });
    fireEvent.click(screen.getAllByText("common.cancel").at(-1)!);
    expect(p.setConfirmBatch).toHaveBeenCalledWith(null);
  });

  it("renders nothing when no batch is pending", () => {
    mount({ confirmBatch: null });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
