import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Wiping the cache is confirmed with the file count and size, and warns how
// many viewers are being transcoded right now (their stream rebuffers);
// deleting one file names it; the concurrency cap is seeded from and written
// to /admin/server-settings, the endpoint that applies it to the running
// transcoder, never to the server-config keys only read at startup.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", async () => ({
  ...(await import("../../../src/test/rnDom")).rnDom,
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, o?: string | Record<string, unknown>) => (typeof o === "string" ? o : typeof o?.defaultValue === "string" ? o.defaultValue : k),
  }),
}));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...a: unknown[]) => eveyFetch(...a),
  useAdminSystemServiceGetSystemInfo: () => ({ data: { hwDevices: [] } }),
  formatBytes: (n: number) => `${n}B`,
  apiErrorMessage: (e: Error) => e.message,
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({ ...(await importOriginal<typeof import("@evey/store")>()), toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", danger: "#f33", error: "#f33" }, radii: { md: 8, sm: 4, lg: 12 } }),
}));

import AdminTranscodeCacheScreen from "./transcode-cache";

const cache = { totalSize: 4096, fileCount: 2, files: [{ name: "a.ts", size: 1024, lastModified: "2026-09-16T04:00:00Z" }, { name: "b/c.ts", size: 3072, lastModified: "2026-09-16T04:00:00Z" }], activeEncodingSessions: 0 };
const settings = { maxConcurrentTranscodes: { value: 3, default: 2, source: "override" } };

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><AdminTranscodeCacheScreen /></QueryClientProvider>);
}

let live = 0;
beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  live = 0;
  eveyFetch.mockImplementation(async (req: { url: string; method: string }) => {
    if (req.url === "/api/v1/admin/transcode-cache" && req.method === "GET") return { ...cache, activeEncodingSessions: live };
    if (req.url === "/api/v1/admin/server-settings" && req.method === "GET") return settings;
    return undefined;
  });
});

describe("admin transcode cache screen (mobile)", () => {
  it("confirms a wipe with count and size, warns about live viewers, then clears and refreshes", async () => {
    live = 2;
    mount();
    await waitFor(() => expect(screen.getByText("a.ts")).toBeTruthy());
    expect(screen.getByText("4096B")).toBeTruthy();
    fireEvent.click(screen.getByText("Clear All Cache"));
    expect(alerts.last.message).toContain("Delete all 2 cached files (4096B)?");
    expect(alerts.last.message).toContain("2 viewer(s) are being transcoded right now");
    expect(eveyFetch).not.toHaveBeenCalledWith(expect.objectContaining({ method: "DELETE" }));
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Cache cleared"));
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/transcode-cache", method: "DELETE" });
  });

  it("no live warning when nobody is transcoding, and deleting one file names it", async () => {
    mount();
    await waitFor(() => expect(screen.getByText("b/c.ts")).toBeTruthy());
    fireEvent.click(screen.getByText("Clear All Cache"));
    expect(alerts.last.message).not.toContain("viewer");
    fireEvent.click(screen.getByRole("button", { name: "Supprimer b/c.ts" }));
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/transcode-cache/b%2Fc.ts", method: "DELETE" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Deleted"));
  });

  // The cap is the shared server-settings field (the same as Transcoding >
  // Capacity): seeded with the stored override only, written to
  // /admin/server-settings, never to the server-config keys read at startup.
  it("seeds the cap from /server-settings and saves only it there, never to server-config", async () => {
    mount();
    const max = (await screen.findByTestId("max-concurrent-input")) as HTMLInputElement;
    await waitFor(() => expect(max.value).toBe("3"));
    // The throttle switch wrote a key no server code reads.
    expect(screen.queryByRole("switch")).toBeNull();
    fireEvent.change(max, { target: { value: "5" } });
    fireEvent.click(screen.getByTestId("transcode-cache-settings-save"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverSettings.saved"));
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/server-settings", method: "PUT", data: { maxConcurrentTranscodes: 5 } });
    expect(eveyFetch).not.toHaveBeenCalledWith(expect.objectContaining({ url: expect.stringContaining("/server-config") }));
  });

  it("clears the override when the field is emptied", async () => {
    mount();
    const max = (await screen.findByTestId("max-concurrent-input")) as HTMLInputElement;
    await waitFor(() => expect(max.value).toBe("3"));
    fireEvent.change(max, { target: { value: "" } });
    fireEvent.click(screen.getByTestId("transcode-cache-settings-save"));
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/admin/server-settings", method: "PUT", data: { maxConcurrentTranscodes: null } }));
  });

  // The numeric keyboard offers "-": parseInt made NaN of it, JSON sent null,
  // and the server dropped the cap under a "saved" toast.
  it("refuses a cap that is not a whole number of at least 1 and sends nothing", async () => {
    mount();
    const max = await screen.findByTestId("max-concurrent-input");
    for (const value of ["-", "0", "1.5"]) {
      vi.mocked(eveyFetch).mockClear();
      fireEvent.change(max, { target: { value } });
      fireEvent.click(screen.getByTestId("transcode-cache-settings-save"));
      await waitFor(() => expect(screen.getByTestId("setting-error").textContent).toBe("admin.settingsLayout.errorNumber"));
      expect(eveyFetch).not.toHaveBeenCalledWith(expect.objectContaining({ method: "PUT" }));
    }
  });
});
