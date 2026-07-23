import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
const replaceMock = vi.fn();
vi.mock("expo-router", () => ({
  router: { push: (...args: unknown[]) => pushMock(...args), replace: (...args: unknown[]) => replaceMock(...args) },
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => fallback ?? key,
    i18n: { language: "en", changeLanguage: vi.fn() },
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
  useAuthStore: (sel: any) => sel({ logout: logoutMock, setTokens: vi.fn() }),
  useThemeStore: (sel: any) => sel({ theme: "default", setTheme: setThemeMock }),
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
  supportedLanguages: [{ code: "en", label: "English" }],
}));

// --- @evey/api mock --------------------------------------------------------------
const currentUserMock = vi.fn();
const listProfilesMock = vi.fn();
const updateProfileMock = vi.fn();
const systemInfoMock = vi.fn();
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: (...args: unknown[]) => currentUserMock(...args),
  useUserServiceListProfiles: (...args: unknown[]) => listProfilesMock(...args),
  useUserServiceUpdateProfile: (...args: unknown[]) => updateProfileMock(...args),
  useAdminSystemServiceGetSystemInfo: (...args: unknown[]) => systemInfoMock(...args),
  eveyFetch: vi.fn(),
}));

import SettingsScreen from "./settings";

beforeEach(() => {
  vi.clearAllMocks();
  currentUserMock.mockReturnValue({ data: { id: "user-1", username: "gravity" } });
  listProfilesMock.mockReturnValue({ data: { profiles: [{ id: "prof-1", isDefault: true }] } });
  updateProfileMock.mockReturnValue({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false });
  systemInfoMock.mockReturnValue({ data: { version: "1.0.0" } });
});

describe("SettingsScreen (TV) playback entry", () => {
  it("renders a Playback row", () => {
    render(<SettingsScreen />);
    expect(screen.getByLabelText("auth.settings.playback.bitrateTitle")).toBeTruthy();
  });

  it("navigates to the playback settings screen on press", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("auth.settings.playback.bitrateTitle"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/settings/playback");
  });
});
