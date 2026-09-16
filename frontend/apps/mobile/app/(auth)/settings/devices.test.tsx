import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A read-only list of push devices: the platform is named, the token is
// shown by its ends only, refresh re-reads (frozen while a read is in
// flight), and loading, failure (retry) and "none" are three states.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8 }, spacing: { xs: 4, md: 16, lg: 24, xl: 32 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import DevicesScreen from "./devices";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><DevicesScreen /></QueryClientProvider>);
}

beforeEach(() => {
  eveyFetch.mockReset();
});

describe("devices screen (mobile)", () => {
  it("names the platform, shows the token by its ends, and refreshes on demand", async () => {
    eveyFetch.mockResolvedValue({
      devices: [
        { token: "abcdef0123456789xyz", platform: "android", createdAt: "2026-09-01T10:00:00Z" },
        { token: "short", platform: "fire-tv", createdAt: "2026-09-02T10:00:00Z" },
      ],
    });
    mount();
    await waitFor(() => expect(screen.getByText("Android")).toBeTruthy());
    expect(screen.getByText("abcdef…789xyz")).toBeTruthy();
    expect(screen.getByText("short")).toBeTruthy();
    expect(screen.getByText("fire-tv")).toBeTruthy();
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/push/devices", method: "GET" });
    fireEvent.click(screen.getByText("common.refresh"));
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledTimes(2));
  });

  it("spins while loading, says none when empty, and offers a retry on failure", async () => {
    let resolve: (v: unknown) => void = () => {};
    eveyFetch.mockReturnValueOnce(new Promise((r) => { resolve = r; }));
    const first = mount();
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect((screen.getByText("common.refresh").closest("button") as HTMLButtonElement).disabled).toBe(true);
    resolve({ devices: [] });
    await waitFor(() => expect(screen.getByText("auth.devices.noDevices")).toBeTruthy());
    expect(screen.queryByText("retry")).toBeNull();
    first.unmount();

    eveyFetch.mockRejectedValueOnce(new Error("500")).mockResolvedValueOnce({ devices: [] });
    mount();
    await waitFor(() => expect(screen.getByText("retry")).toBeTruthy());
    fireEvent.click(screen.getByText("retry"));
    await waitFor(() => expect(screen.getByText("auth.devices.noDevices")).toBeTruthy());
  });
});
