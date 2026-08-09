import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  Modal: ({ children, visible }: any) =>
    // RN Modal keeps children MOUNTED when visible=false (it only hides the
    // native presentation) - the mock mirrors that so the test below means
    // something.
    React.createElement("div", { "data-visible": String(visible) }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
  Animated: {
    View: ({ children, ...p }: any) => React.createElement("div", p, children),
    Value: class { setValue() {} interpolate() { return 0; } },
    timing: () => ({ start: (cb?: () => void) => cb?.() }),
    spring: () => ({ start: (cb?: () => void) => cb?.() }),
    parallel: () => ({ start: (cb?: () => void) => cb?.() }),
  },
}));
vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, f?: string) => f ?? k }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" },
    radii: { lg: 12, md: 8, sm: 4 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));
vi.mock("@expo/vector-icons", () => ({
  Ionicons: () => React.createElement("span"),
}));

import { BottomSheet } from "./BottomSheet";

// A closed sheet must render NOTHING. RN's Modal keeps children mounted when
// visible=false, so every child of every sheet re-rendered on each of the
// player's ~2/s currentTime ticks, playback-long, on every low-end phone.
describe("BottomSheet mount discipline", () => {
  it("renders no children while hidden", () => {
    const probe = vi.fn(() => null);
    const Probe = () => probe();
    render(
      <BottomSheet visible={false} onClose={() => {}} title="t">
        <Probe />
      </BottomSheet>,
    );
    expect(probe).not.toHaveBeenCalled();
  });

  it("renders children when visible", () => {
    const probe = vi.fn(() => React.createElement("span", null, "content"));
    const Probe = () => probe();
    render(
      <BottomSheet visible={true} onClose={() => {}} title="t">
        <Probe />
      </BottomSheet>,
    );
    expect(probe).toHaveBeenCalled();
    expect(screen.getByText("content")).toBeTruthy();
  });
});
