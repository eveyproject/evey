import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeSelector } from "./ThemeSelector";

const mockSetTheme = vi.fn();
let mockCurrentTheme = "default";

// Selector-aware: the component reads the store both as a whole object and
// through a selector (`s => s.applyFromPreferences`), so a mock that ignores
// the selector hands back undefined and the effect throws.
vi.mock("@evey/store", () => {
  // Built on every call, never captured: vi.mock factories are hoisted above
  // the `let mockCurrentTheme` below, so reading it at factory time throws.
  const noop = vi.fn();
  const themeState = () => ({
    theme: mockCurrentTheme,
    setTheme: mockSetTheme,
    applyFromPreferences: noop,
  });
  return {
    useThemeStore: (selector?: (s: ReturnType<typeof themeState>) => unknown) =>
      selector ? selector(themeState()) : themeState(),
    useAuthStore: (selector?: (s: { profileId: string | null }) => unknown) =>
      selector ? selector({ profileId: "p-1" }) : { profileId: "p-1" },
    themePreferences: (current: Record<string, string> | undefined, theme: string) => ({ ...current, theme }),
  };
});

vi.mock("@evey/theme", () => ({
  themes: {
    default: {},
    amoled: {},
    "light-default": {},
    "light-blue": {},
  },
}));

// The selector now reads the active profile so the theme follows the viewer
// across clients; these stubs keep the unit test about the pills themselves.
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn().mockResolvedValue({ themes: [] }),
  useUserServiceGetCurrentUser: () => ({ data: { id: "u-1" } }),
  useUserServiceListProfiles: () => ({
    data: { profiles: [{ id: "p-1", isDefault: true, preferences: {} }] },
  }),
  useUserServiceUpdateProfile: () => ({ mutate: vi.fn() }),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

describe("ThemeSelector", () => {
  beforeEach(() => {
    mockSetTheme.mockReset();
    mockCurrentTheme = "default";
  });

  it("renders theme options", () => {
    render(<ThemeSelector />, { wrapper: createWrapper() });
    expect(screen.getByText("Dark")).toBeInTheDocument();
    expect(screen.getByText("AMOLED")).toBeInTheDocument();
    expect(screen.getByText("Light")).toBeInTheDocument();
    expect(screen.getByText("Light Blue")).toBeInTheDocument();
  });

  it("changes theme on select", async () => {
    const user = userEvent.setup();
    render(<ThemeSelector />, { wrapper: createWrapper() });
    await user.click(screen.getByText("AMOLED"));
    expect(mockSetTheme).toHaveBeenCalledWith("amoled");
  });

  it("highlights active theme", () => {
    mockCurrentTheme = "amoled";
    render(<ThemeSelector />, { wrapper: createWrapper() });
    const amoledBtn = screen.getByText("AMOLED");
    expect(amoledBtn.className).toMatch(/bg-\[var\(--accent\)\]/);
  });
});
