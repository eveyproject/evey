import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The header title renders inside the NATIVE toolbar, which clips children to
// the bar's bounds on both platforms - an in-header result dropdown can never
// show on device (it only worked in jsdom tests, where nothing clips). The
// component is now a trigger that opens the search screen, which owns the
// live-as-you-type results; these tests pin that navigation contract.

vi.mock("react-native", () => ({
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("expo-router", () => ({ router: { navigate: vi.fn() } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surfaceRaised: "#222", border: "#333" },
  }),
}));

import { router } from "expo-router";
import { HeaderSearch } from "./HeaderSearch";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("HeaderSearch - navigation trigger", () => {
  it("opens the search screen with a focus param on press", () => {
    render(<HeaderSearch />);

    fireEvent.click(screen.getByLabelText("Search movies, series…"));

    expect(router.navigate).toHaveBeenCalledOnce();
    const arg = vi.mocked(router.navigate).mock.calls[0][0] as any;
    expect(arg.pathname).toBe("/(auth)/(tabs)/search");
    expect(arg.params.focus).toBeTruthy();
  });

  it("sends a fresh focus value on each press so an already-open search screen re-focuses", async () => {
    render(<HeaderSearch />);
    const trigger = screen.getByLabelText("Search movies, series…");

    fireEvent.click(trigger);
    await new Promise((r) => setTimeout(r, 2));
    fireEvent.click(trigger);

    const calls = vi.mocked(router.navigate).mock.calls;
    expect(calls).toHaveLength(2);
    const first = (calls[0][0] as any).params.focus;
    const second = (calls[1][0] as any).params.focus;
    expect(first).not.toBe(second);
  });
});
