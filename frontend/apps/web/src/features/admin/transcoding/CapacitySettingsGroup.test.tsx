import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, within } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({ entries: [] as unknown[], setEntry: vi.fn(), fetch: vi.fn() }));
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => api.fetch(...a),
  apiErrorMessage: (e: Error) => e.message,
  useAdminSystemServiceGetSystemInfo: () => ({ data: undefined }),
  useServerConfigServiceListServerConfigEntries: (opts: any) => ({ data: opts.query.select({ entries: api.entries }) }),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["config"],
  serverConfigServiceSetServerConfigEntry: (...a: unknown[]) => api.setEntry(...a),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: unknown) => (o && typeof o === "object" && "value" in (o as object) ? `${k}(${String((o as { value: unknown }).value)})` : k) }),
}));

import { CapacitySettingsGroup } from "./CapacitySettingsGroup";

let settings: Record<string, unknown>;
function mount() {
  api.fetch.mockImplementation((o: { method: string }) => Promise.resolve(o.method === "GET" ? settings : {}));
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(CapacitySettingsGroup)));
}
const puts = () => api.fetch.mock.calls.map((c) => c[0] as { method: string; data: unknown }).filter((c) => c.method === "PUT").map((c) => c.data);

beforeEach(() => {
  vi.clearAllMocks();
  api.entries = [];
  api.setEntry.mockResolvedValue({});
  settings = { maxConcurrentTranscodes: { value: 2, default: 2, source: "default" } };
});

// Two limits, two APIs, one Save: the live-stream cap (a config entry) and
// the background conversions (a server setting, a fixed default of 2 when
// blank). Each sends only when changed; an invalid number is refused inline.
describe("CapacitySettingsGroup", () => {
  it("has a single Save for both limits, and sends only what changed", async () => {
    api.entries = [{ key: "transcoding.max_video_sessions", value: "4" }];
    mount();
    const sessions = (await screen.findByLabelText("admin.transcoding.maxVideoSessionsLabel")) as HTMLInputElement;
    expect(sessions.value).toBe("4");
    expect(sessions.placeholder).toBe("admin.transcoding.maxVideoSessionsAuto");
    const background = (await screen.findByTestId("max-concurrent-input")) as HTMLInputElement;
    expect(background.value).toBe("");
    expect(background.placeholder).toBe("admin.settingsLayout.autoDefault(2)");
    expect(within(screen.getByTestId("group-capacity")).getAllByRole("button", { name: "common.save" })).toHaveLength(1);

    const save = screen.getByTestId("capacity-save-btn");
    expect(save).toBeDisabled();
    fireEvent.change(sessions, { target: { value: " 2 " } });
    fireEvent.click(save);
    await waitFor(() => expect(api.setEntry).toHaveBeenCalledWith("transcoding.max_video_sessions", { value: "2" }));
    expect(puts()).toEqual([]);

    fireEvent.change(background, { target: { value: "1" } });
    fireEvent.click(save);
    await waitFor(() => expect(puts()).toEqual([{ maxConcurrentTranscodes: 1 }]));
    expect(api.setEntry).toHaveBeenCalledTimes(1);
  });

  it("clears a background cap with an explicit null when emptied", async () => {
    settings = { maxConcurrentTranscodes: { value: 3, default: 2, source: "override" } };
    mount();
    const background = (await screen.findByTestId("max-concurrent-input")) as HTMLInputElement;
    await waitFor(() => expect(background.value).toBe("3"));
    fireEvent.change(background, { target: { value: "" } });
    fireEvent.click(screen.getByTestId("capacity-save-btn"));
    await waitFor(() => expect(puts()).toEqual([{ maxConcurrentTranscodes: null }]));
  });

  it("refuses a zero or a fraction inline, without a request", async () => {
    mount();
    fireEvent.change(await screen.findByLabelText("admin.transcoding.maxVideoSessionsLabel"), { target: { value: "0" } });
    fireEvent.change(screen.getByTestId("max-concurrent-input"), { target: { value: "1.5" } });
    fireEvent.click(screen.getByTestId("capacity-save-btn"));
    await waitFor(() => expect(screen.getAllByTestId("setting-error")).toHaveLength(2));
    expect(api.setEntry).not.toHaveBeenCalled();
    expect(puts()).toEqual([]);
  });
});
