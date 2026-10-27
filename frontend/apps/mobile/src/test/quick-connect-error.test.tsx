// A failed InitiateQuickConnect call left the screen on the QR spinner
// forever - no error, no retry, just an indicator that would never resolve.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (styles: any) => styles },
}));

vi.mock("react-native-qrcode-svg", () => ({
  default: () => React.createElement("div", { "data-testid": "qr" }),
}));

vi.mock("expo-router", () => ({
  router: { replace: vi.fn(), push: vi.fn(), back: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

const checkMock = vi.fn();
vi.mock("@evey/api", () => ({
  // The server never answers: mutate() calls onError instead of onSuccess,
  // exactly like a failed InitiateQuickConnect network call.
  useUserServiceInitiateQuickConnect: ({ mutation }: any) => ({
    mutate: () => mutation.onError?.(new Error("network down")),
    isError: true,
  }),
  checkQuickConnect: (...a: unknown[]) => checkMock(...a),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ setTokens: vi.fn() }),
}));

vi.mock("../lib/refresh-token", () => ({ setRefreshToken: vi.fn(async () => undefined) }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333" },
    radii: { md: 8 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrlNullable: () => "http://srv.test" }));

import QuickConnectScreen from "../../app/quick-connect";

beforeEach(() => {
  vi.clearAllMocks();
  checkMock.mockResolvedValue({ authorized: false });
});

describe("quick connect - a failed initiate must not spin forever", () => {
  it("shows a retry instead of an infinite loading indicator when initiate fails", () => {
    render(React.createElement(QuickConnectScreen));

    expect(screen.getByLabelText("common.retry")).toBeTruthy();
    expect(screen.queryByTestId("loading")).toBeNull();
    expect(screen.queryByTestId("qr")).toBeNull();
  });
});
