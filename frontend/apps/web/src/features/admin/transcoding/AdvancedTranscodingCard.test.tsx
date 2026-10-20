import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a), apiErrorMessage: (e: Error) => e.message }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { AdvancedTranscodingCard } from "./AdvancedTranscodingCard";

const sv = (value: unknown, def: unknown, source: "override" | "default" = "default", requiresRestart = true) => ({ value, default: def, source, requiresRestart });

let current: Record<string, unknown>;
function settings(overrides: Record<string, unknown> = {}) {
  return {
    transcodeMaxSessions: sv(0, 0),
    softwareMaxHeight: sv(1080, 0, "override"),
    softwareHDRMaxHeight: sv(0, 0),
    softwareAV1MaxHeight: sv(0, 0, "default", false),
    transcodeSingleVariant: sv(true, true),
    transcodeHWPerVariant: sv(false, false),
    ...overrides,
  };
}

function mount() {
  eveyFetchMock.mockImplementation((o: { method: string }) => Promise.resolve(o.method === "GET" ? current : {}));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(AdvancedTranscodingCard)));
}

const puts = () =>
  eveyFetchMock.mock.calls.map((c) => c[0] as { method: string; data: unknown }).filter((c) => c.method === "PUT").map((c) => c.data);
const checked = (testId: string) => within(screen.getByTestId(testId)).getByRole("radio", { checked: true }).getAttribute("data-value");

beforeEach(() => {
  vi.clearAllMocks();
  current = settings();
});

// The expert limits send only what the admin changed (an untouched switch
// never becomes a stored override), a cleared choice as an explicit null,
// and a height only from the ladder the server accepts. A refetch caused by
// another card on the page never blanks a row being edited.
describe("AdvancedTranscodingCard", () => {
  it("seeds only overrides and sends only the touched rows", async () => {
    mount();
    expect(await screen.findByTestId("advanced-restart-hint")).toBeInTheDocument();
    expect((screen.getByTestId("max-sessions-input") as HTMLInputElement).value).toBe("");
    expect(checked("software-max-height")).toBe("1080");
    expect(checked("software-hdr-max-height")).toBe("");
    expect(screen.getByTestId("single-variant-toggle")).toHaveAttribute("aria-checked", "true");
    const save = screen.getByTestId("advanced-transcoding-save-btn");
    expect(save).toBeDisabled();

    fireEvent.change(screen.getByTestId("max-sessions-input"), { target: { value: "2" } });
    fireEvent.click(within(screen.getByTestId("software-max-height")).getByRole("radio", { name: "admin.settingsLayout.autoMeasured" }));
    fireEvent.click(save);
    await waitFor(() => expect(puts()).toEqual([{ transcodeMaxSessions: 2, softwareMaxHeight: null }]));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.transcoding.savedRestart"));
    await waitFor(() => expect(save).toBeDisabled());
  });

  it("offers the software heights the server accepts, and shows an off-ladder stored value", async () => {
    current = settings({ softwareHDRMaxHeight: sv(900, 0, "override") });
    mount();
    const sdr = await screen.findByTestId("software-max-height");
    expect(within(sdr).getAllByRole("radio").map((r) => r.getAttribute("data-value"))).toEqual(["", "480", "720", "1080", "1440", "2160"]);
    expect(checked("software-hdr-max-height")).toBe("900");
    expect(within(screen.getByTestId("software-hdr-max-height")).getByRole("radio", { checked: true })).toHaveTextContent("900p");
  });

  it("the AV1 software ceiling applies live: no restart badge, plain 'Saved', Auto clears it", async () => {
    current = settings({ softwareAV1MaxHeight: sv(1080, 0, "override", false) });
    mount();
    const row = await screen.findByTestId("setting-row-softwareAV1MaxHeight");
    expect(within(row).queryByTestId("setting-restart-badge")).toBeNull();
    expect(checked("software-av1-max-height")).toBe("1080");
    fireEvent.click(within(row).getByRole("radio", { name: "admin.settingsLayout.autoPlain" }));
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    await waitFor(() => expect(puts()).toEqual([{ softwareAV1MaxHeight: null }]));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverSettings.saved"));
  });

  it("refuses a session count that is not a whole number, without a request", async () => {
    mount();
    fireEvent.change(await screen.findByTestId("max-sessions-input"), { target: { value: "-1" } });
    fireEvent.click(screen.getByTestId("advanced-transcoding-save-btn"));
    expect(await screen.findByTestId("setting-error")).toHaveTextContent("admin.settingsLayout.errorNumber");
    expect(puts()).toEqual([]);
  });

  it("explains every limit behind an 'i' and badges the rows read at boot", async () => {
    mount();
    await screen.findByTestId("advanced-restart-hint");
    expect(screen.getAllByRole("button", { name: "admin.settingsLayout.moreInfo" })).toHaveLength(6);
    expect(screen.getAllByTestId("setting-restart-badge")).toHaveLength(5);
  });

  it("drops the restart hint once no knob needs a restart", async () => {
    current = { transcodeMaxSessions: sv(0, 0, "default", false), softwareMaxHeight: sv(0, 0, "default", false) };
    mount();
    await screen.findByTestId("max-sessions-input");
    expect(screen.queryByTestId("advanced-restart-hint")).toBeNull();
  });
});
