import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { themes, type ThemeName } from "@evey/theme";
import { useThemeStore } from "@evey/store";
import { ThemeProvider, useTheme } from "./ThemeProvider";

// contrast.test.ts already proves the DEFINITIONS in @evey/theme carry a
// valid onAccent/onSuccess/onWarning/onDanger. It never renders the TV
// ThemeProvider though, so a regression where the provider narrows the type,
// destructures only a subset of `colors`, or falls back to a stale local
// ThemeTokens shape would slip through undetected - which is exactly how the
// 17 hardcoded "#fff" literals survived until now. This asserts the actual
// consumption path: store -> ThemeProvider -> useTheme().
const HEX = /^#[0-9a-fA-F]{3,8}$/;

// A query client because the provider can ask for the plugins' themes; a
// built-in choice never does, so nothing is fetched here.
const queryClient = new QueryClient();
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>{children}</ThemeProvider>
    </QueryClientProvider>
  );
}

describe("TV ThemeProvider - exposes the on-fill tokens for every shipped theme", () => {
  beforeEach(() => {
    useThemeStore.setState({ theme: "default", pluginId: null, customVars: null });
  });

  for (const name of Object.keys(themes) as ThemeName[]) {
    it(`${name}: useTheme() surfaces onAccent/onSuccess/onWarning/onDanger matching @evey/theme`, () => {
      useThemeStore.setState({ theme: name });
      const { result } = renderHook(() => useTheme(), { wrapper });

      for (const key of ["onAccent", "onSuccess", "onWarning", "onDanger"] as const) {
        expect(result.current.colors[key]).toMatch(HEX);
        expect(result.current.colors[key]).toBe(themes[name].colors[key]);
      }
    });
  }

  it("falls back to defaultTheme for an unknown theme name", () => {
    useThemeStore.setState({ theme: "not-a-real-theme" });
    const { result } = renderHook(() => useTheme(), { wrapper });
    expect(result.current.colors.onAccent).toBe(themes.default.colors.onAccent);
  });
});
