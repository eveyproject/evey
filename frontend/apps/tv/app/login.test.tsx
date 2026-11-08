import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// Quick Connect takes the remote's initial focus (typing a password on a
// D-pad is the last resort); a password login trims the name, refuses blanks,
// stores the refresh token before the access token so the session survives
// the next cold start, and lands on the profile picker; a refusal is named.
const alerts = vi.hoisted(() => ({ calls: [] as string[][] }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel, onSubmitEditing }: any) =>
    React.createElement("input", {
      value, "aria-label": accessibilityLabel,
      onChange: (e: any) => onChangeText(e.target.value),
      onKeyDown: (e: any) => e.key === "Enter" && onSubmitEditing?.(),
    }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel, "data-focus": String(!!hasTVPreferredFocus) }, children),
  Alert: { alert: (...a: string[]) => { alerts.calls.push(a); } },
  StyleSheet: { create: (s: any) => s },
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  BackHandler: { addEventListener: (_e: string, h: () => boolean) => { back.handler = h; return { remove: () => { back.handler = null; } }; } },
}));
const back = vi.hoisted(() => ({ handler: null as (() => boolean) | null }));
const server = vi.hoisted(() => ({ cleared: 0, cacheCleared: 0 }));
vi.mock("../src/lib/server-config", () => ({ clearServerUrl: async () => { server.cleared += 1; } }));
vi.mock("../src/lib/query-client", () => ({ queryClient: { clear: () => { server.cacheCleared += 1; } } }));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn() }));
vi.mock("expo-router", () => ({ router, useFocusEffect: (cb: () => void | (() => void)) => React.useEffect(cb, [cb]) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const api = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn(), isPending: false }));
vi.mock("@evey/api", () => ({
  useUserServiceLogin: (o: any) => {
    api.opts = o.mutation;
    return { mutate: api.mutate, isPending: api.isPending };
  },
  useAdminSystemServiceGetSetupStatus: () => ({ data: undefined }),
}));
const order = vi.hoisted(() => ({ calls: [] as string[] }));
// The store a sign-in screen watches: a token landing from outside (the boot
// or wake renewal) is announced to its subscribers.
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
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign(
    (sel: (s: { setTokens: (t: string) => void }) => unknown) => sel({ setTokens: () => { order.calls.push("access"); } }),
    {
      getState: () => store.state,
      subscribe: (l: (typeof store.listeners)[number]) => { store.listeners.push(l); return () => { store.listeners.splice(store.listeners.indexOf(l), 1); }; },
    },
  ),
  useDemoStore: { getState: () => ({ setDemoMode: () => {} }) },
}));
vi.mock("../src/lib/auth-refresh", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../src/lib/auth-refresh")>()),
  renewOnSignInScreen: () => renew.fn(),
}));
vi.mock("../src/lib/refresh-token", () => ({ setRefreshToken: async () => { order.calls.push("refresh"); } }));
vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333" }, radii: { md: 8 } }),
}));

import Login from "./login";

const signIn = () => screen.getByRole("button", { name: "auth.login.signIn" }) as HTMLButtonElement;

beforeEach(() => {
  vi.clearAllMocks();
  alerts.calls.length = 0;
  order.calls.length = 0;
  api.isPending = false;
  server.cleared = 0;
  server.cacheCleared = 0;
  back.handler = null;
  store.state = { accessToken: null };
  store.listeners.length = 0;
  renew.fn.mockClear();
});

function liveJwt(): string {
  return `h.${btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + 900 }))}.s`;
}

// A boot whose renewal got no answer lands here with a session the server
// may still honour: the screen renews first, and leaves for the picker as
// soon as a live token is held - from that renewal, or from the wake renewal
// landing while the form is up. Its own sign-in lands on its own.
describe("tv login screen - a session the server may still honour", () => {
  it("asks for a renewal on arrival", () => {
    render(<Login />);
    expect(renew.fn).toHaveBeenCalledOnce();
  });

  it("leaves for the picker when a live token lands from outside", () => {
    render(<Login />);
    store.land(liveJwt());
    expect(router.replace).toHaveBeenCalledWith("/profile-select");
  });

  it("stays when the token that lands is expired, and when the screen is gone", () => {
    const { unmount } = render(<Login />);
    store.land(`h.${btoa(JSON.stringify({ exp: 1 }))}.s`);
    expect(router.replace).not.toHaveBeenCalled();
    unmount();
    store.land(liveJwt());
    expect(router.replace).not.toHaveBeenCalled();
  });

  it("lets its own sign-in land on its own: one navigation, not two", async () => {
    render(<Login />);
    fireEvent.change(screen.getByLabelText("auth.login.username"), { target: { value: "ann" } });
    fireEvent.change(screen.getByLabelText("auth.login.password"), { target: { value: "pw" } });
    fireEvent.click(signIn());
    // The mutation's setTokens announces the token while the sign-in is in flight.
    store.land(liveJwt());
    await api.opts.onSuccess({ accessToken: "acc", refreshToken: "ref" });
    api.opts.onSettled();
    expect(router.replace).toHaveBeenCalledTimes(1);
    expect(router.replace).toHaveBeenCalledWith("/profile-select");
  });
});

// Once a server is chosen this screen is the root: the remote's BACK key
// used to leave the app for the Android home, and a viewer who picked the
// wrong server from the LAN scan (a dev box, 2026-09-19) had no way back.
describe("tv login screen - back to the server screen", () => {
  it("offers a change-server action that forgets the URL, drops the cache and goes to the server screen", async () => {
    render(React.createElement(Login));
    fireEvent.click(screen.getByRole("button", { name: "mobile.settings.changeServer" }));
    await Promise.resolve();
    await Promise.resolve();
    expect(server.cleared).toBe(1);
    expect(server.cacheCleared).toBe(1);
    expect(router.replace).toHaveBeenCalledWith("/server-connect");
  });

  it("the remote's BACK key does the same instead of leaving the app", async () => {
    const { unmount } = render(React.createElement(Login));
    expect(back.handler).not.toBeNull();
    expect(back.handler!()).toBe(true);
    await Promise.resolve();
    await Promise.resolve();
    expect(server.cleared).toBe(1);
    expect(router.replace).toHaveBeenCalledWith("/server-connect");
    unmount();
    expect(back.handler, "listener removed with the screen").toBeNull();
  });
});

describe("tv login screen", () => {
  it("gives Quick Connect the initial focus and opens it", () => {
    render(<Login />);
    const qc = screen.getByRole("button", { name: "auth.settings.quickConnect.title" });
    expect(qc.getAttribute("data-focus")).toBe("true");
    expect(signIn().getAttribute("data-focus")).toBe("false");
    fireEvent.click(qc);
    expect(router.push).toHaveBeenCalledWith("/quick-connect");
  });

  it("refuses blanks, trims the name, and Enter in the password field submits", () => {
    render(<Login />);
    fireEvent.click(signIn());
    expect(api.mutate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("auth.login.username"), { target: { value: "  bob " } });
    fireEvent.click(signIn());
    expect(api.mutate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("auth.login.password"), { target: { value: "pw" } });
    fireEvent.keyDown(screen.getByLabelText("auth.login.password"), { key: "Enter" });
    expect(api.mutate).toHaveBeenCalledWith({ data: { username: "bob", password: "pw" } });
  });

  // Production, 2026-09-22: every attempt of one account arrived TWICE within
  // the same second (two request ids), because the password field's submit
  // and the Sign in button both fire on the same OK press. The server locks
  // an account after five failures in fifteen minutes, so a viewer typing on
  // a remote-control keyboard was locked out after barely two tries.
  it("sends one request per press, even when the field and the button both fire", () => {
    render(<Login />);
    fireEvent.change(screen.getByLabelText("auth.login.username"), { target: { value: "bob" } });
    fireEvent.change(screen.getByLabelText("auth.login.password"), { target: { value: "pw" } });
    fireEvent.keyDown(screen.getByLabelText("auth.login.password"), { key: "Enter" });
    fireEvent.click(signIn());
    expect(api.mutate).toHaveBeenCalledTimes(1);
  });

  it("lets the viewer try again once the server has answered", () => {
    render(<Login />);
    fireEvent.change(screen.getByLabelText("auth.login.username"), { target: { value: "bob" } });
    fireEvent.change(screen.getByLabelText("auth.login.password"), { target: { value: "pw" } });
    fireEvent.keyDown(screen.getByLabelText("auth.login.password"), { key: "Enter" });
    api.opts.onSettled?.();
    fireEvent.click(signIn());
    expect(api.mutate).toHaveBeenCalledTimes(2);
  });

  it("stores the refresh token before the access token, then goes to the profile picker; a refusal is named", async () => {
    render(<Login />);
    await api.opts.onSuccess({ accessToken: "acc", refreshToken: "ref" });
    expect(order.calls).toEqual(["refresh", "access"]);
    expect(router.replace).toHaveBeenCalledWith("/profile-select");

    await api.opts.onSuccess({});
    expect(router.replace).toHaveBeenCalledTimes(1);

    api.opts.onError(new Error("locked out"));
    expect(alerts.calls.at(-1)).toEqual(["auth.login.loginFailed", "locked out"]);
    api.opts.onError("weird");
    expect(alerts.calls.at(-1)).toEqual(["auth.login.loginFailed", "auth.login.invalidCredentials"]);
  });

  it("freezes the button while signing in", () => {
    api.isPending = true;
    render(<Login />);
    expect(signIn().disabled).toBe(true);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });
});
