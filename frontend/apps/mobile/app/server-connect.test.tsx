import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The connect screen now lets a saved server be deleted before sign-in, so a
// stale address can be removed without first logging into it. Delete is
// confirmed by name, and the only server the viewer is currently on cannot be
// deleted (it would strand them with nothing to connect to).
const alerts = vi.hoisted(() => ({ last: null as null | { title: string; message?: string; buttons?: { text: string; style?: string; onPress?: () => void }[] } }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message?: string, buttons?: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, fallback?: string, o?: { name?: string }) => (fallback ?? k).replace("{{name}}", o?.name ?? "") }),
}));
vi.mock("@evey/api", () => ({ setBaseURL: vi.fn(), isRemoteAccessDisabled: () => false, remoteAccessDisabledMessage: () => "" }));
const store = vi.hoisted(() => ({
  servers: {} as Record<string, { id: string; url: string; label?: string; serverName?: string; lastUsedAt: number }>,
  activeServerId: "s1" as string | null,
  addServer: vi.fn(), setActiveServer: vi.fn(), removeServer: vi.fn(), setServerName: vi.fn(),
  accessToken: null as string | null,
}));
vi.mock("@evey/store", () => ({
  useServerListStore: (sel: (s: typeof store) => unknown) => sel(store),
  useAuthStore: { getState: () => ({ accessToken: store.accessToken }) },
  serverDisplayName: (s: { label?: string; serverName?: string; url: string }) => s.label || s.serverName || s.url,
  isDefaultServerName: () => false,
}));
vi.mock("../src/lib/server-config", () => ({
  setServerUrl: vi.fn(async () => {}),
  getServerUrl: vi.fn(async () => null),
  probeServerHealth: vi.fn(async () => ({ probe: "ok", health: { name: "Home" } })),
}));
vi.mock("../src/lib/fetcher-setup", () => ({ initFetcher: vi.fn(async () => {}) }));
vi.mock("../src/i18n", () => ({ applyServerLanguage: vi.fn(async () => {}), initI18n: vi.fn(async () => {}) }));
vi.mock("../src/lib/serverIdentity", () => ({ parseEveyHealth: (b: unknown) => b }));
vi.mock("../src/lib/privateHost", () => ({ isCleartextToPublicHost: () => false, confirmInsecureServer: vi.fn(async () => true) }));
vi.mock("../src/lib/versionLabel", () => ({ versionLabel: () => "" }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10, sm: 12, md: 14, lg: 18 } }));
vi.mock("../src/lib/network-scan", () => ({ scanLocalNetwork: vi.fn(async () => []) }));
vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", error: "#f33" }, radii: { md: 8, sm: 4 }, spacing: { xs: 4, sm: 8, md: 16, lg: 24, xl: 32 } }),
}));

import ServerConnect from "./server-connect";

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  store.activeServerId = "s1";
});

describe("ServerConnect saved-server delete", () => {
  it("deletes a non-active saved server after confirmation", () => {
    store.servers = {
      s1: { id: "s1", url: "https://home", label: "Home", lastUsedAt: 1 },
      s2: { id: "s2", url: "https://old", label: "Old", lastUsedAt: 2 }, // listed first (newer)
    };
    render(React.createElement(ServerConnect));
    const deletes = screen.getAllByRole("button", { name: "Delete" });
    // s2 is first (sorted by lastUsedAt desc).
    fireEvent.click(deletes[0]);
    expect(alerts.last?.title).toBe("Delete server?");
    const destructive = alerts.last?.buttons?.find((b) => b.style === "destructive");
    destructive?.onPress?.();
    expect(store.removeServer).toHaveBeenCalledWith("s2");
  });

  it("refuses to delete the only server the viewer is on", () => {
    store.servers = { s1: { id: "s1", url: "https://home", label: "Home", lastUsedAt: 1 } };
    store.activeServerId = "s1";
    render(React.createElement(ServerConnect));
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(alerts.last?.title).toBe("Can't delete the only saved server");
    expect(alerts.last?.buttons).toBeUndefined();
    expect(store.removeServer).not.toHaveBeenCalled();
  });
});
