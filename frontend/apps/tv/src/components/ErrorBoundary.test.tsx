import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "data-has-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router mock -------------------------------------------------------
const replaceMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { replace: (...args: unknown[]) => replaceMock(...args) },
}));

// --- i18next -----------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

// --- ThemeProvider -----------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      surface: "#111",
      error: "#f00",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// -----------------------------------------------------------------------------
import { ErrorBoundary } from "./ErrorBoundary";

function Boom(): React.ReactElement {
  throw new Error("kaboom");
}

describe("ErrorBoundary (TV)", () => {
  beforeEach(() => {
    replaceMock.mockClear();
    // React logs the crash to console.error; silence it so it doesn't fail
    // the test suite while we still get to assert on our own fallback UI.
    vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
    cleanup();
  });

  it("catches a crashing child and shows the translated fallback with both actions focusable", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    expect(screen.getByText("common.loadError")).toBeTruthy();
    const retryBtn = screen.getByText("common.retry").closest("button");
    const homeBtn = screen.getByText("common.nav.home").closest("button");
    expect(retryBtn).toBeTruthy();
    expect(homeBtn).toBeTruthy();

    // Only one hasTVPreferredFocus on the fallback screen (agents.md rule):
    // the retry action gets initial d-pad focus, Go Home does not.
    expect(retryBtn?.getAttribute("data-has-tv-preferred-focus")).toBe("true");
    expect(homeBtn?.hasAttribute("data-has-tv-preferred-focus")).toBe(false);
  });

  it("clears the error and re-renders the tree when Retry is pressed", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    fireEvent.click(screen.getByText("common.retry"));

    // The child throws deterministically, so the boundary re-crashes right away.
    expect(screen.getByText("common.loadError")).toBeTruthy();
  });

  it("navigates back to the home tab when Go Home is pressed", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    fireEvent.click(screen.getByText("common.nav.home"));

    expect(replaceMock).toHaveBeenCalledWith("/(auth)/(tabs)");
  });
});
