import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { LastJobCard } from "./LastJobCard";
import { JobProgressCard } from "./JobProgressCard";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}:${Object.values(o).join(",")}` : k) }),
}));
vi.mock("@/components/ui/Card", () => ({ Card: ({ children, ...p }: any) => React.createElement("div", { "data-testid": p["data-testid"] }, children) }));
vi.mock("@/components/ui/Badge", () => ({ Badge: ({ children, variant }: any) => React.createElement("span", { "data-testid": `badge-${variant}` }, children) }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => null }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, ...p }: any) =>
    React.createElement("button", { onClick: p.onClick, disabled: p.disabled, title: p.title, "data-testid": p["data-testid"] }, children),
}));

// The last-job line names the kind and counts, and "processed" includes
// skips, so the skipped count is shown on its own (a repair batch skipping
// 133 files once read as a full success for days). Failure beats OK, an
// error replaces OK, read-only gets its hint, and each failure names its
// file by basename with the full path on hover.
describe("LastJobCard", () => {
  it("a clean run: kind, counts, OK badge, no skipped line", () => {
    render(<LastJobCard last={{ kind: "reindex", done: 10, total: 10, failed: 0 }} />);
    expect(screen.getByText("admin.seekIndex.lastJobLine:admin.seekIndex.jobReindex,10,10,0")).toBeInTheDocument();
    expect(screen.getByTestId("badge-success")).toBeInTheDocument();
    expect(screen.queryByTestId("seek-index-skipped-count")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("shows skips, failures with their files, and the read-only hint", () => {
    render(
      <LastJobCard
        last={{
          kind: "unknown-kind", done: 138, total: 138, failed: 2, skipped: 133, readOnly: true,
          failures: [{ path: "/media/a/one.mkv", error: "EACCES" }, { path: "two.mkv", error: "short" }],
        }}
      />,
    );
    expect(screen.getByText(/admin\.seekIndex\.jobScan,138,138,2/)).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-skipped-count")).toHaveTextContent("admin.seekIndex.jobSkippedCount:133");
    expect(screen.getByTestId("badge-warning")).toHaveTextContent("admin.seekIndex.jobFailedCount:2");
    expect(screen.queryByTestId("badge-success")).toBeNull();
    expect(screen.getByTestId("seek-index-ro-hint")).toBeInTheDocument();
    expect(screen.getByTitle("/media/a/one.mkv")).toHaveTextContent("one.mkv");
    expect(screen.getByText(/two\.mkv/).closest("li")).toHaveTextContent("two.mkv - short");
  });

  it("an aborted run reports its error instead of OK", () => {
    render(<LastJobCard last={{ kind: "scan", done: 3, total: 9, failed: 0, error: "disk gone" }} />);
    expect(screen.getByRole("alert")).toHaveTextContent("admin.seekIndex.lastJobError:disk gone");
    expect(screen.queryByTestId("badge-success")).toBeNull();
  });

  // A stopped job is neither a success nor a failure: 3/9 with a "Completed"
  // badge read like a batch that ran out of work.
  it("a stopped run says so instead of OK", () => {
    render(<LastJobCard last={{ kind: "reindex", done: 3, total: 9, failed: 0, cancelled: true }} />);
    expect(screen.getByTestId("badge-default")).toHaveTextContent("admin.seekIndex.lastJobCancelled");
    expect(screen.queryByTestId("badge-success")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

// While scanning the total is unknown: an indeterminate bar with no value.
// Once processing, the bar is done/total (clamped to 100 %), failures are
// counted in red, and the current file is named by basename.
describe("JobProgressCard", () => {
  it("is indeterminate while the total is unknown", () => {
    render(<JobProgressCard job={{ kind: "scan", phase: "scanning", done: 0, total: 0 }} />);
    expect(screen.getByText("admin.seekIndex.jobRunning:admin.seekIndex.jobScan,admin.seekIndex.phaseScanning")).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-job-progress")).toHaveTextContent("…");
    const bar = screen.getByRole("progressbar");
    expect(bar.getAttribute("aria-valuenow")).toBeNull();
    expect(bar.getAttribute("aria-valuetext")).toBe("admin.seekIndex.phaseScanning");
    expect((bar.firstElementChild as HTMLElement).style.width).toBe("");
  });

  it("reports done/total, failures and the current file while processing", () => {
    render(<JobProgressCard job={{ kind: "fix_container", phase: "processing", done: 3, total: 4, failed: 1, currentPath: "/x/y/file.mkv" }} />);
    expect(screen.getByText(/admin\.seekIndex\.jobFixContainer,admin\.seekIndex\.phaseProcessing/)).toBeInTheDocument();
    expect(screen.getByTestId("seek-index-job-progress")).toHaveTextContent("3/4");
    expect(screen.getByTestId("seek-index-job-progress")).toHaveTextContent("admin.seekIndex.jobFailedCount:1");
    const bar = screen.getByRole("progressbar");
    expect(bar.getAttribute("aria-valuenow")).toBe("3");
    expect(bar.getAttribute("aria-valuemax")).toBe("4");
    expect((bar.firstElementChild as HTMLElement).style.width).toBe("75%");
    expect(screen.getByTitle("/x/y/file.mkv")).toHaveTextContent("file.mkv");
  });

  it("never overshoots 100 % when done outruns total", () => {
    render(<JobProgressCard job={{ kind: "scan", phase: "processing", done: 12, total: 10 }} />);
    expect((screen.getByRole("progressbar").firstElementChild as HTMLElement).style.width).toBe("100%");
  });

  // The stop button asks the server once and then stays disabled: the job
  // endpoint reports the end, a second click would only repeat the request.
  it("offers to stop the job and disables the button once asked", () => {
    const onCancel = vi.fn();
    const { rerender } = render(<JobProgressCard job={{ kind: "reindex", phase: "processing", done: 1, total: 9 }} onCancel={onCancel} />);
    const btn = screen.getByTestId("seek-index-job-cancel");
    expect(btn).toHaveTextContent("admin.seekIndex.jobCancel");
    expect(btn).toHaveAttribute("title", "admin.seekIndex.jobCancelHint");
    btn.click();
    expect(onCancel).toHaveBeenCalledTimes(1);
    rerender(<JobProgressCard job={{ kind: "reindex", phase: "processing", done: 1, total: 9 }} onCancel={onCancel} cancelling />);
    expect(screen.getByTestId("seek-index-job-cancel")).toBeDisabled();
  });

  it("has no stop button when nobody handles it", () => {
    render(<JobProgressCard job={{ kind: "scan", phase: "scanning", done: 0, total: 0 }} />);
    expect(screen.queryByTestId("seek-index-job-cancel")).toBeNull();
  });
});
