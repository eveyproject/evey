import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, o?: { version?: string }) => (o?.version ? `${key}:${o.version}` : key) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", accent: "#f00", text: "#fff", textMuted: "#888", bg: "#000", onAccent: "#fff" }, radii: { md: 8 } }),
}));
vi.mock("@evey/api", () => ({
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 0 } }),
  useUserServiceGetCurrentUser: () => ({ data: undefined }),
  useUserServiceListProfiles: () => ({ data: undefined }),
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: any) => sel({ accessToken: null, profileId: null }) }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrlNullable: () => "http://srv" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
const update = vi.fn();
vi.mock("../hooks/useAppUpdate", () => ({ useAppUpdate: () => update() }));
const openStorePage = vi.fn();
vi.mock("../lib/appUpdate", () => ({ openStorePage: () => openStorePage() }));

import { TopHeader } from "./TopHeader";

const props = { onSearch: vi.fn(), onNotifications: vi.fn(), onProfile: vi.fn() };

beforeEach(() => { update.mockReset(); openStorePage.mockReset(); });

describe("TopHeader app update", () => {
  it("shows nothing while this build is the latest", () => {
    update.mockReturnValue(null);
    render(<TopHeader {...props} />);
    expect(screen.queryByText(/mobile.appUpdate.available/)).toBeNull();
  });

  it("names the newer build and opens its store page on OK", () => {
    update.mockReturnValue({ version: "0.1.12", versionCode: 57 });
    render(<TopHeader {...props} />);
    fireEvent.click(screen.getByText("mobile.appUpdate.available:0.1.12"));
    expect(openStorePage).toHaveBeenCalled();
  });
});
