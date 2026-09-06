// Offline is a supported mode of this app: a downloaded title must play on a
// plane, and the screens that need no server (Downloads, the local player)
// must stay reachable. Two routing gates used to make that impossible, and
// neither lived anywhere near the player:
//   - the entry screen pinged the server and sent EVERY failure, including a
//     device with no link at all, to /server-connect;
//   - the (auth) layout redirected to /login whenever a token refresh came
//     back false, without distinguishing a refused credential from an
//     unanswered request.
// Both dead-end offline: /server-connect and /login cannot be completed
// without a server. These tests hold the two gates open.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";

// --- React Native ----------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- expo-router -----------------------------------------------------------
// Redirect renders its destination so a test can assert on where the app sent
// the user, which is the whole subject here.
vi.mock("expo-router", () => ({
  Redirect: ({ href }: { href: string }) =>
    React.createElement("div", { "data-testid": "redirect", "data-href": href }),
  Stack: Object.assign(
    ({ children }: any) => React.createElement("div", { "data-testid": "stack" }, children),
    { Screen: () => null },
  ),
}));

// --- auth store ------------------------------------------------------------
const authState = { accessToken: null as string | null, profileId: null as string | null };
const serverListState = { getActiveServer: () => ({ url: "http://srv.test" }) };
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: typeof authState) => unknown) => sel(authState),
  useServerListStore: Object.assign(() => serverListState, { getState: () => serverListState }),
}));

// --- server reachability ---------------------------------------------------
const pingMock = vi.fn(async () => false);
vi.mock("../lib/server-config", () => ({
  pingServer: (url: string) => pingMock(url),
  resolveServerUrl: async (u?: string) => u ?? "http://srv.test",
  getServerUrl: async () => "http://srv.test",
}));

let serverUrl: string | null | undefined = "http://srv.test";
vi.mock("../hooks/useServerUrl", () => ({
  useServerUrlNullable: () => serverUrl,
}));

// --- token refresh ---------------------------------------------------------
const ensureAccessTokenMock = vi.fn(async () => "unavailable" as string);
vi.mock("../lib/auth-refresh", () => ({
  ensureAccessToken: () => ensureAccessTokenMock(),
  refreshAccessTokenIfNeeded: async () => false,
}));

// --- cached profile list ---------------------------------------------------
// The entry screen reads it to decide whether a cold start stops at the
// picker. Empty here: these cases are about the routing gates, not the picker.
let cachedProfiles: Array<{ id: string; name: string; isKids: boolean }> = [];
vi.mock("../lib/profileCache", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/profileCache")>()),
  loadProfileCache: () => Promise.resolve(cachedProfiles),
}));

// --- connectivity ----------------------------------------------------------
let online = false;
vi.mock("../hooks/useOnlineStatus", () => ({ useOnlineStatus: () => online }));

// --- everything the (auth) shell hangs off, none of it under test ----------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", accent: "#f00" } }),
}));
vi.mock("../i18n", () => ({ initI18n: vi.fn(async () => undefined) }));
vi.mock("../hooks/useDeviceRegistration", () => ({ useDeviceRegistration: () => undefined }));
vi.mock("../hooks/useProfileCacheSync", () => ({ useProfileCacheSync: () => undefined }));
vi.mock("../hooks/usePresenceHeartbeat", () => ({ usePresenceHeartbeat: () => undefined }));
vi.mock("../providers/DrawerProvider", () => ({
  DrawerProvider: ({ children }: any) => React.createElement("div", null, children),
}));
vi.mock("../components/SideDrawer", () => ({ SideDrawer: () => null }));
vi.mock("../components/MiniPlayer", () => ({ MiniPlayer: () => null }));
vi.mock("../components/MusicMiniPlayer", () => ({ MusicMiniPlayer: () => null }));
vi.mock("../components/OfflineBanner", () => ({ OfflineBanner: () => null }));
vi.mock("../components/ImpersonationBanner", () => ({ ImpersonationBanner: () => null }));

import EntryRedirect from "../../app/index";
import AuthLayout from "../../app/(auth)/_layout";

function jwt(expSeconds: number): string {
  const body = Buffer.from(JSON.stringify({ exp: expSeconds })).toString("base64url");
  return `h.${body}.s`;
}
const freshToken = () => jwt(Math.floor(Date.now() / 1000) + 3600);
const staleToken = () => jwt(Math.floor(Date.now() / 1000) - 10);

function redirectedTo(): string | null {
  return screen.queryByTestId("redirect")?.getAttribute("data-href") ?? null;
}

beforeEach(() => {
  authState.accessToken = null;
  authState.profileId = null;
  serverUrl = "http://srv.test";
  online = false;
  pingMock.mockClear();
  pingMock.mockResolvedValue(false);
  ensureAccessTokenMock.mockClear();
  ensureAccessTokenMock.mockResolvedValue("unavailable");
  cachedProfiles = [];
});

describe("entry screen - a saved session is not gated on a reachable server", () => {
  it("goes into the app with an unreachable server, and never pings", async () => {
    authState.accessToken = freshToken();
    authState.profileId = "p1";

    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/(auth)/(tabs)"));
    // The probe answers a question nothing reads here: firing it only spends a
    // 5s round-trip, and a 5s spinner, before the same redirect.
    expect(pingMock).not.toHaveBeenCalled();
  });

  it("still sends a user with no saved server to the connect screen", async () => {
    serverUrl = null;
    authState.accessToken = freshToken();
    authState.profileId = "p1";

    render(React.createElement(EntryRedirect));

    expect(redirectedTo()).toBe("/server-connect");
  });

  it("keeps the probe as the gate when there is no session to protect", async () => {
    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/server-connect"));
    expect(pingMock).toHaveBeenCalledWith("http://srv.test");
  });

  // "Who is watching", asked once per process launch and only where somebody
  // could pick wrong. This screen is the initial route, so it mounts on a cold
  // start and never on a resume - which is the distinction the user asked for.
  it("stops at the picker on a cold start when the household has a kids profile", async () => {
    authState.accessToken = freshToken();
    authState.profileId = "p1";
    online = true;
    cachedProfiles = [
      { id: "p1", name: "Papa", isKids: false },
      { id: "p2", name: "Lou", isKids: true },
    ];

    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/profile-select"));
  });

  it("walks straight in when no profile is a kids profile", async () => {
    authState.accessToken = freshToken();
    authState.profileId = "p1";
    online = true;
    cachedProfiles = [
      { id: "p1", name: "Papa", isKids: false },
      { id: "p2", name: "Maman", isKids: false },
    ];

    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/(auth)/(tabs)"));
  });

  // Offline the picker leads nowhere new - switching profiles is a server
  // round-trip - and making the one screen a signed-in user must cross into a
  // network dependency is how the app stopped working on a plane last time.
  it("skips the picker offline and keeps the profile the session was left on", async () => {
    authState.accessToken = freshToken();
    authState.profileId = "p1";
    online = false;
    cachedProfiles = [
      { id: "p1", name: "Papa", isKids: false },
      { id: "p2", name: "Lou", isKids: true },
    ];

    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/(auth)/(tabs)"));
  });

  it("sends an authenticated user with no profile to the profile picker", async () => {
    authState.accessToken = freshToken();
    pingMock.mockResolvedValue(true);

    render(React.createElement(EntryRedirect));

    await waitFor(() => expect(redirectedTo()).toBe("/profile-select"));
  });
});

describe("(auth) layout - an unanswered refresh is not a logout", () => {
  it("renders the app when the access token aged out with no network", async () => {
    authState.accessToken = staleToken();
    authState.profileId = "p1";
    ensureAccessTokenMock.mockResolvedValue("unavailable");

    render(React.createElement(AuthLayout));

    await waitFor(() => expect(screen.queryByTestId("stack")).not.toBeNull());
    expect(redirectedTo()).toBeNull();
  });

  it("goes to /login when the server REFUSES the refresh credential", async () => {
    authState.accessToken = staleToken();
    authState.profileId = "p1";
    ensureAccessTokenMock.mockResolvedValue("refused");

    render(React.createElement(AuthLayout));

    await waitFor(() => expect(redirectedTo()).toBe("/login"));
  });

  it("renders nothing while the refresh is still in flight", async () => {
    authState.accessToken = staleToken();
    authState.profileId = "p1";
    ensureAccessTokenMock.mockImplementation(() => new Promise(() => {}));

    render(React.createElement(AuthLayout));

    expect(screen.queryByTestId("stack")).toBeNull();
    expect(redirectedTo()).toBeNull();
  });

  it("retries the refresh when connectivity comes back", async () => {
    authState.accessToken = staleToken();
    authState.profileId = "p1";

    const { rerender } = render(React.createElement(AuthLayout));
    await waitFor(() => expect(ensureAccessTokenMock).toHaveBeenCalledTimes(1));

    online = true;
    rerender(React.createElement(AuthLayout));

    await waitFor(() => expect(ensureAccessTokenMock).toHaveBeenCalledTimes(2));
  });

  it("still bounces a session with no token at all", () => {
    render(React.createElement(AuthLayout));
    expect(redirectedTo()).toBe("/login");
  });
});
