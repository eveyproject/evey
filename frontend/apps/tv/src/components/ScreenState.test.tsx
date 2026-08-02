import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

const backMock = vi.fn();
vi.mock("expo-router", () => ({ router: { back: (...a: unknown[]) => backMock(...a) } }));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", surface: "#111" },
    radii: { sm: 4, md: 8 },
  }),
}));

import { ScreenState } from "./ScreenState";

describe("ScreenState", () => {
  it("shows a spinner while loading", () => {
    render(<ScreenState loading />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });

  it("renders nothing when there is neither a wait nor a failure", () => {
    const { container } = render(<ScreenState />);
    expect(container.innerHTML).toBe("");
  });

  // The bug this component exists for: a failed request left isLoading false
  // and the data undefined, and the screen's `if (isLoading || !data)` spun
  // forever. On a television that reads as a frozen app.
  it("says the request failed instead of spinning forever", () => {
    render(<ScreenState error />);
    expect(screen.queryByTestId("spinner")).toBeNull();
    expect(screen.getByText("Something went wrong")).toBeTruthy();
  });

  // House rule: no state may leave the remote without somewhere to go.
  it("always leaves a focusable target, retry or back", () => {
    const onRetry = vi.fn();
    const { unmount } = render(<ScreenState error onRetry={onRetry} />);
    expect(document.querySelector('[data-tv-preferred-focus="true"]')).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Retry"));
    expect(onRetry).toHaveBeenCalled();
    unmount();

    // Without a retry the Back button takes the initial focus, so the screen
    // is never a dead end.
    render(<ScreenState error />);
    expect(screen.queryByLabelText("Retry")).toBeNull();
    const focused = document.querySelector('[data-tv-preferred-focus="true"]');
    expect(focused?.getAttribute("aria-label")).toBe("Back");
    fireEvent.click(screen.getByLabelText("Back"));
    expect(backMock).toHaveBeenCalled();
  });

  it("prefers a caller-supplied explanation over the generic one", () => {
    render(<ScreenState error message="Le serveur redémarre" />);
    expect(screen.getByText("Le serveur redémarre")).toBeTruthy();
    expect(screen.queryByText("Something went wrong")).toBeNull();
  });

  // Exactly one hasTVPreferredFocus per screen: two would make the initial
  // focus non-deterministic on a real box.
  it("declares a single preferred-focus target", () => {
    render(<ScreenState error onRetry={vi.fn()} />);
    expect(document.querySelectorAll('[data-tv-preferred-focus="true"]')).toHaveLength(1);
  });
});
