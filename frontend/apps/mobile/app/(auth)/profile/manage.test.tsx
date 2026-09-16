import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The list is asked only once the user is known; loading, failure (with a
// retry) and "no profiles" are three distinct states; Create opens the
// dialog, which is told whether a kids profile already exists so the
// household can only ever hold one.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({
  user: undefined as { id: string } | undefined,
  list: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  listArgs: null as any,
}));
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: api.user }),
  useUserServiceListProfiles: (id: string, o: any) => {
    api.listArgs = { id, enabled: o.query.enabled };
    return api.list;
  },
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000" }, radii: { md: 8 }, spacing: { md: 16, lg: 24, xl: 32 } }),
}));
vi.mock("../../../src/features/profile/ProfileManagerList", () => ({
  ProfileManagerList: ({ userId, profiles }: any) => React.createElement("div", { "data-testid": "list", "data-user": userId }, String(profiles.length)),
}));
vi.mock("../../../src/features/profile/CreateProfileDialog", () => ({
  CreateProfileDialog: ({ open, onClose, hasKidsProfile }: any) =>
    React.createElement("div", { "data-testid": "dialog", "data-open": String(open), "data-kids": String(hasKidsProfile) }, React.createElement("button", { onClick: onClose }, "close")),
}));
vi.mock("../../../src/components/LoadFailed", () => ({
  LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry"),
}));

import ProfileManageScreen from "./manage";

beforeEach(() => {
  vi.clearAllMocks();
  api.user = { id: "u1" };
  api.list = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("profile manage screen (mobile)", () => {
  it("lists the profiles and opens the create dialog knowing a kids profile exists", () => {
    api.list.data = { profiles: [{ id: "p1", isDefault: true }, { id: "p2", isKids: true }] };
    render(<ProfileManageScreen />);
    expect(api.listArgs).toEqual({ id: "u1", enabled: true });
    expect(screen.getByTestId("list").textContent).toBe("2");
    expect(screen.getByTestId("list").getAttribute("data-user")).toBe("u1");
    const dialog = screen.getByTestId("dialog");
    expect(dialog.getAttribute("data-open")).toBe("false");
    expect(dialog.getAttribute("data-kids")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Create Profile" }));
    expect(screen.getByTestId("dialog").getAttribute("data-open")).toBe("true");
    fireEvent.click(screen.getByText("close"));
    expect(screen.getByTestId("dialog").getAttribute("data-open")).toBe("false");
  });

  it("asks nothing before the user is known and mounts no dialog", () => {
    api.user = undefined;
    render(<ProfileManageScreen />);
    expect(api.listArgs).toEqual({ id: "", enabled: false });
    expect(screen.queryByTestId("dialog")).toBeNull();
    expect(screen.getByText("No profiles yet.")).toBeTruthy();
  });

  it("keeps loading, failure and empty apart", () => {
    api.list.isLoading = true;
    const { unmount } = render(<ProfileManageScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    expect(screen.queryByText("No profiles yet.")).toBeNull();
    unmount();

    api.list.isLoading = false;
    api.list.isError = true;
    render(<ProfileManageScreen />);
    fireEvent.click(screen.getByText("retry"));
    expect(api.list.refetch).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("No profiles yet.")).toBeNull();
    expect(screen.queryByTestId("list")).toBeNull();
    expect(screen.getByTestId("dialog").getAttribute("data-kids")).toBe("false");
  });
});
