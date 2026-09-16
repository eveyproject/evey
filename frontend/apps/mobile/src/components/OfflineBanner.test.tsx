import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React, { useSyncExternalStore } from "react";

vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel }: any) => React.createElement("div", { role: "alert", "aria-label": accessibilityLabel }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k }) }));
vi.mock("react-native-safe-area-context", () => ({ useSafeAreaInsets: () => ({ top: 20 }) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { warning: "#fa0", onWarning: "#000" } }) }));
const nav = vi.hoisted(() => ({ pathname: "/home", push: vi.fn() }));
vi.mock("expo-router", () => ({ usePathname: () => nav.pathname, router: { push: (...a: unknown[]) => nav.push(...a) } }));
// The online flag is a tiny external store so a flip re-renders the memoised
// banner the way the real hook would.
const net = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  let online = true;
  return {
    get: () => online,
    set: (v: boolean) => {
      online = v;
      listeners.forEach((l) => l());
    },
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  };
});
vi.mock("../hooks/useOnlineStatus", () => ({ useOnlineStatus: () => useSyncExternalStore(net.subscribe, net.get) }));

import { OfflineBanner } from "./OfflineBanner";

beforeEach(() => {
  vi.useFakeTimers();
  nav.pathname = "/home";
  net.set(true);
});
afterEach(() => vi.useRealTimers());

// The strip announces the online -> offline transition and gets out of the
// way after five seconds (offline is a supported mode, not an error); it
// comes back on the next transition, vanishes as soon as the network is
// back, and never covers the player's own chrome.
describe("OfflineBanner", () => {
  it("shows on the transition, hides after five seconds, and returns on the next drop", () => {
    render(<OfflineBanner />);
    expect(screen.queryByRole("alert")).toBeNull();
    act(() => net.set(false));
    expect(screen.getByRole("alert")).toBeTruthy();
    act(() => vi.advanceTimersByTime(4999));
    expect(screen.getByRole("alert")).toBeTruthy();
    act(() => vi.advanceTimersByTime(1));
    expect(screen.queryByRole("alert")).toBeNull();
    act(() => net.set(true));
    act(() => net.set(false));
    expect(screen.getByRole("alert")).toBeTruthy();
    act(() => net.set(true));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Offline the one screen that works is Downloads, so the strip is the way
  // there - except when the viewer already is there.
  it("offers the way to Downloads, except from Downloads itself", () => {
    render(<OfflineBanner />);
    act(() => net.set(false));
    act(() => { screen.getByRole("button", { name: "Open my downloads" }).click(); });
    expect(nav.push).toHaveBeenCalledWith("/(auth)/downloads");
    act(() => net.set(true));
    nav.pathname = "/downloads";
    act(() => net.set(false));
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open my downloads" })).toBeNull();
  });

  it("never shows over the player", () => {
    nav.pathname = "/player/m1";
    render(<OfflineBanner />);
    act(() => net.set(false));
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
