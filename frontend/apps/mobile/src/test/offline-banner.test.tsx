import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: (p: any) => React.createElement("i", p),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, fallback: string) => fallback }),
}));

vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { warning: "#f80", onWarning: "#000" } }),
}));

// Both signals the banner reacts to, mutable per test. The online mock is a
// real external store (like the production useSyncExternalStore over React
// Query's onlineManager): the component is memo()ed with no props, so only a
// hook-driven re-render can reach it - a plain value mock would freeze it on
// its mount-time answer.
const { state, listeners } = vi.hoisted(() => ({
  state: { online: true, pathname: "/home" },
  listeners: new Set<() => void>(),
}));
function setOnline(v: boolean) {
  state.online = v;
  for (const l of listeners) l();
}
vi.mock("../hooks/useOnlineStatus", async () => {
  const R = await import("react");
  return {
    useOnlineStatus: () =>
      R.useSyncExternalStore(
        (cb: () => void) => {
          listeners.add(cb);
          return () => listeners.delete(cb);
        },
        () => state.online,
      ),
  };
});
vi.mock("expo-router", () => ({
  usePathname: () => state.pathname,
  router: { push: vi.fn() },
}));

import { OfflineBanner } from "../components/OfflineBanner";

beforeEach(() => {
  vi.useFakeTimers();
  state.online = true;
  state.pathname = "/home";
});

afterEach(() => {
  vi.useRealTimers();
});

describe("OfflineBanner", () => {
  it("renders nothing while online", () => {
    render(<OfflineBanner />);
    expect(screen.queryByText("Offline")).toBeNull();
  });

  it("announces the offline transition, then auto-hides", () => {
    state.online = false;
    render(<OfflineBanner />);
    expect(screen.queryByText("Offline")).not.toBeNull();

    // Offline is a supported mode (downloads play fine): after the
    // announcement the strip must get out of the way on its own.
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.queryByText("Offline")).toBeNull();
  });

  it("never shows over the player, even inside the announcement window", () => {
    state.online = false;
    state.pathname = "/player/abc-123";
    render(<OfflineBanner />);
    expect(screen.queryByText("Offline")).toBeNull();
  });

  it("hides immediately when connectivity returns", () => {
    state.online = false;
    render(<OfflineBanner />);
    expect(screen.queryByText("Offline")).not.toBeNull();

    act(() => {
      setOnline(true);
    });
    expect(screen.queryByText("Offline")).toBeNull();
  });
});
