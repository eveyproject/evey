import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k) }),
}));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => React.createElement("div", { "data-testid": "spinner" }) }));
vi.mock("@/components/ui/Card", () => ({ Card: ({ children }: any) => React.createElement("div", null, children) }));
vi.mock("@/components/ui/PageHeader", () => ({ PageHeader: ({ title }: any) => React.createElement("h1", null, title) }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading }: any) => React.createElement("button", { onClick, disabled: disabled || loading }, children),
}));

import { ArtworksAdmin } from "./ArtworksAdmin";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(ArtworksAdmin)));
}

beforeEach(() => {
  vi.clearAllMocks();
});

// Bulk download and blurhash backfill both hand back a job id the page polls
// until it is done OR failed (a failed job used to be polled forever with no
// word to the admin); the button stays disabled while a job runs, the stats
// refresh once at the end, and each terminal state is announced once.
describe("ArtworksAdmin", () => {
  it("starts a download, follows the job to done, refreshes the stats and says so once", async () => {
    let polls = 0;
    let statsFetches = 0;
    eveyFetchMock.mockImplementation(({ url, method }: { url: string; method: string }) => {
      if (url === "/api/v1/admin/artworks/stats") {
        statsFetches++;
        return Promise.resolve({ totalRecords: 10, withLocalPath: 4, missingLocalPath: 6, totalSizeBytes: 1024 });
      }
      if (url === "/api/v1/admin/artworks/download-all" && method === "POST") return Promise.resolve({ jobId: "job-1", totalToDownload: 6 });
      if (url === "/api/v1/admin/artworks/download-all/job-1") {
        polls++;
        return Promise.resolve(polls < 2 ? { status: "running", processed: 3, total: 6, errors: [] } : { status: "done", processed: 6, total: 6, errors: [] });
      }
      return Promise.reject(new Error("unexpected " + url));
    });
    mount();
    const btn = await screen.findByText("admin.artworks.downloadMissing");
    fireEvent.click(btn);
    await waitFor(() => expect(screen.getByText("admin.artworks.downloading")).toBeInTheDocument());
    expect((screen.getByText("admin.artworks.downloading") as HTMLButtonElement).disabled).toBe(true);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.artworks.downloadComplete"), { timeout: 6000 });
    expect(toast.success).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(statsFetches).toBeGreaterThanOrEqual(2));
    expect(screen.getByText("admin.artworks.downloadAgain")).toBeInTheDocument();
  });

  it("a failed job is announced once and polling stops; a refused start is reported", async () => {
    let polls = 0;
    eveyFetchMock.mockImplementation(({ url, method }: { url: string; method: string }) => {
      if (url === "/api/v1/admin/artworks/stats") return Promise.resolve({ totalRecords: 0, withLocalPath: 0, missingLocalPath: 0, totalSizeBytes: 0 });
      if (url === "/api/v1/admin/artworks/backfill-blurhashes" && method === "POST") return Promise.resolve({ jobId: "job-2", total: 3 });
      if (url === "/api/v1/admin/artworks/download-all/job-2") {
        polls++;
        return Promise.resolve({ status: "failed", processed: 1, total: 3, errors: ["boom"] });
      }
      if (url === "/api/v1/admin/artworks/download-all" && method === "POST") return Promise.reject(new Error("503"));
      return Promise.reject(new Error("unexpected " + url));
    });
    mount();
    fireEvent.click(await screen.findByText("admin.artworks.backfillBlurhashesRun"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.artworks.downloadJobFailed"));
    await new Promise((r) => setTimeout(r, 2500));
    expect(polls).toBe(1);
    expect(toast.error).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText("admin.artworks.downloadMissing"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.artworks.downloadFailed"));
  });
});
