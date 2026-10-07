import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, waitFor, act } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  // The change-server confirmation renders inside a Modal.
  Modal: ({ children, visible }: any) => (visible ? children : null),
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
let authStoreState: any;
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (sel: any) => sel(authStoreState),
    { getState: () => authStoreState },
  ),
  // applyFromPreferences included: the screen adopts the theme stored on the
  // profile, so the store mock has to carry it or the mount effect throws.
  useThemeStore: (sel: any) => sel({ theme: "default", setTheme: setThemeMock, applyFromPreferences: () => {} }),
  themePreferences: (current: Record<string, string> | undefined, theme: string) => ({ ...current, theme }),
  useLocaleStore: (sel: any) => sel({ locale: "en", setLocale: setLocaleMock }),
  isDemoMode: () => false,
}));

// --- hooks mocks ---------------------------------------------------------------
// The themes the plugins ship: empty unless a test fills the list.
const pluginThemesRef = { current: [] as Array<{ id: string; name: string; tokens?: Record<string, string> }> };
vi.mock("../../../src/hooks/usePluginThemes", () => ({
  usePluginThemes: () => ({ data: pluginThemesRef.current }),
}));

vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrlNullable: () => "http://srv.test",
}));

vi.mock("expo-image", () => ({
  Image: (props: any) =>
    React.createElement("img", {
      alt: "",
      src: props.source?.uri,
      "data-headers": props.source?.headers ? JSON.stringify(props.source.headers) : undefined,
    }),
}));

const pluginEntries = { current: [] as { pluginId: string; name: string }[] };
vi.mock("../../../src/features/plugins/PluginSlot", () => ({
  usePluginSlotEntries: (slot: string) => (slot === "settings.user.appearance" ? pluginEntries.current : []),
}));

// --- server-config mock ---------------------------------------------------------
vi.mock("../../../src/lib/server-config", () => ({
  clearServerUrl: vi.fn(),
}));

// --- shared-TV teardown mocks ---------------------------------------------------
// Sign-out must purge the react-query cache (query keys carry no identity, so
// the next viewer would be served the previous account's rows), the on-disk
// refresh token, and the in-memory parental unlocks.
const queryClientClearMock = vi.fn();
vi.mock("../../../src/lib/query-client", () => ({
  queryClient: { clear: (...a: unknown[]) => queryClientClearMock(...a) },
}));
const clearRefreshTokenMock = vi.fn().mockResolvedValue(undefined);
const signOutDeviceMock = vi.fn();
vi.mock("../../../src/lib/sign-out", () => ({
  signOutDevice: () => signOutDeviceMock(),
}));

const forceRefreshMock = vi.fn(async () => ({ accessToken: "rotated-access-token" }));
vi.mock("../../../src/lib/auth-refresh", () => ({
  forceRefresh: () => forceRefreshMock(),
}));

const getUiScaleMock = vi.fn();
const setUiScaleMock = vi.fn();
vi.mock("@evey/media-caps", () => ({
  getUiScalePercent: () => getUiScaleMock(),
  setUiScalePercent: (p: number) => setUiScaleMock(p),
}));

vi.mock("../../../src/lib/refresh-token", () => ({
  clearRefreshToken: (...a: unknown[]) => clearRefreshTokenMock(...a),
}));
const clearUnlockedMock = vi.fn();
vi.mock("../../../src/hooks/useParentalGate", () => ({
  clearUnlocked: (...a: unknown[]) => clearUnlockedMock(...a),
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
  getUiScaleMock.mockReset();
  getUiScaleMock.mockResolvedValue(null);
  setUiScaleMock.mockReset();
  setUiScaleMock.mockResolvedValue(true);
  vi.clearAllMocks();
  pluginEntries.current = [];
  authStoreState = { logout: logoutMock, setTokens: vi.fn(), profileId: "prof-2", accessToken: "current-access-token", isAdmin: false };
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
  signOutDeviceMock.mockResolvedValue(undefined);
  eveyFetchMock.mockResolvedValue({});
});

// The interface size is this television's: picked here, stored natively,
// applied at the next process start (the native side restarts the app).
describe("SettingsScreen (TV) interface size", () => {
  it("offers the sizes with the default checked when nothing was ever stored", async () => {
    getUiScaleMock.mockResolvedValue(null);
    render(<SettingsScreen />);
    expect(screen.getAllByText(/^\d+ %$/).map((e) => e.textContent)).toEqual(["70 %", "80 %", "90 %", "100 %"]);
    await act(async () => {});
    const row80 = screen.getByText("80 %").parentElement!;
    expect(row80.textContent).toContain("✓");
  });

  it("shows the stored size and persists a new one through the native module", async () => {
    getUiScaleMock.mockResolvedValue(90);
    render(<SettingsScreen />);
    await act(async () => {});
    expect(screen.getByText("90 %").parentElement!.textContent).toContain("✓");
    fireEvent.click(screen.getByText("70 %"));
    expect(setUiScaleMock).toHaveBeenCalledWith(70);
    // Re-picking the current value is a no-op: no pointless restart.
    setUiScaleMock.mockClear();
    fireEvent.click(screen.getByText("70 %"));
    expect(setUiScaleMock).not.toHaveBeenCalled();
  });
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
  it("ends this device's session server-side (sign-out helper), not just locally", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(signOutDeviceMock).toHaveBeenCalledTimes(1);
  });

  it("clears local session state and navigates away immediately, without waiting on the network call", () => {
    signOutDeviceMock.mockReturnValue(new Promise(() => {})); // never resolves
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(logoutMock).toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/login");
  });

  it("still leaves the local session cleared when the offline TV can't reach the server", async () => {
    signOutDeviceMock.mockRejectedValue(Object.assign(new Error("network down"), { httpStatus: undefined }));
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(logoutMock).toHaveBeenCalled();
    expect(replaceMock).toHaveBeenCalledWith("/login");
    // Let the rejected promise settle - it must be swallowed, not thrown as
    // an unhandled rejection that would fail this test.
    await new Promise((r) => setTimeout(r, 0));
  });

  it("purges the query cache, the refresh token and parental unlocks so the next viewer on a shared TV starts clean", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.signOut"));
    expect(queryClientClearMock).toHaveBeenCalled();
    expect(signOutDeviceMock).toHaveBeenCalled();
    expect(clearUnlockedMock).toHaveBeenCalled();
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
    expect(pushMock).toHaveBeenCalledWith("/profile-select?pick=1");
  });
});

// Regression for the fix landed the night of 2026-08-02/03: entering Settings
// used to hand initial D-pad focus to Sign out, one reflex OK press away from
// a session a keyboard-less television has no comfortable way to restore.
// Changing server signs the television out, and the row is the first thing
// under the thumb in Settings (owner, 2026-09-20): it asks first, with Cancel
// holding the focus, and only the explicit confirmation tears the session down.
describe("SettingsScreen (TV) change server asks first", () => {
  it("does nothing but open the confirmation, with Cancel preselected", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.changeServer"));
    expect(logoutMock).not.toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalledWith("/server-connect");
    const cancel = screen.getByLabelText("common.cancel");
    expect(cancel.getAttribute("data-tv-preferred-focus")).toBe("true");
  });

  it("cancel closes it and keeps the session", () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.changeServer"));
    fireEvent.click(screen.getByLabelText("common.cancel"));
    expect(screen.queryByLabelText("common.cancel")).toBeNull();
    expect(logoutMock).not.toHaveBeenCalled();
  });

  it("confirming signs out and goes to the server page", async () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("mobile.settings.changeServer"));
    const confirm = screen.getAllByLabelText("mobile.settings.changeServer").at(-1)!;
    fireEvent.click(confirm);
    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/server-connect"));
    expect(logoutMock).toHaveBeenCalled();
  });
});

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
    // "prof-1" (Alice) - see handleLanguageChange's comment in settings.tsx:
    // writing the locale onto the default profile instead of the active one
    // was the bug this screen fixed.
    expect(updateProfileMutateAsyncMock).toHaveBeenCalledWith({ id: "prof-2", data: { locale: "fr" } });
  });

  // The JWT refresh that follows must go through auth-refresh (which writes
  // the ROTATED refresh token back to disk), never a bare /auth/refresh call:
  // the bare call rotated the token server-side and left the old one on the
  // device, and the next silent refresh replayed it into a forced logout.
  it("refreshes the JWT through the path that persists the rotated refresh token", async () => {
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("Français"));
    await waitFor(() => expect(forceRefreshMock).toHaveBeenCalledTimes(1));
    expect(eveyFetchMock).not.toHaveBeenCalledWith(expect.objectContaining({ url: "/api/v1/auth/refresh" }));
  });
});

// The TV was the only client that never rendered a profile's avatar image
// on this card, falling back to an initial letter even when one existed.
describe("SettingsScreen (TV) profile card avatar", () => {
  it("renders the avatar image with the token in the Authorization header, not the URL", () => {
    listProfilesMock.mockReturnValue({
      data: {
        profiles: [
          { id: "prof-1", name: "Alice", isDefault: true },
          { id: "prof-2", name: "Chloé", avatarPath: "avatars/chloe.png" },
        ],
      },
    });
    render(<SettingsScreen />);
    const img = screen.getByAltText("") as HTMLImageElement;
    expect(img.src).toBe("http://srv.test/api/v1/profiles/prof-2/avatar");
    expect(img.src).not.toContain("token=");
    expect(img.getAttribute("data-headers")).toBe(
      JSON.stringify({ Authorization: "Bearer current-access-token" }),
    );
  });

  it("falls back to the initial when the active profile has no avatarPath", () => {
    render(<SettingsScreen />);
    expect(screen.queryByAltText("")).toBeNull();
    expect(screen.getByText("C")).toBeTruthy();
  });
});

// A raw <PluginSlot name="settings.user.appearance" /> used to sit inline
// here, stacking every plugin's WebView with nothing saying which plugin
// owned which block. Parity fix with web/mobile: one named row per plugin,
// each opening its own screen; no plugin installed means no section at all.
describe("SettingsScreen (TV) plugins section", () => {
  it("renders no Plugins section when no plugin mounts the slot", () => {
    render(<SettingsScreen />);
    expect(screen.queryByText("auth.settings.tabs.plugins")).toBeNull();
  });

  it("renders one row per plugin that mounts the slot", () => {
    pluginEntries.current = [
      { pluginId: "plg-weather", name: "Weather" },
      { pluginId: "plg-notes", name: "Notes" },
    ];
    render(<SettingsScreen />);
    expect(screen.getByText("auth.settings.tabs.plugins")).toBeTruthy();
    expect(screen.getByLabelText("Weather")).toBeTruthy();
    expect(screen.getByLabelText("Notes")).toBeTruthy();
  });

  it("navigates to the plugin's own settings screen on press", () => {
    pluginEntries.current = [{ pluginId: "plg-weather", name: "Weather" }];
    render(<SettingsScreen />);
    fireEvent.click(screen.getByLabelText("Weather"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/settings/plugins/plg-weather");
  });
});

// GetSystemInfo is an admin-only RPC: a non-admin got a bare 403 on every
// visit to this screen and the version line always read "-".
describe("SettingsScreen (TV) server version is admin-only", () => {
  it("does not call GetSystemInfo for a non-admin user", () => {
    render(<SettingsScreen />);
    expect(systemInfoMock).toHaveBeenCalledWith({ query: { enabled: false } });
    expect(screen.queryByText("common.serverVersion")).toBeNull();
  });

  it("calls GetSystemInfo and shows the version for an admin user", () => {
    authStoreState = { ...authStoreState, isAdmin: true };
    render(<SettingsScreen />);
    expect(systemInfoMock).toHaveBeenCalledWith({ query: { enabled: true } });
    expect(screen.getByText("1.0.0")).toBeTruthy();
  });
});

// The television used to list three of the five built-in themes and none of
// the plugins': a profile on Dark Red or on a plugin theme had no row to
// land on here, and picking a plugin theme must store the plugin's id with
// it, or the phone and the browser cannot resolve the choice.
describe("SettingsScreen (TV) themes", () => {
  it("lists the five built-in themes and the plugins' themes, and saves the plugin's id with its choice", () => {
    pluginThemesRef.current = [{ id: "ocean", name: "Ocean", tokens: { "--accent": "#0af" } }];
    render(<SettingsScreen />);
    for (const name of ["Dark", "AMOLED", "Dark Red", "Light", "Light Blue", "Ocean"]) {
      expect(screen.getByText(name)).toBeTruthy();
    }
    fireEvent.click(screen.getByText("Ocean"));
    expect(setThemeMock).toHaveBeenCalledWith("plugin:Ocean", "ocean");
    fireEvent.click(screen.getByText("Dark Red"));
    expect(setThemeMock).toHaveBeenCalledWith("dark-red", undefined);
    pluginThemesRef.current = [];
  });
});
