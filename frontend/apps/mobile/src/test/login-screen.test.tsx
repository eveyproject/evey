import { describe, it, expect, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Guards the keyboard-covers-fields fix: the screen used to stack its fields
// in a plain non-scrollable View, so once the keyboard opened there was no
// way to reach the sign-in button or the links below it.

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Alert: { alert: vi.fn() },
  Linking: { addEventListener: () => ({ remove: vi.fn() }), openURL: vi.fn() },
  ScrollView: ({ children, ...p }: any) => React.createElement("div", { "data-testid": "scroll-view", ...p }, children),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

const routerMock = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({
  router: routerMock,
  useFocusEffect: (cb: () => void | (() => void)) => React.useEffect(cb, [cb]),
}));

// The store a sign-in screen watches: a token landing from outside (the boot
// or resume renewal) is announced to its subscribers.
const store = vi.hoisted(() => ({
  state: { accessToken: null as string | null },
  listeners: [] as Array<(s: { accessToken: string | null }, prev: { accessToken: string | null }) => void>,
  land(accessToken: string | null) {
    const prev = this.state;
    this.state = { accessToken };
    for (const l of this.listeners) l(this.state, prev);
  },
}));
const renew = vi.hoisted(() => ({ fn: vi.fn(async () => false) }));
vi.mock("../lib/auth-refresh", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../lib/auth-refresh")>()),
  renewOnSignInScreen: () => renew.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

const setupStatus = vi.hoisted(() => ({ data: undefined as { passwordRecoveryEnabled?: boolean } | undefined }));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceLogin: () => ({ mutate: vi.fn(), isPending: false }),
  useAdminSystemServiceGetSetupStatus: () => setupStatus,
  eveyFetch: vi.fn(() => Promise.resolve({ enabled: false })),
}));

vi.mock("@evey/store", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/store")>()),
  useAuthStore: Object.assign(
    (selector: (s: any) => unknown) => selector({ setTokens: vi.fn(), accessToken: null }),
    {
      getState: () => store.state,
      subscribe: (l: (typeof store.listeners)[number]) => { store.listeners.push(l); return () => { store.listeners.splice(store.listeners.indexOf(l), 1); }; },
    },
  ),
  useServerListStore: (selector: (s: any) => unknown) => selector({ activeServerId: null, setServerName: vi.fn() }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      surface: "#111",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      border: "#333",
    },
    radii: { md: 8 },
    spacing: { xl: 24, lg: 16, md: 12, sm: 8 },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));

vi.mock("../lib/refresh-token", () => ({ setRefreshToken: vi.fn() }));

const keystore = vi.hoisted(() => ({ available: true }));

vi.mock("../lib/saved-credentials", () => ({
  canSaveCredentials: vi.fn(() => Promise.resolve(keystore.available)),
  getSavedCredentials: vi.fn(() => Promise.resolve(null)),
  saveCredentials: vi.fn(),
  clearSavedCredentials: vi.fn(),
}));

vi.mock("../hooks/useBiometric", () => ({
  useBiometricAvailable: () => ({ available: false, biometricType: "fingerprint" }),
  useBiometricEnabled: () => ({ enabled: false }),
  authenticateBiometric: vi.fn(() => Promise.resolve(false)),
}));

vi.mock("../components/PasswordInput", () => ({
  PasswordInput: (p: any) => React.createElement("input", { type: "password", ...p }),
}));

// -------------------------------------------------------------------------------------
import Login from "../../app/login";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

function renderLogin() {
  return render(React.createElement(Login), { wrapper: makeWrapper() });
}

function hasRememberMe(container: HTMLElement): boolean {
  return [...container.querySelectorAll("button")].some(
    (b) => b.getAttribute("accessibilityLabel") === "Remember me",
  );
}

function hasForgotPasswordLink(container: HTMLElement): boolean {
  return [...container.querySelectorAll("button")].some(
    (b) => b.getAttribute("accessibilityLabel") === "auth.login.forgotPassword",
  );
}

describe("Login screen", () => {
  it("wraps its fields in a scrollable container so the keyboard can't hide controls below the fold", () => {
    setupStatus.data = { passwordRecoveryEnabled: true };
    const { container } = renderLogin();
    expect(container.querySelector('[data-testid="scroll-view"]')).toBeTruthy();
  });

  it("offers the reset link when the server has password recovery configured", () => {
    setupStatus.data = { passwordRecoveryEnabled: true };
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(true);
  });

  // Without the gate the link led to a screen that could only apologize.
  it("hides the reset link when recovery is disabled server-side", () => {
    setupStatus.data = { passwordRecoveryEnabled: false };
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(false);
  });

  it("hides the reset link while the setup status is still unknown", () => {
    setupStatus.data = undefined;
    expect(hasForgotPasswordLink(renderLogin().container)).toBe(false);
  });

  it("offers remember me when the device has a keystore", async () => {
    keystore.available = true;
    const { container } = renderLogin();
    await waitFor(() => expect(hasRememberMe(container)).toBe(true));
  });

  // The checkbox used to stay ticked on a device without a keystore while
  // nothing was saved: a promise the device could not keep.
  it("hides remember me when the device has no keystore", async () => {
    keystore.available = false;
    const { container } = renderLogin();
    await new Promise((r) => setTimeout(r, 0));
    expect(hasRememberMe(container)).toBe(false);
  });
});

function liveJwt(): string {
  return `h.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 900 }))}.s`;
}

// A boot whose renewal got no answer lands here with a session the server
// may still honour: the screen renews first, and leaves for the picker as
// soon as a live token is held - from that renewal, or from the resume
// renewal landing while the form is up.
describe("Login screen - a session the server may still honour", () => {
  it("asks for a renewal on arrival", () => {
    renew.fn.mockClear();
    renderLogin();
    expect(renew.fn).toHaveBeenCalledOnce();
  });

  it("leaves for the picker when a live token lands from outside, not when the screen is gone", () => {
    routerMock.replace.mockClear();
    store.state = { accessToken: null };
    const { unmount } = renderLogin();
    store.land(`h.${btoa(JSON.stringify({ exp: 1 }))}.s`);
    expect(routerMock.replace).not.toHaveBeenCalled();
    store.land(liveJwt());
    expect(routerMock.replace).toHaveBeenCalledWith("/profile-select");
    routerMock.replace.mockClear();
    unmount();
    store.land(liveJwt());
    expect(routerMock.replace).not.toHaveBeenCalled();
  });
});
