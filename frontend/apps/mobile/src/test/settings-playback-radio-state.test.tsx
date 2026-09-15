import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A screen reader has no other way to tell which bitrate/codec/language is
// active: the check mark was purely visual (accessibilityRole="radio" with no
// accessibilityState). This guards that every option row now reports its
// selection through accessibilityState, not just through a glyph.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Switch: (p: any) => React.createElement("input", { type: "checkbox", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityRole, accessibilityState, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        role: accessibilityRole,
        "aria-checked": accessibilityState?.selected,
        "aria-disabled": accessibilityState?.disabled,
        ...p,
      },
      children,
    ),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  TextInput: (p: any) => React.createElement("input", p),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));

// The language pickers open a BottomSheet (native deps); stub it to a passthrough
// so this radio-accessibility test keeps rendering the DOM-only screen.
vi.mock("../components/BottomSheet", () => ({ BottomSheet: () => null }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key), i18n: { language: "en" } }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333" },
    radii: { md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

const PROFILE = { id: "p1", preferences: { max_bitrate_kbps: "4000" } };
vi.mock("../hooks/useActiveProfile", () => ({
  useActiveProfile: () => ({ profile: PROFILE, profiles: [PROFILE], isLoading: false }),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
  eveyFetch: () => Promise.resolve({}),
}));

import PlaybackSettingsScreen from "../../app/(auth)/settings/playback";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("playback settings - radio accessibility state", () => {
  it("marks the profile's current bitrate as selected and the others as not", async () => {
    render(<PlaybackSettingsScreen />, { wrapper: makeWrapper() });

    // "4 Mbps" appears in both the WiFi and cellular bitrate groups; the
    // profile only sets the WiFi one (max_bitrate_kbps), so only the first
    // match should report itself selected.
    await waitFor(() => {
      const wifiOption = screen.getAllByRole("radio", { name: /4 Mbps/ })[0];
      expect(wifiOption.getAttribute("aria-checked")).toBe("true");
    });

    const [wifiOption, cellularOption] = screen.getAllByRole("radio", { name: /4 Mbps/ });
    expect(wifiOption.getAttribute("aria-checked")).toBe("true");
    expect(cellularOption.getAttribute("aria-checked")).toBe("false");

    // The default-selected "Unlimited" WiFi option lost its selection once
    // the profile's saved bitrate applied.
    expect(screen.getAllByRole("radio", { name: "Unlimited" })[0].getAttribute("aria-checked")).toBe("false");
  });
});
