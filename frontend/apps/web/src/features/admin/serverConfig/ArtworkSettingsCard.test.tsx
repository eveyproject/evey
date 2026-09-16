import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ArtworkSettingsCard } from "./ArtworkSettingsCard";

const setEntry = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false }));
vi.mock("./hooks", () => ({ useSetConfigEntry: () => setEntry }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), apiErrorMessage: (e: Error) => e.message }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => <span data-testid="spinner" /> }));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading }: any) => React.createElement("button", { onClick, disabled: loading }, children),
}));

function mount(entries: { key: string; value: string }[] | undefined) {
  const qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><ArtworkSettingsCard entries={entries} /></QueryClientProvider>);
}

beforeEach(() => {
  vi.clearAllMocks();
  setEntry.isPending = false;
  eveyFetchMock.mockResolvedValue({ jobId: "j1", total: 4 });
});

const box = () => screen.getByLabelText("admin.serverConfig.artworkAutoDownload") as HTMLInputElement;

// Auto-download is ON unless the stored value is exactly "false" (the server's
// own default rule); the toggle writes "true"/"false"; "retry now" posts the
// backfill job once and only confirms it started (progress lives elsewhere).
describe("ArtworkSettingsCard", () => {
  it("defaults to on and only an explicit false disables", () => {
    mount(undefined);
    expect(box().checked).toBe(true);
  });

  it("reads false, writes the flipped value", () => {
    mount([{ key: "artwork.auto_download", value: "false" }]);
    expect(box().checked).toBe(false);
    fireEvent.click(box());
    expect(setEntry.mutate).toHaveBeenCalledWith({ key: "artwork.auto_download", value: "true" });
  });

  it("any other stored value still means on", () => {
    mount([{ key: "artwork.auto_download", value: "yes" }]);
    expect(box().checked).toBe(true);
    fireEvent.click(box());
    expect(setEntry.mutate).toHaveBeenCalledWith({ key: "artwork.auto_download", value: "false" });
  });

  it("retry now posts the backfill and confirms it started, or reports the refusal", async () => {
    mount([]);
    fireEvent.click(screen.getByText("admin.serverConfig.artworkRetryNow"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.artworkRetryStarted"));
    expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/artworks/retry-missing", method: "POST" });

    eveyFetchMock.mockRejectedValueOnce(new Error("busy"));
    fireEvent.click(screen.getByText("admin.serverConfig.artworkRetryNow"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("busy"));
  });

  it("freezes the toggle while a write is in flight", () => {
    setEntry.isPending = true;
    mount([]);
    expect(box().disabled).toBe(true);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });
});
