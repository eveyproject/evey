import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, d?: unknown, o?: Record<string, unknown>) => (typeof d === "string" ? d : k).replace("{{n}}", String(o?.n)) }),
}));
const settings = vi.hoisted(() => ({ data: undefined as unknown, isLoading: false }));
vi.mock("./hooks", () => ({ SETTINGS_KEY: ["settings"], useTranscodingServerSettings: () => settings }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => React.createElement("div", { "data-testid": "spinner" }) }));
vi.mock("@/components/ui/Input", () => ({
  Input: ({ label, value, onChange, placeholder, ...p }: any) => React.createElement("input", { "aria-label": label, value, onChange, placeholder, "data-testid": p["data-testid"] }),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, ...p }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-testid": p["data-testid"] }, children),
}));
vi.mock("@/components/ui/Switch", () => ({
  Switch: ({ checked, onChange, label, ...p }: any) => React.createElement("button", { role: "switch", "aria-checked": checked, "aria-label": label, onClick: onChange, "data-testid": p["data-testid"] }),
}));

import { ConcurrencyCard } from "./ConcurrencyCard";

const setting = (value: number, source: "override" | "default") => ({ value, default: 3, source, requiresRestart: false });

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(ConcurrencyCard)));
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetchMock.mockResolvedValue({});
  settings.isLoading = false;
  settings.data = { maxConcurrentTranscodes: setting(3, "default") };
});

// Auto means "no override": the field is hidden, the hint quotes the server
// default and a save clears the cap with an explicit null. A manual cap is
// seeded from the override and sent as a number; a blank manual field is the
// same as Auto rather than a 0 or NaN cap.
describe("ConcurrencyCard", () => {
  it("shows a spinner until the settings arrive", () => {
    settings.isLoading = true;
    settings.data = undefined;
    mount();
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
    expect(screen.queryByTestId("concurrency-save-btn")).toBeNull();
  });

  it("starts on Auto with the server default in the hint and saves null", async () => {
    mount();
    expect(screen.getByTestId("max-concurrent-auto-toggle").getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText("Server default (3)")).toBeInTheDocument();
    expect(screen.queryByTestId("max-concurrent-input")).toBeNull();

    fireEvent.click(screen.getByTestId("concurrency-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledTimes(1));
    expect(eveyFetchMock.mock.calls[0][0]).toMatchObject({
      url: "/api/v1/admin/server-settings",
      method: "PUT",
      data: { maxConcurrentTranscodes: null },
    });
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Saved"));
  });

  it("seeds a manual cap from the override and sends the picked number", async () => {
    settings.data = { maxConcurrentTranscodes: setting(2, "override") };
    mount();
    expect(screen.getByTestId("max-concurrent-auto-toggle").getAttribute("aria-checked")).toBe("false");
    const input = screen.getByTestId("max-concurrent-input") as HTMLInputElement;
    expect(input.value).toBe("2");
    expect(input.placeholder).toBe("3");
    expect(screen.getByText("Cap concurrent transcodes")).toBeInTheDocument();

    fireEvent.change(input, { target: { value: "5" } });
    fireEvent.click(screen.getByTestId("concurrency-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledTimes(1));
    expect(eveyFetchMock.mock.calls[0][0].data).toEqual({ maxConcurrentTranscodes: 5 });
  });

  it("treats a blank manual field as Auto and flipping back to Auto clears the cap", async () => {
    mount();
    fireEvent.click(screen.getByTestId("max-concurrent-auto-toggle"));
    const input = screen.getByTestId("max-concurrent-input") as HTMLInputElement;
    expect(input.value).toBe("");
    fireEvent.click(screen.getByTestId("concurrency-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledTimes(1));
    expect(eveyFetchMock.mock.calls[0][0].data).toEqual({ maxConcurrentTranscodes: null });

    fireEvent.change(input, { target: { value: "4" } });
    fireEvent.click(screen.getByTestId("max-concurrent-auto-toggle"));
    expect(screen.queryByTestId("max-concurrent-input")).toBeNull();
    fireEvent.click(screen.getByTestId("concurrency-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledTimes(2));
    expect(eveyFetchMock.mock.calls[1][0].data).toEqual({ maxConcurrentTranscodes: null });
  });

  it("reports a refused save through the error toast", async () => {
    eveyFetchMock.mockRejectedValueOnce(new Error("nope"));
    mount();
    fireEvent.click(screen.getByTestId("concurrency-save-btn"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("nope"));
    expect(toast.success).not.toHaveBeenCalled();
  });
});
