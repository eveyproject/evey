import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Modal: ({ children, visible, onRequestClose }: any) =>
    visible ? React.createElement("div", { "data-testid": "modal", onKeyUp: onRequestClose }, children) : null,
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Animated: {
    View: ({ children, accessibilityViewIsModal, onAccessibilityEscape, ...p }: any) =>
      React.createElement("div", { ...p, "data-testid": "drawer-panel", "data-modal": String(accessibilityViewIsModal), onKeyDown: onAccessibilityEscape }, children),
  },
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
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

let mockIsAdmin = true;
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: mockIsAdmin, accessToken: "tok" }),
}));

vi.mock("@evey/api", () => ({
  useMediaRequestServiceGetRequestPolicy: () => ({ data: { canRequest: true } }),
}));

vi.mock("@evey/theme", () => ({
  FONT_SIZE: { sm: 14 },
  ICON_SIZE: { lg: 22 },
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", border: "#333", overlay: "#0008",
    },
  }),
}));

const closeMock = vi.fn();
const dismissMock = vi.fn();
vi.mock("../providers/DrawerProvider", () => ({
  useDrawer: () => ({ isOpen: true, close: closeMock, dismiss: dismissMock, toggle: vi.fn(), translateX: 0 }),
}));

let mockPluginEntries: any[] = [];
vi.mock("../features/plugins/PluginSlot", () => ({
  PluginSlot: () => null,
  usePluginSlotEntries: () => mockPluginEntries,
}));

vi.mock("./EveyLogo", () => ({
  EveyWordmark: () => React.createElement("span", { "data-testid": "wordmark" }),
}));

import { SideDrawer } from "./SideDrawer";

beforeEach(() => {
  vi.clearAllMocks();
  mockPathname = "/";
  mockIsAdmin = true;
  mockPluginEntries = [];
});

describe("SideDrawer - main vs admin tray", () => {
  it("shows the main nav (with the admin entry) outside /admin", () => {
    render(<SideDrawer />);
    expect(screen.getByText("common.nav.home")).toBeTruthy();
    expect(screen.getByText("common.nav.admin")).toBeTruthy();
    expect(screen.queryByText("common.nav.backToApp")).toBeNull();
    expect(screen.queryByText("admin.nav.users")).toBeNull();
  });

  it("hides the admin entry for non-admin users", () => {
    mockIsAdmin = false;
    render(<SideDrawer />);
    expect(screen.queryByText("common.nav.admin")).toBeNull();
  });

  it("swaps to the admin sections and back-to-app inside /admin", () => {
    mockPathname = "/admin/users";
    render(<SideDrawer />);
    expect(screen.getByText("common.nav.backToApp")).toBeTruthy();
    expect(screen.getByText("admin.nav.users")).toBeTruthy();
    expect(screen.getByText("admin.nav.logs")).toBeTruthy();
    expect(screen.getByText("ADMIN.NAV.USERSACCESS")).toBeTruthy();
    // The main nav is replaced, not stacked above the sections.
    expect(screen.queryByText("common.nav.home")).toBeNull();
  });

  it("back-to-app navigates to the home stack", async () => {
    mockPathname = "/admin/logs";
    render(<SideDrawer />);
    fireEvent.click(screen.getByText("common.nav.backToApp").closest("button")!);
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/(auth)/(tabs)/"));
  });

  it("lists admin.nav plugin extensions in the admin tray", () => {
    mockPathname = "/admin";
    mockPluginEntries = [{ pluginId: "plg-librarian", name: "Librarian", entryPath: "", externalFetch: [] }];
    render(<SideDrawer />);
    fireEvent.click(screen.getByText("Librarian").closest("button")!);
    return waitFor(() => expect(pushMock).toHaveBeenCalledWith("/admin/extensions/plg-librarian"));
  });
});

// The drawer is a modal: the screen reader stays inside it, and every way out
// that is not a navigation (Escape or Back via onRequestClose, the backdrop,
// VoiceOver's escape gesture) is a dismiss, which hands the focus back to the
// rail control that opened it.
describe("SideDrawer - modal accessibility", () => {
  it("keeps the screen reader inside the drawer panel", () => {
    render(<SideDrawer />);
    expect(screen.getByTestId("drawer-panel").getAttribute("data-modal")).toBe("true");
  });

  it("dismisses on Escape/Back, on the backdrop and on the screen reader escape gesture", () => {
    render(<SideDrawer />);
    fireEvent.keyUp(screen.getByTestId("modal"));
    fireEvent.click(screen.getByLabelText("Close"));
    fireEvent.keyDown(screen.getByTestId("drawer-panel"));
    expect(dismissMock).toHaveBeenCalledTimes(3);
    expect(closeMock).not.toHaveBeenCalled();
  });

  it("a navigation closes without dismissing, so the focus goes to the new screen", async () => {
    render(<SideDrawer />);
    fireEvent.click(screen.getByText("common.nav.home").closest("button")!);
    expect(closeMock).toHaveBeenCalledTimes(1);
    expect(dismissMock).not.toHaveBeenCalled();
    await waitFor(() => expect(pushMock).toHaveBeenCalled());
  });
});
