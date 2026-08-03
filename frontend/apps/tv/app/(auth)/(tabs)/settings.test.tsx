import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops unknown boolean props on DOM elements, so the focus test needs
  // something it can actually query for.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
const replaceMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args), replace: (...args: unknown[]) => replaceMock(...args) },
}));

// --- react-i18next mock --------------------------------------------------------
const changeLanguageMock = vi.fn();
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
    i18n: { language: "en", changeLanguage: (...args: unknown[]) => changeLanguageMock(...args) },
  }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- @evey/store mock ---------------------------------------------------------
const logoutMock = vi.fn();
const setThemeMock = vi.fn();
const setLocaleMock = vi.fn();
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (sel: any) => sel({ logout: logoutMock, setTokens: vi.fn(), profileId: "prof-2" }),
    { getState: () => ({ setTokens: vi.fn() }) },
  ),
  // applyFromPreferences included: the screen adopts the theme stored on the
  // profile, so the store mock has to carry it or the mount effect throws.
  useThemeStore: (sel: any) => sel({ theme: "default", setTheme: setThemeMock, applyFromPreferences: () => {} }),
  themePreferences: (current: Record<string, string> | undefined, theme: string) => ({ ...current, theme }),
  useLocaleStore: (sel: any) => sel({ locale: "en", setLocale: setLocaleMock }),
}));

// --- hooks mocks ---------------------------------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrlNullable: () => "http://srv.test",
}));

// --- server-config mock ---------------------------------------------------------
vi.mock("../../../src/lib/server-config", () => ({
  clearServerUrl: vi.fn(),
}));

// --- i18n mock -------------------------------------------------------------------
vi.mock("../../../src/i18n", () => ({
  supportedLanguages: [
    { code: "en", label: "English" },
    { code: "fr", label: "Français" },
  ],
}));

// --- @evey/api mock --------------------------------------------------------------
const currentUserMock = vi.fn();
const listProfilesMock = vi.fn();
const updateProfileMock = vi.fn();
const updateProfileMutateMock = vi.fn();
const updateProfileMutateAsyncMock = vi.fn();
const systemInfoMock = vi.fn();
const userServiceLogoutMock = vi.fn();
const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => currentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
  useUserServiceUpdateProfile: (...args: unknown[]) => updateProfileMock(...args),
  useAdminSystemServiceGetSystemInfo: (...args: unknown[]) => systemInfoMock(...args),
  userServiceLogout: (...args: unknown[]) => userServiceLogoutMock(...args),
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

import SettingsScreen from "./settings";

beforeEach(() => {
  vi.clearAllMocks();
  currentUserMock.mockReturnValue({ data: { id: "user-1", username: "gravity" } });
  listProfilesMock.mockReturnValue({
    data: {
      profiles: [
        { id: "prof-1", name: "Alice", isDefault: true },
        { id: "prof-2", name: "Chloé" },
      ],
    },
  });
  updateProfileMock.mockReturnValue({
    mutate: updateProfileMutateMock,
    mutateAsync: updateProfileMutateAsyncMock,
    isPending: false,
  });
  systemInfoMock.mockReturnValue({ data: { version: "1.0.0" } });
  userServiceLogoutMock.mockResolvedValue({});
  eveyFetchMock.mockResolvedValue({});
});

describe("SettingsScreen (TV) playback entry", () => {
  it("renders a Playback row", () => {
    render(<SettingsScreen />);
    expect(screen.getByLabelText("auth.settings.tabs.playback")).toBeTruthy();
  });

  it("navigates to the playback settings screen on press", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("auth.settings.tabs.playback"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/settings/playback");
  });
});

describe("SettingsScreen (TV) sign out revokes the server session", () => {
  it("calls the logout RPC so the refresh token is revoked server-side, not just cleared locally", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(userServiceLogoutMock).toHaveBeenCalledWith({});
  });

  it("clears local session state and navigates away immediately, without waiting on the network call", () => {
    userServiceLogoutMock.mockReturnValue(new Promise(() => {})); // never resolves
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(logoutMock).toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/login");
  });

  it("still leaves the local session cleared when the offline TV can't reach the server", async () => {
    userServiceLogoutMock.mockRejectedValue(Object.assign(new Error("network down"), { httpStatus: undefined }));
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(logoutMock).toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/login");
    // Let the rejected promise settle - it must be swallowed, not thrown as
    // an unhandled rejection that would fail this test.
    await new Promise((r) => setTimeout(r, 0));
  });
});

// The owner asked (2026-08-02) for the profile menu to match web/mobile. Three
// entries were missing outright and the screen never said WHOSE profile was in
// use - on a shared television that is the setting that decides what the home
// shows and what the parental gate allows.
describe("SettingsScreen (TV) profile menu parity", () => {
  it("names the profile actually in use, not merely the account", () => {
    render(<SettingsScreen />);
    expect(screen.getByText("Chloé")).toBeTruthy();
    expect(screen.getByText("gravity")).toBeTruthy();
  });

  it("offers my statistics, and opens the screen", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("My statistics"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/settings/stats");
  });

  // The syncplay screen already existed on this TV with no way in.
  it("offers joining a watch-together room, and opens the screen", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("Join a room"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/syncplay");
  });

  it("still lets the viewer switch profile and sign out", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.switchProfile"));
    expect(pushMock).toHaveBeenCalledWith("/profile-select");
  });
});

// Regression for the fix landed the night of 2026-08-02/03: entering Settings
// used to hand initial D-pad focus to Sign out, one reflex OK press away from
// a session a keyboard-less television has no comfortable way to restore.
describe("SettingsScreen (TV) initial focus", () => {
  it("claims initial TV focus on 'change server', not on sign out", () => {
    render(<SettingsScreen />);
    const focused = document.querySelector('[data-tv-preferred-focus="true"]');
    expect(focused).toBe(screen.getByLabelText("mobile.settings.changeServer"));
  });

  it("never leaves sign out as the preferred focus target", () => {
    render(<SettingsScreen />);
    expect(screen.getByLabelText("mobile.settings.signOut").getAttribute("data-tv-preferred-focus")).not.toBe("true");
  });
});

describe("SettingsScreen (TV) server info", () => {
  it("shows the URL of the server this television is paired with", () => {
    render(<SettingsScreen />);
    expect(screen.getByText("http://srv.test")).toBeTruthy();
  });
});

describe("SettingsScreen (TV) language change", () => {
  it("applies the choice in-app and persists it on the ACTIVE profile, not the default one", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("Français"));
    expect(changeLanguageMock).toHaveBeenCalledWith("fr");
    // The auth store mock's profileId is "prof-2" (Chloé), not the default
    // "prof-1" (Romain) - see handleLanguageChange's comment in settings.tsx:
    // writing the locale onto the default profile instead of the active one
    // was the bug this screen fixed.
    expect(updateProfileMutateAsyncMock).toHaveBeenCalledWith({ id: "prof-2", data: { locale: "fr" } });
  });
});
