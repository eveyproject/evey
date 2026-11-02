import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, fireEvent, screen, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus: _f, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
  DeviceEventEmitter: { addListener: () => ({ remove: () => {} }), emit: () => {} },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => {
  const router = { push: vi.fn() };
  return { Slot: () => null, useRouter: () => router, usePathname: () => "/" };
});
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../../src/providers/ThemeProvider", () => {
  const theme = { colors: { accent: "#f00", textMuted: "#888", text: "#fff", surface: "#111", border: "#333" } };
  return { useTheme: () => theme };
});
vi.mock("../../../src/components/EveyLogo", () => ({ EveyWordmark: () => null, EveyMark: () => null }));
vi.mock("../../../src/components/SearchOverlay", () => ({ SearchOverlay: () => null }));
const headerRenders = { n: 0 };
vi.mock("../../../src/components/TopHeader", async () => {
  const { memo } = await import("react");
  return {
    TopHeader: memo(function TopHeader() {
      headerRenders.n += 1;
      return null;
    }),
  };
});

import TabsLayout from "./_layout";

const label = (key: string) => screen.queryByText(key);

describe("TabsLayout (TV) side rail", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    headerRenders.n = 0;
  });
  afterEach(() => vi.useRealTimers());

  it("stays unfolded while the focus moves between entries, and folds once it leaves", () => {
    render(<TabsLayout />);
    const home = screen.getByLabelText("common.nav.home");
    const libraries = screen.getByLabelText("common.nav.libraries");
    expect(label("common.nav.home")).toBeNull();

    fireEvent.focus(home);
    expect(label("common.nav.home")).toBeTruthy();
    const headerAfterUnfold = headerRenders.n;

    // Blur then focus, as a D-pad press down the rail delivers them.
    fireEvent.blur(home);
    fireEvent.focus(libraries);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(label("common.nav.home")).toBeTruthy();
    // Focus first, then the previous entry's blur: still unfolded.
    fireEvent.focus(home);
    fireEvent.blur(libraries);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(label("common.nav.home")).toBeTruthy();
    expect(headerRenders.n).toBe(headerAfterUnfold);

    fireEvent.blur(home);
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(label("common.nav.home")).toBeNull();
  });
});
