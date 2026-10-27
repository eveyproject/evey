import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
}));

vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      text: "#fff",
      textMuted: "#aaa",
      accent: "#88f",
      border: "#333",
      onAccent: "#000",
      error: "#f55",
    },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 16, lg: 24, xl: 32 },
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

vi.mock("react-native-qrcode-svg", () => ({
  default: () => React.createElement("div", { "data-testid": "qrcode" }),
}));

const replace = vi.fn();
const dismissAll = vi.fn();
vi.mock("expo-router", () => ({
  router: { replace: (...a: unknown[]) => replace(...a), back: vi.fn(), canDismiss: () => true, dismissAll: (...a: unknown[]) => dismissAll(...a) },
}));

const setTokens = vi.fn();
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { setTokens: typeof setTokens }) => unknown) => sel({ setTokens }),
  // The server url hook reads the active server from this store first.
  useServerListStore: (sel: (s: { getActiveServer: () => { url: string } | null }) => unknown) =>
    sel({ getActiveServer: () => ({ url: "http://srv.test" }) }),
}));

vi.mock("../src/hooks/useServerUrl", () => ({
  useServerUrlNullable: () => "http://srv.test",
}));

const checkQuickConnect = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: (opts: { mutation: { onSuccess: (d: { code: string }) => void } }) => ({
    mutate: () => opts.mutation.onSuccess({ code: "abcdef123456" }),
  }),
  checkQuickConnect: (...a: unknown[]) => checkQuickConnect(...a),
}));

import QuickConnectScreen from "./quick-connect";

describe("QuickConnectScreen", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  // The web screen waits on SSE, but EventSource is not a React Native
  // global: the TV must poll CheckQuickConnect like mobile does, and the
  // screen must not touch any browser-only API to survive rendering.
  it("polls CheckQuickConnect and stores tokens once authorized", async () => {
    checkQuickConnect.mockResolvedValue({
      authorized: true,
      auth: { accessToken: "tok-123" },
    });

    // The first poll fires from a mount effect and settles asynchronously:
    // flushing it inside act keeps React from flagging the state update.
    await act(async () => {
      render(<QuickConnectScreen />);
    });

    await waitFor(() => {
      expect(checkQuickConnect).toHaveBeenCalledWith("abcdef123456", null);
      expect(setTokens).toHaveBeenCalledWith("tok-123");
      expect(replace).toHaveBeenCalledWith("/(auth)/(tabs)");
    // The login page it was pushed over must go too, or BACK on the home shows it again.
    expect(dismissAll).toHaveBeenCalled();
    });
  });

  it("keeps polling while the code is not yet authorized", async () => {
    vi.useFakeTimers();
    checkQuickConnect.mockResolvedValue({ authorized: false });

    await act(async () => {
      render(<QuickConnectScreen />);
    });

    // Each interval tick polls and updates the countdown - both inside act.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3100);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3100);
    });
    // Immediate first tick + one per 3s interval.
    expect(checkQuickConnect.mock.calls.length).toBeGreaterThanOrEqual(3);
    expect(setTokens).not.toHaveBeenCalled();
    expect(replace).not.toHaveBeenCalled();
  });
});
