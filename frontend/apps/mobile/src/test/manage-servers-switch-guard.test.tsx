// handleSwitch had no busy state: tapping a saved server row multiple times
// while the reachability probe was in flight fired a probe (and potentially
// a full server switch) per tap, with zero visual feedback in between.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", {}, children) : null),
  Pressable: ({ children, onPress, accessibilityLabel, disabled, accessibilityState, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        disabled,
        "aria-busy": accessibilityState?.busy,
        ...p,
      },
      children,
    ),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "switch-spinner", ...p }),
  Alert: { alert: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("span", {}) }));
vi.mock("expo-router", () => ({ router: { push: vi.fn(), replace: vi.fn() } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", error: "#f00", onAccent: "#fff" },
    radii: { md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

const SERVER = { id: "srv-1", url: "https://evey.example", label: "Home", lastUsedAt: 1 };
vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useServerListStore: (selector: (s: any) => unknown) =>
    selector({
      servers: { "srv-1": SERVER },
      activeServerId: "other",
      setActiveServer: vi.fn(),
      removeServer: vi.fn(),
      renameServer: vi.fn(),
      setServerName: vi.fn(),
    }),
  useAuthStore: { getState: () => ({ accessToken: "tok" }) },
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@evey/api", () => ({ setBaseURL: vi.fn() }));
vi.mock("../lib/fetcher-setup", () => ({ initFetcher: vi.fn(async () => undefined) }));

type ProbeResult = { probe: string; health: { status: string; version: string; name?: string } | null };
let resolveProbe: (v: ProbeResult) => void;
const probeServerMock = vi.fn(
  () =>
    new Promise<ProbeResult>((resolve) => {
      resolveProbe = resolve;
    }),
);
vi.mock("../lib/server-config", () => ({
  probeServerHealth: (...a: unknown[]) => probeServerMock(...a),
  setServerUrl: vi.fn(async () => undefined),
}));

import ManageServersScreen from "../../app/(auth)/manage-servers";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("manage servers - double-tap guard while probing", () => {
  it("shows a busy indicator and ignores extra taps until the probe resolves", async () => {
    render(React.createElement(ManageServersScreen));

    const row = screen.getByLabelText("Home");
    fireEvent.click(row);
    fireEvent.click(row);
    fireEvent.click(row);

    await waitFor(() => expect(screen.getByTestId("switch-spinner")).toBeTruthy());
    // Only the first tap actually started a probe.
    expect(probeServerMock).toHaveBeenCalledTimes(1);

    resolveProbe({ probe: "ok", health: { status: "ok", version: "1" } });
    await waitFor(() => expect(screen.queryByTestId("switch-spinner")).toBeNull());
  });
});
