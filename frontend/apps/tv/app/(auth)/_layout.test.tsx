import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
// The demo strip fetches the public setup status through react-query; this
// layout test renders without a QueryClient and is not about the demo.
// The lock has its own tests; here it would need a query client.
vi.mock("../../src/components/ProfileLockScreen", () => ({ ProfileLockScreen: () => null }));
vi.mock("../../src/components/DemoBanner", () => ({ DemoBanner: () => null }));
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
}));

// --- expo-router mock --------------------------------------------------------
// Stack is rendered as an inert marker: expo-router itself owns the plain
// Back key (it pops this Stack) - nothing in AuthLayout intercepts it, which
// is exactly the regression this suite guards: the shortcut is an ADDITIONAL
// focusable target, never a reinterpretation of the physical Back key.
const replaceMock = vi.fn();
let pathnameMock = "/media/123";
vi.mock("expo-router", () => ({
  Stack: (p: any) => React.createElement("div", { "data-testid": "stack" }, p?.children),
  Redirect: ({ href }: { href: string }) => React.createElement("div", { "data-testid": "redirect", "data-href": href }),
  router: { replace: (...args: unknown[]) => replaceMock(...args) },
  usePathname: () => pathnameMock,
}));

// --- @evey/store mock ---------------------------------------------------------
// A well-formed (non-expired) fake JWT: header.payload.signature, payload
// base64-decoding to { exp: 9999999999 } - isTokenExpired() in AuthLayout
// would otherwise fail to parse a plain string and treat it as expired,
// redirecting to /login before the shortcut ever gets to render.
const FAKE_TOKEN = "h.eyJleHAiOjk5OTk5OTk5OTl9.s";
const EXPIRED_TOKEN = "h.eyJleHAiOjF9.s";
let tokenMock = FAKE_TOKEN;
const logoutMock = vi.fn();
vi.mock("@evey/store", async (orig) => ({
  ...(await orig<typeof import("@evey/store")>()),
  useAuthStore: (sel: any) => sel({ accessToken: tokenMock, profileId: "p1", logout: logoutMock }),
}));

// --- @expo/vector-icons mock ---------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: (props: any) => React.createElement("span", { "data-testid": `icon-${props.name}` }),
}));

// --- react-i18next mock -------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

// --- hooks/components mocks ---------------------------------------------------
vi.mock("../../src/hooks/useDeviceRegistration", () => ({ useDeviceRegistration: () => {} }));
vi.mock("../../src/hooks/usePresenceHeartbeat", () => ({ usePresenceHeartbeat: () => {} }));
vi.mock("../../src/components/TVMusicPlayer", () => ({ TVMusicPlayer: () => null }));
// The layout now renews a stale token before deciding where to send the
// viewer; auth-refresh pulls in expo-secure-store, which has no native side
// under vitest.
const initI18nMock = vi.fn(async (_url: string) => undefined);
vi.mock("../../src/i18n", () => ({ initI18n: (url: string) => initI18nMock(url) }));
vi.mock("../../src/lib/server-config", () => ({ resolveServerUrl: async () => "http://srv:9091" }));
const refreshMock = vi.fn(async (): Promise<string> => "valid");
vi.mock("../../src/lib/auth-refresh", () => ({ ensureAccessToken: () => refreshMock() }));
vi.mock("../../src/components/ScreenState", () => ({
  ScreenState: ({ onRetry }: { onRetry?: () => void }) => React.createElement("button", { "data-testid": "renewal-retry", onClick: onRetry }, "retry"),
}));
vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", accent: "#88f" } }),
}));

import AuthLayout from "./_layout";

beforeEach(() => {
  vi.clearAllMocks();
  pathnameMock = "/media/123";
  tokenMock = FAKE_TOKEN;
  refreshMock.mockResolvedValue("valid");
});

describe("AuthLayout home shortcut", () => {
  it("renders the Home shortcut on a deep screen with no sidebar", () => {
    pathnameMock = "/media/123";
    render(<AuthLayout />);
    expect(screen.getByLabelText("common.nav.home")).toBeTruthy();
  });

  it("navigates home via router.replace when the shortcut is activated", () => {
    pathnameMock = "/person/9";
    render(<AuthLayout />);
    fireEvent.click(screen.getByLabelText("common.nav.home"));
    expect(replaceMock).toHaveBeenCalledWith("/(auth)/(tabs)");
  });

  it("hides the shortcut on a tab screen - the sidebar already offers Home there", () => {
    pathnameMock = "/library";
    render(<AuthLayout />);
    expect(screen.queryByLabelText("common.nav.home")).toBeNull();
  });

  it("hides the shortcut inside the player - it owns its own exit contract, no shortcut should compete with it", () => {
    pathnameMock = "/player/123";
    render(<AuthLayout />);
    expect(screen.queryByLabelText("common.nav.home")).toBeNull();
  });

  it("never wires the shortcut through router.back - the plain Back key regression this covers", () => {
    // AuthLayout renders the Stack as an inert marker and never calls
    // router.back itself: the Stack (expo-router/React Navigation) still owns
    // popping one level on a plain Back press, untouched by this feature.
    pathnameMock = "/media/123";
    render(<AuthLayout />);
    expect(screen.getByTestId("stack")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("common.nav.home"));
    expect(replaceMock).toHaveBeenCalled();
    expect(replaceMock).not.toHaveBeenCalledWith(undefined);
  });
});

// A boot whose one catalogue fetch failed (the NAS asleep) kept the
// pre-connect strings all session: the signed-in shell retries, as the
// phone's does.
describe("AuthLayout translation catalogue", () => {
  it("gives the catalogue a second chance against the active server", async () => {
    pathnameMock = "/media/123";
    render(<AuthLayout />);
    await vi.waitFor(() => expect(initI18nMock).toHaveBeenCalledWith("http://srv:9091"));
  });
});

// A renewal that throws (the keystore refused the refresh token read) is a
// renewal that failed: the spinner must give way to the sign-in screen
// instead of turning for the rest of the session.
describe("AuthLayout token renewal", () => {
  it("leaves the spinner for /login when the renewal throws", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    tokenMock = EXPIRED_TOKEN;
    refreshMock.mockRejectedValue(new Error("keystore"));
    render(<AuthLayout />);
    await vi.waitFor(() => expect(screen.getByTestId("redirect").getAttribute("data-href")).toBe("/login"));
    expect(screen.queryByTestId("spinner")).toBeNull();
    warn.mockRestore();
  });

  it("signs out when the server refuses the refresh token", async () => {
    tokenMock = EXPIRED_TOKEN;
    refreshMock.mockResolvedValue("refused");
    render(<AuthLayout />);
    await vi.waitFor(() => expect(logoutMock).toHaveBeenCalledOnce());
  });

  // A server that did not answer (a restart during a deployment, a proxy 502)
  // left the session alive: logging out threw a valid refresh token away and
  // left the TV on a password form.
  it("keeps the session and offers a retry when the server does not answer", async () => {
    tokenMock = EXPIRED_TOKEN;
    refreshMock.mockResolvedValue("unavailable");
    render(<AuthLayout />);
    await vi.waitFor(() => expect(screen.getByTestId("renewal-retry")).toBeTruthy());
    expect(logoutMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId("redirect")).toBeNull();
    fireEvent.click(screen.getByTestId("renewal-retry"));
    await vi.waitFor(() => expect(refreshMock).toHaveBeenCalledTimes(2));
    expect(logoutMock).not.toHaveBeenCalled();
  });
});
