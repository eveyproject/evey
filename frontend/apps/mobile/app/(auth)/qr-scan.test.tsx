import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// A scanned QR is reduced to the six-character code (from a ?code= URL or a
// raw token) and authorised ONCE - a second frame while the first is in
// flight is ignored; a refused code re-arms the scanner; success goes back.
// Without camera permission the screen asks for it instead of a viewfinder.
const alerts = vi.hoisted(() => ({ messages: [] as string[] }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
  Alert: { alert: (m: string) => { alerts.messages.push(m); } },
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));
const camera = vi.hoisted(() => ({
  permission: { granted: true } as { granted: boolean } | null,
  request: vi.fn(),
  onScan: null as null | ((e: { data: string }) => void),
}));
vi.mock("expo-camera", () => ({
  useCameraPermissions: () => [camera.permission, camera.request],
  CameraView: ({ onBarcodeScanned }: any) => {
    camera.onScan = onBarcodeScanned ?? null;
    return React.createElement("div", { "data-testid": "camera", "data-armed": String(!!onBarcodeScanned) });
  },
}));
const back = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, useRouter: () => ({ back }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const hook = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn(), isPending: false }));
vi.mock("@evey/api", () => ({
  useUserServiceAuthorizeQuickConnect: (o: any) => {
    hook.opts = o.mutation;
    return { mutate: hook.mutate, isPending: hook.isPending };
  },
}));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000" }, radii: { md: 8 } }),
}));

import QrScanScreen from "./qr-scan";

const scan = (data: string) => act(() => camera.onScan?.({ data }));

beforeEach(() => {
  vi.clearAllMocks();
  alerts.messages.length = 0;
  camera.permission = { granted: true };
  camera.onScan = null;
  hook.isPending = false;
});

describe("qr scan screen (mobile)", () => {
  it("reduces a scanned URL or token to the six-character code and authorises it once", () => {
    render(<QrScanScreen />);
    expect(screen.getByTestId("camera").getAttribute("data-armed")).toBe("true");
    scan("https://evey.local/quickconnect?code=full-token-abc123");
    expect(hook.mutate).toHaveBeenCalledWith({ data: { code: "ABC123" } });
    expect(screen.getByTestId("camera").getAttribute("data-armed")).toBe("false");
    scan("another");
    expect(hook.mutate).toHaveBeenCalledTimes(1);

    act(() => hook.opts.onSuccess());
    expect(back).toHaveBeenCalledTimes(1);
    expect(alerts.messages).toEqual(["auth.settings.quickConnect.authorized"]);
  });

  it("a raw short code is upper-cased; a refused code re-arms the scanner; retry re-arms it by hand", () => {
    render(<QrScanScreen />);
    scan("a1b2");
    expect(hook.mutate).toHaveBeenCalledWith({ data: { code: "A1B2" } });
    act(() => hook.opts.onError());
    expect(alerts.messages).toEqual(["auth.settings.quickConnect.invalidCode"]);
    expect(screen.getByTestId("camera").getAttribute("data-armed")).toBe("true");

    scan("x1y2z3");
    expect(screen.getByTestId("camera").getAttribute("data-armed")).toBe("false");
    fireEvent.click(screen.getByText("common.retry"));
    expect(screen.getByTestId("camera").getAttribute("data-armed")).toBe("true");
    scan("q1w2e3");
    expect(hook.mutate).toHaveBeenCalledTimes(3);
  });

  it("asks for the camera before showing a viewfinder, and waits while the permission is unknown", () => {
    camera.permission = null;
    const { unmount } = render(<QrScanScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByTestId("camera")).toBeNull();
    unmount();

    camera.permission = { granted: false };
    render(<QrScanScreen />);
    expect(screen.queryByTestId("camera")).toBeNull();
    fireEvent.click(screen.getByText("auth.settings.quickConnect.grantPermission"));
    expect(camera.request).toHaveBeenCalledTimes(1);
  });
});
