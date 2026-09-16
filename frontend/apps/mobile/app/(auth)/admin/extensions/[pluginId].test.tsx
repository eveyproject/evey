import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The dashboard row opens ONE plugin's admin page on its own screen (web
// parity with /admin/extensions/<id>): the header carries the plugin's name
// when the slot knows it, and a missing id is a notice rather than a blank
// WebView.
vi.mock("react-native", () => ({
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
const params = { current: { pluginId: "stats" } as Record<string, string | undefined> };
vi.mock("expo-router", () => ({
  Stack: { Screen: (p: any) => React.createElement("div", { "data-testid": "screen-title" }, p.options?.title) },
  useLocalSearchParams: () => params.current,
}));
vi.mock("../../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888" } }),
}));
vi.mock("../../../../src/components/layout/HeaderBack", () => ({ HeaderBack: () => null }));
const entries = { current: [] as { pluginId: string; name: string }[] };
vi.mock("../../../../src/features/plugins/PluginSlot", () => ({
  usePluginSlotEntries: (slot: string) => (slot === "admin.nav" ? entries.current : []),
  PluginSlot: ({ name, pluginId }: { name: string; pluginId?: string }) =>
    React.createElement("div", { "data-testid": "plugin-slot", "data-slot": name, "data-plugin": pluginId ?? "" }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => fallback ?? _k }) }));

async function renderScreen() {
  const mod = await import("./[pluginId]");
  render(React.createElement(mod.default));
}

beforeEach(() => {
  params.current = { pluginId: "stats" };
  entries.current = [];
});

describe("admin plugin extension screen (mobile)", () => {
  it("opens the plugin named in the route in the admin.nav slot, titled by its name", async () => {
    entries.current = [{ pluginId: "other", name: "Other" }, { pluginId: "stats", name: "Stats" }];
    await renderScreen();
    const slot = screen.getByTestId("plugin-slot");
    expect(slot.getAttribute("data-slot")).toBe("admin.nav");
    expect(slot.getAttribute("data-plugin")).toBe("stats");
    expect(screen.getByTestId("screen-title").textContent).toBe("Stats");
  });

  it("falls back to a generic title for a plugin the slot does not know", async () => {
    await renderScreen();
    expect(screen.getByTestId("screen-title").textContent).toBe("Plugin");
    expect(screen.getByTestId("plugin-slot").getAttribute("data-plugin")).toBe("stats");
  });

  it("shows a notice instead of a slot when the route carries no id", async () => {
    params.current = {};
    await renderScreen();
    expect(screen.queryByTestId("plugin-slot")).toBeNull();
    expect(screen.getByText(/not installed, enabled, or has no admin page/)).toBeTruthy();
  });
});
