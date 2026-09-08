import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The settings tab used to stack every plugin's WebView inline; each plugin
// now gets this screen, so what it must prove is that it opens the plugin the
// menu row asked for, and only that one.
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  StyleSheet: { create: (s: any) => s },
}));

const params = { current: { pluginId: "weather" } as Record<string, string> };
const backMock = vi.fn();
vi.mock("expo-router", () => ({
  Stack: { Screen: (p: any) => React.createElement("div", { "data-testid": "screen-title" }, p.options?.title) },
  useLocalSearchParams: () => params.current,
  router: { back: (...a: unknown[]) => backMock(...a) },
}));

vi.mock("../../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888" } }),
}));

vi.mock("../../../../src/components/FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
}));

const entries = { current: [] as { pluginId: string; name: string }[] };
vi.mock("../../../../src/features/plugins/PluginSlot", () => ({
  usePluginSlotEntries: (slot: string) => (slot === "settings.user.appearance" ? entries.current : []),
  PluginSlot: ({ name, pluginId }: { name: string; pluginId?: string }) =>
    React.createElement("div", { "data-testid": "plugin-slot", "data-slot": name, "data-plugin": pluginId ?? "" }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, fallback?: string) => fallback ?? _k }),
}));

async function renderScreen() {
  const mod = await import("./[pluginId]");
  return render(React.createElement(mod.default));
}

describe("plugin user-settings screen (TV)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    params.current = { pluginId: "weather" };
    entries.current = [
      { pluginId: "weather", name: "Weather" },
      { pluginId: "notes", name: "Notes" },
    ];
  });

  it("mounts the user-settings slot for the plugin in the route, and no other", async () => {
    await renderScreen();
    const slot = screen.getByTestId("plugin-slot");
    expect(slot.getAttribute("data-slot")).toBe("settings.user.appearance");
    expect(slot.getAttribute("data-plugin")).toBe("weather");
  });

  it("titles the screen with the plugin name so the reader knows whose settings these are", async () => {
    params.current = { pluginId: "notes" };
    await renderScreen();
    expect(screen.getByTestId("screen-title").textContent).toBe("Notes");
  });

  it("says so instead of mounting an empty frame when the route carries no plugin", async () => {
    params.current = {};
    await renderScreen();
    expect(screen.queryByTestId("plugin-slot")).toBeNull();
  });
});
