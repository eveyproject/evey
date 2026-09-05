// Quick Connect used to be the only sign-in path that dropped the refresh
// token the server hands it (CheckQuickConnect calls GenerateTokensForUser
// exactly like a password login). The session then lasted only as long as the
// 15-minute access token: at expiry the refresher found nothing in SecureStore,
// read the absence as a dead session, and logged the user back out. Nothing
// failed loudly - the sign-in itself worked - which is why it needs a test.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
}));

vi.mock("react-native-qrcode-svg", () => ({
  default: () => React.createElement("div", { "data-testid": "qr" }),
}));

const replaceMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { replace: (...a: unknown[]) => replaceMock(...a), push: vi.fn(), back: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

const checkMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: ({ mutation }: any) => ({
    mutate: () => mutation.onSuccess({ code: "abc123" }),
  }),
  userServiceCheckQuickConnect: (...a: unknown[]) => checkMock(...a),
}));

const setTokensMock = vi.fn();
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ setTokens: setTokensMock }),
}));

const setRefreshTokenMock = vi.fn(async () => undefined);
vi.mock("../lib/refresh-token", () => ({
  setRefreshToken: (...a: unknown[]) => setRefreshTokenMock(...a),
}));

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
  setRefreshTokenMock.mockResolvedValue(undefined);
});

describe("quick connect - the session it opens must survive the access token", () => {
  it("stores the refresh token the server returns alongside the access token", async () => {
    checkMock.mockResolvedValue({
      authorized: true,
      auth: { accessToken: "acc-1", refreshToken: "rt-1" },
    });

    render(React.createElement(QuickConnectScreen));

    await waitFor(() => expect(setTokensMock).toHaveBeenCalledWith("acc-1"));
    // The whole point: without this the session dies at the 15-minute mark.
    await waitFor(() => expect(setRefreshTokenMock).toHaveBeenCalledWith("rt-1"));
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  it("does not overwrite a stored refresh token when the answer carries none", async () => {
    checkMock.mockResolvedValue({ authorized: true, auth: { accessToken: "acc-2" } });

    render(React.createElement(QuickConnectScreen));

    await waitFor(() => expect(setTokensMock).toHaveBeenCalledWith("acc-2"));
    expect(setRefreshTokenMock).not.toHaveBeenCalled();
  });

  it("keeps polling and signs nobody in while the code is unauthorized", async () => {
    checkMock.mockResolvedValue({ authorized: false });

    render(React.createElement(QuickConnectScreen));

    await waitFor(() => expect(checkMock).toHaveBeenCalled());
    expect(setTokensMock).not.toHaveBeenCalled();
    expect(setRefreshTokenMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalled();
  });
});
