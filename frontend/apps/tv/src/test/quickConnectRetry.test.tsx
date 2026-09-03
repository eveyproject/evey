import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, act } from "@testing-library/react";
import React from "react";

// The code request runs on mount, so a failure leaves no gesture to repeat -
// and a remote cannot reload the screen. Without a way out the spinner spun for
// ever (audited 2026-08-04).

const initiateMutate = vi.fn();
let isError = false;

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p },
      children,
    ),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-native-qrcode-svg", () => ({
  default: () => React.createElement("div", { "data-testid": "qr" }),
}));

// Shape copied from the real hook: the screen reads isError to choose between
// the spinner and the way out.
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: () => ({ mutate: initiateMutate, isError }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { setTokens: () => void }) => unknown) => sel({ setTokens: vi.fn() }),
}));

vi.mock("expo-router", () => ({ router: { back: vi.fn(), replace: vi.fn() } }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#999", accent: "#f00" },
    radii: { md: 8 },
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

beforeEach(() => {
  initiateMutate.mockClear();
  isError = false;
});

describe("TV quick connect", () => {
  it("offers a way out when the code request failed", async () => {
    isError = true;
    const Screen = (await import("../../app/quick-connect")).default;
    // The mount effect requests a code and starts timers: flush inside act.
    await act(async () => {
      render(<Screen />);
    });

    expect(screen.queryByTestId("spinner")).toBeNull();
    const retry = screen.getByTestId("quick-connect-retry");
    initiateMutate.mockClear();
    act(() => { retry.click(); });
    expect(initiateMutate).toHaveBeenCalledTimes(1);
  });

  it("shows the spinner while the code is still coming", async () => {
    isError = false;
    const Screen = (await import("../../app/quick-connect")).default;
    // The mount effect requests a code and starts timers: flush inside act.
    await act(async () => {
      render(<Screen />);
    });

    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByTestId("quick-connect-retry")).toBeNull();
  });
});
