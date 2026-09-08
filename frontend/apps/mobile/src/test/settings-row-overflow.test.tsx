// Row labels sit in a flexDirection: "row" with no flex constraint (RN Text
// defaults to flexShrink: 0), so a long French/German string pushed the
// value/checkmark out of the row and the section clipped it. These tests
// mount the real screens and read back the flattened style RN would compute,
// locking in that the label can shrink/wrap and the trailing element cannot
// be pushed off screen.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

function flattenStyle(style: unknown): Record<string, unknown> {
  if (!style) return {};
  if (Array.isArray(style)) {
    return style.reduce<Record<string, unknown>>((acc, s) => ({ ...acc, ...flattenStyle(s) }), {});
  }
  return style as Record<string, unknown>;
}

vi.mock("react-native", () => {
  const el = (tag: string) =>
    ({ children, style, numberOfLines, ...rest }: any) =>
      React.createElement(
        tag,
        {
          "data-style": JSON.stringify(flattenStyle(style)),
          "data-numberoflines": numberOfLines,
          "aria-label": rest.accessibilityLabel,
          role: rest.accessibilityRole,
        },
        children,
      );
  return {
    View: el("div"),
    Text: el("span"),
    ScrollView: el("div"),
    TextInput: (p: any) => React.createElement("input", p),
    ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
    Pressable: ({ children, onPress, style, ...rest }: any) => {
      const resolved = typeof style === "function" ? style({ pressed: false }) : style;
      return React.createElement(
        "button",
        {
          onClick: onPress,
          "data-style": JSON.stringify(flattenStyle(resolved)),
          "aria-label": rest.accessibilityLabel,
          role: rest.accessibilityRole,
        },
        children,
      );
    },
    FlatList: ({ data, renderItem, keyExtractor }: any) =>
      React.createElement(
        "div",
        {},
        (data ?? []).map((item: any, i: number) =>
          React.createElement(
            React.Fragment,
            { key: keyExtractor ? keyExtractor(item, i) : i },
            renderItem({ item, index: i }),
          ),
        ),
      ),
    Alert: { alert: vi.fn() },
    StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  };
});

vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", surfaceRaised: "#222", border: "#333" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

const PROFILE = { id: "p1", region: "FR" };
vi.mock("../hooks/useActiveProfile", () => ({
  useActiveProfile: () => ({ profile: PROFILE, profiles: [PROFILE], isLoading: false }),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
  useNotificationServiceGetNotificationSettings: () => ({
    data: { movieLevel: "NOTIFICATION_LEVEL_ALL", seriesLevel: "NOTIFICATION_LEVEL_ALL", musicLevel: "NOTIFICATION_LEVEL_ALL" },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
  useNotificationServiceUpdateNotificationSettings: () => ({ mutate: vi.fn(), isPending: false }),
  apiErrorMessage: (_err: unknown, _t: unknown, fallback: string) => fallback,
}));

import RegionScreen from "../../app/(auth)/settings/region";
import NotificationSettingsScreen from "../../app/(auth)/settings/notifications";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("region settings row - label shrinks, checkmark stays put", () => {
  it("gives the country label a flex constraint so it can shrink/wrap instead of pushing the checkmark out", () => {
    render(<RegionScreen />, { wrapper: makeWrapper() });

    // "FR" is the selected row (see PROFILE above), so it is the one with a checkmark.
    const selectedRow = screen.getByRole("radio", { name: "FR" });
    const label = selectedRow.querySelector("span")!;
    const labelStyle = JSON.parse(label.getAttribute("data-style") || "{}");
    expect(labelStyle.flex).toBe(1);
    expect(label.getAttribute("data-numberoflines")).toBe("2");

    const check = selectedRow.querySelectorAll("span")[1]!;
    const checkStyle = JSON.parse(check.getAttribute("data-style") || "{}");
    expect(checkStyle.flexShrink).toBe(0);
  });
});

describe("notifications settings LevelPicker row - label shrinks, checkmark stays put", () => {
  it("gives the level label a flex constraint and keeps the checkmark from being squeezed out", () => {
    render(<NotificationSettingsScreen />, { wrapper: makeWrapper() });

    // NOTIFICATION_LEVEL_ALL is selected for every category above, so any of
    // the "all" rows carries a checkmark.
    const rows = screen.getAllByText("auth.notificationSettings.levelAll");
    const label = rows[0]!;
    const labelStyle = JSON.parse(label.getAttribute("data-style") || "{}");
    expect(labelStyle.flex).toBe(1);

    const row = label.closest("button")!;
    const check = Array.from(row.querySelectorAll("span")).find((el) => el.textContent === "✓")!;
    const checkStyle = JSON.parse(check.getAttribute("data-style") || "{}");
    expect(checkStyle.flexShrink).toBe(0);
  });
});
