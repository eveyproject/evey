import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// The region row had no accessibilityRole/Label/State at all - a screen
// reader could not tell it was selectable, let alone which one was active
// (the check mark was purely visual).

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      {},
      data.map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item }))),
    ),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityRole, accessibilityState, style: _style, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        role: accessibilityRole,
        "aria-checked": accessibilityState?.selected,
        ...p,
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: "en" },
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333" },
    radii: { md: 8 },
    spacing: { md: 12 },
  }),
}));

const PROFILE = { id: "p1", region: "FR" };
vi.mock("../hooks/useActiveProfile", () => ({
  useActiveProfile: () => ({ profile: PROFILE, profiles: [PROFILE], isLoading: false }),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
}));

import RegionScreen from "../../app/(auth)/settings/region";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("region settings - row accessibility", () => {
  it("exposes the region row as a labeled, stateful radio", () => {
    render(<RegionScreen />, { wrapper: makeWrapper() });

    const selected = screen.getByRole("radio", { name: "FR" });
    expect(selected.getAttribute("aria-checked")).toBe("true");

    const otherOption = screen.getAllByRole("radio").find((el) => el.getAttribute("aria-label") !== "FR");
    expect(otherOption?.getAttribute("aria-checked")).toBe("false");
  });
});
