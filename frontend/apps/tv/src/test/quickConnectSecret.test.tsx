import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import React from "react";

// The code is on the screen for the whole room; the secret only ever reaches
// this device. The screen declares it will send it back, and does so on every
// poll, so a phone that scanned the QR cannot collect the tokens in its place.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, testID, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "data-testid": testID, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
}));
vi.mock("react-native-qrcode-svg", () => ({
  default: () => React.createElement("div", { "data-testid": "qr" }),
}));
vi.mock("expo-router", () => ({ router: { back: vi.fn(), replace: vi.fn(), canDismiss: () => false, dismissAll: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#999", accent: "#f00" },
    radii: { md: 8 },
  }),
}));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrlNullable: () => "http://srv.test" }));
vi.mock("../lib/refresh-token", () => ({ setRefreshToken: vi.fn(async () => undefined) }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { setTokens: () => void }) => unknown) => sel({ setTokens: vi.fn() }),
}));

const checkMock = vi.fn();
const initiateArgs: unknown[] = [];
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: ({ mutation }: any) => ({
    mutate: (vars?: unknown) => {
      initiateArgs.push(vars);
      mutation.onSuccess({ code: "qc-code-abc123", secret: "qc-secret-1" });
    },
    isError: false,
  }),
  userServiceCheckQuickConnect: (...a: unknown[]) => checkMock(...a),
}));

import Screen from "../../app/quick-connect";

beforeEach(() => {
  checkMock.mockReset().mockResolvedValue({ authorized: false });
  initiateArgs.length = 0;
});

describe("TV quick connect secret", () => {
  it("declares itself secret-aware and polls with the secret it was handed", async () => {
    render(<Screen />);
    await waitFor(() => expect(checkMock).toHaveBeenCalled());
    expect(initiateArgs[0]).toEqual({ params: { secretAware: true } });
    expect(checkMock).toHaveBeenCalledWith("qc-code-abc123", { secret: "qc-secret-1" });
  });
});
