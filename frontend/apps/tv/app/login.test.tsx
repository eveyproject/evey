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
}));
const router = vi.hoisted(() => ({ replace: vi.fn(), push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const api = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn(), isPending: false }));
vi.mock("@evey/api", () => ({
  useUserServiceLogin: (o: any) => {
    api.opts = o.mutation;
    return { mutate: api.mutate, isPending: api.isPending };
  },
}));
const order = vi.hoisted(() => ({ calls: [] as string[] }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { setTokens: (t: string) => void }) => unknown) => sel({ setTokens: () => { order.calls.push("access"); } }),
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
