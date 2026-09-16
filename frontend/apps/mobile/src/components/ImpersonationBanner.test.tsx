import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", { "data-testid": "banner" }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  TouchableOpacity: ({ children, onPress, accessibilityRole }: any) => React.createElement("button", { onClick: onPress, role: accessibilityRole }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: { name?: string }) => (o?.name ? `${k}:${o.name}` : k) }),
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { warning: "#fa0", text: "#fff" } }) }));
const auth = vi.hoisted(() => ({
  state: { impersonatedBy: undefined as string | undefined, impersonatedUsername: undefined as string | undefined, exitImpersonation: vi.fn() },
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: typeof auth.state) => unknown) => sel(auth.state) }));
const clear = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ clear }) }));

import { ImpersonationBanner } from "./ImpersonationBanner";

beforeEach(() => {
  vi.clearAllMocks();
  auth.state.impersonatedBy = undefined;
  auth.state.impersonatedUsername = undefined;
});

// The banner is the only way back from "connect as": it exists solely while
// an admin is impersonating, names the borrowed account when known, and
// leaving restores the admin and drops every response fetched as that user.
describe("ImpersonationBanner", () => {
  it("renders nothing outside impersonation", () => {
    render(<ImpersonationBanner />);
    expect(screen.queryByTestId("banner")).toBeNull();
  });

  it("names the impersonated account and falls back to a generic label", () => {
    auth.state.impersonatedBy = "admin-1";
    auth.state.impersonatedUsername = "bob";
    const { unmount } = render(<ImpersonationBanner />);
    expect(screen.getByText("admin.users.impersonationBanner:bob")).toBeTruthy();
    unmount();

    auth.state.impersonatedUsername = undefined;
    render(<ImpersonationBanner />);
    expect(screen.getByText("admin.users.impersonationBannerGeneric")).toBeTruthy();
  });

  it("leaving restores the admin and clears every cached response", () => {
    auth.state.impersonatedBy = "admin-1";
    render(<ImpersonationBanner />);
    fireEvent.click(screen.getByRole("button"));
    expect(auth.state.exitImpersonation).toHaveBeenCalledTimes(1);
    expect(clear).toHaveBeenCalledTimes(1);
  });
});
