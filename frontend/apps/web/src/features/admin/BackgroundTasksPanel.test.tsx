import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BackgroundTasksPanel, type BackgroundTaskView } from "./BackgroundTasksPanel";

const mockFetch = vi.fn();
const mockToast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
}));

vi.mock("@evey/store", () => ({ toast: mockToast }));

function renderPanel() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <BackgroundTasksPanel />
    </QueryClientProvider>,
  );
}

const base: BackgroundTaskView = {
  name: "level_backfill", state: "idle", processed: 12, controllable: true, paused: false,
  processes: 0, rssBytes: 0, cpuPercent: 0, cpuSecondsTotal: 0, forks: 0,
};

function tasks(): BackgroundTaskView[] {
  return [
    base,
    {
      ...base, name: "intro_detection", state: "running", detail: "mi-42", processed: 3,
      startedAt: new Date(Date.now() - 5 * 60_000).toISOString(),
      processes: 2, rssBytes: 900 * 1024 * 1024, cpuPercent: 187, cpuSecondsTotal: 640, forks: 9,
    },
    { ...base, name: "compat_download", controllable: false },
  ];
}

describe("BackgroundTasksPanel", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    mockToast.success.mockReset();
    mockToast.error.mockReset();
    mockFetch.mockImplementation(({ method }: { method: string }) => {
      if (method === "GET") return Promise.resolve(tasks());
      return Promise.resolve(undefined);
    });
  });

  it("puts the running task first and shows what it costs right now", async () => {
    renderPanel();
    await screen.findByText("Intro detection");
    const rows = screen.getAllByRole("listitem");
    expect(rows[0]).toHaveTextContent("Intro detection");
    expect(rows[0]).toHaveTextContent("mi-42");
    // Two ffmpeg at ~1 core each and 900 MB: the operator can see why the
    // box is busy without reading a log.
    expect(screen.getByTestId("task-cost-intro_detection")).toHaveTextContent("2 proc");
    expect(screen.getByTestId("task-cost-intro_detection")).toHaveTextContent("900.0 MB");
    expect(screen.getByTestId("task-cost-intro_detection")).toHaveTextContent("187% CPU");
    expect(rows[0]).toHaveTextContent("for 5 min");
    expect(rows[0]).toHaveTextContent("640 s CPU");
  });

  it("stops a task only on the second click, and posts to the stop endpoint", async () => {
    renderPanel();
    await screen.findByText("Intro detection");
    const row = screen.getAllByRole("listitem")[0];
    const stop = row.querySelector("button") as HTMLButtonElement;
    expect(stop).toHaveTextContent("Stop");

    await userEvent.click(stop);
    expect(stop).toHaveTextContent("Confirm stop?");
    expect(mockFetch).not.toHaveBeenCalledWith(expect.objectContaining({ method: "POST" }));

    await userEvent.click(stop);
    await waitFor(() =>
      expect(mockFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/background-tasks/intro_detection/stop", method: "POST" }),
    );
    await waitFor(() => expect(mockToast.success).toHaveBeenCalled());
  });

  it("offers Resume on a stopped task and no button on one that cannot be controlled", async () => {
    mockFetch.mockImplementation(({ method }: { method: string }) =>
      Promise.resolve(method === "GET" ? [{ ...base, paused: true }, { ...base, name: "compat_download", controllable: false }] : undefined),
    );
    renderPanel();
    await screen.findByText("stopped by admin");
    const [stoppedRow, plainRow] = screen.getAllByRole("listitem");
    expect(stoppedRow).toHaveTextContent("Level backfill");
    await userEvent.click(screen.getByRole("button", { name: /Resume/ }));
    await waitFor(() =>
      expect(mockFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/background-tasks/level_backfill/resume", method: "POST" }),
    );
    expect(plainRow.querySelector("button")).toBeNull();
  });

  it("draws how far the announced batch is, and no bar without one", async () => {
    mockFetch.mockImplementation(({ method }: { method: string }) =>
      Promise.resolve(method === "GET"
        ? [{ ...base, name: "library_scan", state: "running", batchTotal: 12, batchDone: 3 }, base]
        : undefined),
    );
    renderPanel();
    const bar = await screen.findByRole("progressbar");
    expect(bar).toHaveAttribute("aria-valuenow", "3");
    expect(bar).toHaveAttribute("aria-valuemax", "12");
    expect(screen.getByTestId("task-progress-library_scan")).toHaveTextContent("3 of 12");
    expect(screen.queryByTestId("task-progress-level_backfill")).toBeNull();
  });

  it("says so when the tasks cannot be read, rather than showing an empty server", async () => {
    mockFetch.mockRejectedValue(new Error("down"));
    renderPanel();
    expect(await screen.findByText("Background tasks could not be read.")).toBeInTheDocument();
  });
});
