import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { SeekIndexControls } from "./SeekIndexControls";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-testid": p["data-testid"] }, children),
}));
vi.mock("@/components/ui/Select", () => ({
  Select: ({ label, value, onChange, options, disabled, ...p }: any) =>
    React.createElement(
      "select",
      { "aria-label": label, value, disabled, onChange: (e: React.ChangeEvent<HTMLSelectElement>) => onChange(e.target.value), "data-testid": p["data-testid"] },
      options.map((o: any) => React.createElement("option", { key: o.value, value: o.value }, o.label)),
    ),
}));

function mount(over: Record<string, unknown> = {}) {
  const props = {
    libraryId: "", setLibraryId: vi.fn(), libraries: [{ id: "l1", name: "Films" }, { id: "l2" }],
    startScan: { mutate: vi.fn() }, busy: false, result: undefined, repairable: 0, backupsRisky: false,
    setKeepBackups: vi.fn(), setConfirmBatch: vi.fn(), withBackup: 0, ...over,
  };
  render(<SeekIndexControls {...(props as any)} />);
  return props;
}

beforeEach(() => vi.clearAllMocks());

// A scan starts with the library scope and the pace the operator picked; the
// gentle default travels as ABSENCE (what the server does with nothing) and
// the fast pace explicitly; the batch buttons only exist after a scan, die at
// zero, and "repair all" pre-sets the safe backup choice from the volumes.
describe("SeekIndexControls", () => {
  it("scans with the scope and pace, gentle as absence", () => {
    const p = mount();
    expect(screen.getByTestId("seek-index-pace-hint")).toHaveTextContent("paceGentleHint");
    fireEvent.click(screen.getByTestId("seek-index-scan-btn"));
    expect(p.startScan.mutate).toHaveBeenCalledWith({ data: {} });
    fireEvent.change(screen.getByTestId("seek-index-pace-select"), { target: { value: "fast" } });
    expect(screen.getByTestId("seek-index-pace-hint")).toHaveTextContent("paceFastHint");
    fireEvent.change(screen.getByLabelText("admin.seekIndex.library"), { target: { value: "l2" } });
    expect(p.setLibraryId).toHaveBeenCalledWith("l2");
    expect(screen.queryByTestId("seek-index-repair-all-btn")).toBeNull();
  });

  it("with a scope and the fast pace both travel", () => {
    const p = mount({ libraryId: "l1" });
    fireEvent.change(screen.getByTestId("seek-index-pace-select"), { target: { value: "fast" } });
    fireEvent.click(screen.getByTestId("seek-index-scan-btn"));
    expect(p.startScan.mutate).toHaveBeenCalledWith({ data: { libraryId: "l1", pace: "fast" } });
  });

  it("after a scan the batch buttons carry their counts, die at zero, and repair-all pre-sets the backup choice", () => {
    const p = mount({ result: {}, repairable: 4, withBackup: 0, backupsRisky: true });
    expect(screen.getByTestId("seek-index-scan-btn")).toHaveTextContent("scanAgain");
    expect((screen.getByTestId("seek-index-rollback-all-btn") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("seek-index-purge-all-btn") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("seek-index-repair-all-btn"));
    expect(p.setKeepBackups).toHaveBeenCalledWith(false);
    expect(p.setConfirmBatch).toHaveBeenCalledWith("repair");

    const q = mount({ result: {}, repairable: 0, withBackup: 2, busy: false });
    expect((screen.getAllByTestId("seek-index-repair-all-btn").at(-1) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getAllByTestId("seek-index-rollback-all-btn").at(-1)!);
    expect(q.setConfirmBatch).toHaveBeenCalledWith("rollback");
    fireEvent.click(screen.getAllByTestId("seek-index-purge-all-btn").at(-1)!);
    expect(q.setConfirmBatch).toHaveBeenCalledWith("purge");
  });
});
