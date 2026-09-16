import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, d?: string) => d ?? k }) }));
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

import { AdvancedTranscodingCard } from "./AdvancedTranscodingCard";

const sv = (value: unknown, def: unknown, source: "override" | "default" = "default", requiresRestart = true) => ({ value, default: def, source, requiresRestart });

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(AdvancedTranscodingCard)));
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetchMock.mockResolvedValue({});
  settings.isLoading = false;
  settings.data = {
    transcodeMaxSessions: sv(0, 0),
    softwareMaxHeight: sv(1080, 0, "override"),
    softwareHDRMaxHeight: sv(0, 0),
    transcodeSingleVariant: sv(false, false),
    transcodeHWPerVariant: sv(true, false),
  };
});

// A blank field is "no override" and is sent as an explicit null (the reset
// path), never as 0 - which is a real value here; an override seeds its
// field; the toggles always travel; the restart hint follows the server's
// own flag rather than a hardcoded assumption.
describe("AdvancedTranscodingCard", () => {
  it("seeds only overrides, sends blanks as null and numbers as numbers", async () => {
    mount();
    expect(screen.getByTestId("advanced-restart-hint")).toBeInTheDocument();
    expect((screen.getByTestId("max-sessions-input") as HTMLInputElement).value).toBe("");
    expect((screen.getByTestId("software-max-height-input") as HTMLInputElement).value).toBe("1080");
    expect(screen.getByTestId("hw-per-variant-toggle").getAttribute("aria-checked")).toBe("true");
    const save = screen.getByTestId("advanced-transcoding-save-btn") as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByTestId("max-sessions-input"), { target: { value: "0" } });
    fireEvent.change(screen.getByTestId("software-max-height-input"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("single-variant-toggle"));
    fireEvent.click(save);
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/admin/server-settings",
      method: "PUT",
      data: { transcodeMaxSessions: 0, softwareMaxHeight: null, softwareHDRMaxHeight: null, transcodeSingleVariant: true, transcodeHWPerVariant: true },
    }));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(save.disabled).toBe(true);
  });

  it("drops the restart hint once no knob needs a restart, and shows a spinner while loading", () => {
    settings.data = { transcodeMaxSessions: sv(0, 0, "default", false), softwareMaxHeight: sv(0, 0, "default", false) };
    mount();
    expect(screen.queryByTestId("advanced-restart-hint")).toBeNull();
    settings.isLoading = true;
    mount();
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });
});
