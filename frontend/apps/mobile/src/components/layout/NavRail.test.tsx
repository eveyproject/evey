import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

const pushMock = vi.fn();
let mockPathname = "/";
vi.mock("expo-router", () => ({
  router: { push: (...a: unknown[]) => pushMock(...a) },
  usePathname: () => mockPathname,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("react-native-safe-area-context", () => ({
  useSafeAreaInsets: () => ({ top: 0, bottom: 0, left: 0, right: 0 }),
}));

let mockIsAdmin = true;
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: mockIsAdmin, accessToken: "tok" }),
}));

vi.mock("@evey/api", () => ({
  useMediaRequestServiceGetRequestPolicy: () => ({ data: { canRequest: true } }),
}));

vi.mock("@evey/theme", () => ({
  ICON_SIZE: { lg: 22 },
  FONT_SIZE: { sm: 14 },
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", border: "#333",
    },
  }),
}));

const openMock = vi.fn();
vi.mock("../../providers/DrawerProvider", () => ({
  useDrawer: () => ({ isOpen: false, open: openMock, close: vi.fn(), toggle: vi.fn(), translateX: 0 }),
}));

vi.mock("../../features/plugins/PluginSlot", () => ({
  usePluginSlotEntries: () => [],
}));

vi.mock("../EveyLogo", () => ({
  EveyMark: () => React.createElement("span", { "data-testid": "mark" }),
}));

import { NavRail } from "./NavRail";

beforeEach(() => {
  vi.clearAllMocks();
  mockPathname = "/";
  mockIsAdmin = true;
});

describe("NavRail", () => {
  it("main mode shows the nav icons plus the admin gear, and navigates on tap", () => {
    render(<NavRail mode="main" />);
    fireEvent.click(screen.getByLabelText("common.nav.libraries"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/(tabs)/library");
    expect(screen.getByLabelText("common.nav.admin")).toBeTruthy();
  });

  it("hides the admin gear for non-admins", () => {
    mockIsAdmin = false;
    render(<NavRail mode="main" />);
    expect(screen.queryByLabelText("common.nav.admin")).toBeNull();
  });

  it("admin mode flattens the admin sections and offers back-to-app", () => {
    mockPathname = "/admin/users";
    render(<NavRail mode="admin" />);
    expect(screen.getByLabelText("admin.nav.users")).toBeTruthy();
    expect(screen.getByLabelText("admin.nav.backupRestore")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("common.nav.backToApp"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/(tabs)/");
  });

  it("the brand mark expands the full drawer and names itself as the control to refocus", () => {
    render(<NavRail mode="main" />);
    fireEvent.click(screen.getByLabelText("Expand sidebar"));
    expect(openMock).toHaveBeenCalledWith(screen.getByLabelText("Expand sidebar"));
  });
});
