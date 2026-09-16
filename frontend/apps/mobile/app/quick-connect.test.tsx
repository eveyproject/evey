import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

// The phone mints a code, shows its tail and a QR pointing at the server,
// polls every 3 s for the authorisation, and on success stores BOTH tokens
// (dropping the refresh token once made this the only sign-in that died
// after 15 minutes) before landing on the tabs. The code is re-minted every
// two minutes; a failed mint offers a retry.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
}));
vi.mock("react-native-qrcode-svg", () => ({ default: ({ value }: any) => React.createElement("div", { "data-testid": "qr" }, value) }));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}:${Object.values(o).join(",")}` : k) }),
}));
const api = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn(), isError: false, check: vi.fn() }));
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: (o: any) => {
    api.opts = o.mutation;
    return { mutate: api.mutate, isError: api.isError };
  },
  userServiceCheckQuickConnect: (...a: unknown[]) => api.check(...a),
}));
const setTokens = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { setTokens: typeof setTokens }) => unknown) => sel({ setTokens }) }));
const setRefreshToken = vi.hoisted(() => vi.fn(async () => {}));
vi.mock("../src/lib/refresh-token", () => ({ setRefreshToken: (...a: unknown[]) => setRefreshToken(...(a as [])) }));
vi.mock("../src/hooks/useServerUrl", () => ({ useServerUrlNullable: () => "https://s" }));
vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000" }, radii: { md: 8 }, spacing: { md: 16, lg: 24, xl: 32 } }),
}));
vi.mock("../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import QuickConnectScreen from "./quick-connect";

beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  api.isError = false;
  api.check.mockResolvedValue({ authorized: false });
});
afterEach(() => vi.useRealTimers());

describe("quick connect screen (mobile)", () => {
  it("mints a code, shows its tail and a QR to the server, and polls until authorised", async () => {
    render(<QuickConnectScreen />);
    expect(api.mutate).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    act(() => api.opts.onSuccess({ code: "long-quick-code-abc123" }));
    expect(screen.getByText("ABC123")).toBeTruthy();
    expect(screen.getByTestId("qr").textContent).toBe("https://s/quickconnect?code=long-quick-code-abc123");
    expect(screen.getByText("auth.settings.quickConnect.expiresIn:120")).toBeTruthy();
    await act(async () => {});
    expect(api.check).toHaveBeenCalledWith("long-quick-code-abc123");

    api.check.mockResolvedValueOnce({ authorized: true, auth: { accessToken: "acc", refreshToken: "ref" } });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(setTokens).toHaveBeenCalledWith("acc");
    expect(setRefreshToken).toHaveBeenCalledWith("ref");
    expect(router.replace).toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  it("counts down and re-mints the code every two minutes; a failed mint offers a retry", async () => {
    const first = render(<QuickConnectScreen />);
    act(() => api.opts.onSuccess({ code: "c1" }));
    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.getByText("auth.settings.quickConnect.expiresIn:115")).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(115_000); });
    expect(api.mutate).toHaveBeenCalledTimes(2);
    expect(screen.queryByText("C1")).toBeNull();
    first.unmount();

    api.isError = true;
    render(<QuickConnectScreen />);
    act(() => api.opts.onError());
    expect(screen.getByText("retry")).toBeTruthy();
    screen.getByText("retry").click();
    expect(api.mutate).toHaveBeenCalledTimes(4);
  });

  it("offers the scanner and the way back", () => {
    render(<QuickConnectScreen />);
    screen.getByRole("button", { name: "auth.settings.quickConnect.scanQr" }).click();
    expect(router.push).toHaveBeenCalledWith("/(auth)/qr-scan");
    screen.getByRole("button", { name: "auth.forgotPassword.backToLogin" }).click();
    expect(router.back).toHaveBeenCalledTimes(1);
  });
});
