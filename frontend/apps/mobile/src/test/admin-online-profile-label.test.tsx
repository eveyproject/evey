// The "who's online" card on the admin dashboard shows a device icon and a
// username, but not WHICH profile of that account is watching. This locks in
// that the row resolves the profile name from the profile list, labels the
// household default profile "Default" instead of its (account-derived) name,
// and shows nothing extra for a profileId that resolves to nothing.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, numberOfLines, ...p }: any) =>
    React.createElement("span", { "data-numberoflines": numberOfLines, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  RefreshControl: () => null,
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Pressable: ({ children, onPress, ...p }: any) => React.createElement("button", { onClick: onPress, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1, absoluteFillObject: {} },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-native-svg", () => ({
  default: ({ children, ...p }: any) => React.createElement("svg", p, children),
  Circle: (p: any) => React.createElement("circle", p),
}));

vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => {
      if (key === "auth.profile.manage.defaultProfileBadge") return "Default";
      if (typeof opts === "string") return opts;
      return key;
    },
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", surfaceRaised: "#222", border: "#333", success: "#0f0", warning: "#ff0", danger: "#f00" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const onlineUsers = [
  { userId: "u1", username: "alice", profileId: "p-named", userAgent: "mobile" },
  { userId: "u2", username: "alice", profileId: "p-default", userAgent: "web" },
  { userId: "u3", username: "alice", profileId: "p-unknown", userAgent: "tv" },
];

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePresenceServiceListOnline: () => ({ data: { users: onlineUsers, total: onlineUsers.length }, isError: false, refetch: vi.fn() }),
  useUserServiceListAllProfiles: () => ({
    data: {
      profiles: [
        { id: "p-named", name: "Alice", isDefault: false },
        { id: "p-default", name: "alice", isDefault: true },
      ],
    },
  }),
  useAdminSystemServiceGetDashboard: () => ({ data: undefined, refetch: vi.fn() }),
  useAdminSystemServiceGetSystemInfo: () => ({ data: undefined, refetch: vi.fn() }),
  useAdminSystemServiceRestartServer: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminSessionServiceListActiveSessions: () => ({ data: { sessions: [] }, isLoading: false, isError: false, refetch: vi.fn() }),
  useAdminSessionServiceGetAuditLog: () => ({ data: { entries: [] } }),
  useActivityServiceGetServerStats: () => ({ data: undefined }),
  useActivityServiceGetAnalytics: () => ({ data: undefined }),
  useLibraryServiceListMediaItems: () => ({ data: { pagination: { totalItems: 0 } }, refetch: vi.fn() }),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [] } }),
  useLibraryServiceScanLibrary: () => ({ mutate: vi.fn(), mutateAsync: vi.fn() }),
  formatBytes: (n: unknown) => `${n ?? 0}B`,
}));

import AdminDashboard from "../../app/(auth)/admin/index";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("admin dashboard - who's online profile label", () => {
  it("shows the profile name for a named profile", () => {
    render(<AdminDashboard />);
    expect(screen.getByText("Alice")).toBeTruthy();
  });

  it("shows 'Default' for the household default profile, not its account-derived name", () => {
    render(<AdminDashboard />);
    expect(screen.getByText("Default")).toBeTruthy();
    // "alice" is both the username (shown for every row) and the default
    // profile's stored name - only the username occurrences should exist,
    // never a second "alice" line for the default profile row.
    expect(screen.getAllByText("alice").length).toBe(onlineUsers.length);
  });

  it("shows nothing extra when the profileId resolves to no known profile", () => {
    render(<AdminDashboard />);
    // Row for u3 has profileId "p-unknown", absent from the profile map: no
    // label span beyond the username/lastSeen text is rendered for it.
    expect(screen.queryByText("p-unknown")).toBeNull();
  });
});
