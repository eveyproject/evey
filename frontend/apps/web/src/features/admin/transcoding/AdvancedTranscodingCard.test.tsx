import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, d?: unknown) => (typeof d === "string" ? d : k) }) }));
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

// The AV1 software ceiling was accepted by the server but offered nowhere.
// It is a segmented choice of the heights the server accepts; Auto is the
// default, so picking it clears the override (null). It is read live: no
// restart badge, and saving only it does not ask for a restart.
describe("AdvancedTranscodingCard - AV1 software ceiling", () => {
  it("offers the accepted heights, sends the pick, and Auto as a reset", async () => {
    settings.data = { ...(settings.data as object), softwareAV1MaxHeight: sv(0, 0, "default", false) };
    mount();
    const group = screen.getByRole("radiogroup", { name: "admin.serverSettings.softwareAV1MaxHeight" });
    const radios = Array.from(group.querySelectorAll("[role=radio]"));
    expect(radios.map((r) => r.getAttribute("data-value"))).toEqual(["0", "480", "720", "1080", "1440"]);
    expect(group.querySelector("[aria-checked=true]")?.getAttribute("data-value")).toBe("0");
    expect(screen.getByTestId("software-av1-row").querySelector("[data-testid=setting-restart-badge]")).toBeNull();

    fireEvent.click(screen.getByRole("radio", { name: "720p" }));
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
    const body = (eveyFetchMock.mock.calls[0][0] as { data: Record<string, unknown> }).data;
    expect(body.softwareAV1MaxHeight).toBe(720);
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Saved"));
  });

  it("seeds a stored ceiling and clears it with Auto", async () => {
    settings.data = { ...(settings.data as object), softwareAV1MaxHeight: sv(1080, 0, "override", false) };
    mount();
    expect(screen.getByRole("radio", { name: "1080p" })).toHaveAttribute("aria-checked", "true");
    fireEvent.click(screen.getByRole("radio", { name: "admin.serverSettings.softwareAV1Auto" }));
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
    expect((eveyFetchMock.mock.calls[0][0] as { data: Record<string, unknown> }).data.softwareAV1MaxHeight).toBeNull();
  });

  it("leaves the AV1 ceiling out of a save that did not touch it, and badges the boot-time rows", async () => {
    settings.data = { ...(settings.data as object), softwareAV1MaxHeight: sv(720, 0, "override", false) };
    mount();
    expect(screen.getAllByTestId("setting-restart-badge")).toHaveLength(5);
    fireEvent.click(screen.getByTestId("single-variant-toggle"));
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalled());
    expect((eveyFetchMock.mock.calls[0][0] as { data: Record<string, unknown> }).data).not.toHaveProperty("softwareAV1MaxHeight");
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Saved - restart the server to apply"));
  });

  it("explains every limit behind an 'i'", () => {
    mount();
    expect(screen.getAllByRole("button", { name: "admin.settingsLayout.moreInfo" })).toHaveLength(6);
  });
});
