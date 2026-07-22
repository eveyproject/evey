import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import React from "react";

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

// --- Expo vector icons ------------------------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", error: "#f00" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- query-client -----------------------------------------------------------------------
const clearMock = vi.fn();
vi.mock("../lib/query-client", () => ({
  queryClient: { clear: (...args: unknown[]) => clearMock(...args) },
}));

// -------------------------------------------------------------------------------------------
import { ErrorBoundary } from "./ErrorBoundary";

function Boom(): React.ReactElement {
  throw new Error("kaboom");
}

describe("ErrorBoundary", () => {
  let consoleErrorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    clearMock.mockClear();
    // React itself also logs the crash to console.error; silence it here so
    // it doesn't fail the test suite, while still letting us assert on our
    // own explicit componentDidCatch log below.
    consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
  });

  afterEach(() => {
    consoleErrorSpy.mockRestore();
    cleanup();
  });

  it("catches a crashing child, logs the error and component stack, and shows the fallback", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Something went wrong")).toBeTruthy();

    const loggedCall = consoleErrorSpy.mock.calls.find((call) => call[0] === "[ErrorBoundary] render crash:");
    expect(loggedCall).toBeDefined();
    const [, loggedError, loggedStack] = loggedCall as [string, Error, string];
    expect(loggedError).toBeInstanceOf(Error);
    expect(loggedError.message).toBe("kaboom");
    expect(typeof loggedStack).toBe("string");
    expect(loggedStack).toContain("Boom");
  });

  it("clears the query cache and re-renders the tree when Try Again is pressed", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    fireEvent.click(screen.getByText("Try Again"));

    expect(clearMock).toHaveBeenCalledTimes(1);
    // The child throws deterministically, so the boundary re-crashes right away.
    expect(screen.getByText("Something went wrong")).toBeTruthy();
  });

  it("stops offering Try Again after repeated failures and asks the user to restart", () => {
    render(
      <ErrorBoundary>
        <Boom />
      </ErrorBoundary>,
    );

    // 1st render already crashed once (attempts = 1). Two more retries push
    // attempts past MAX_RETRIES (2), at which point the button must disappear.
    fireEvent.click(screen.getByText("Try Again"));
    fireEvent.click(screen.getByText("Try Again"));

    expect(screen.queryByText("Try Again")).toBeNull();
    expect(screen.getByText("This keeps happening. Please restart the app.")).toBeTruthy();
  });
});
