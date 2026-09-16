import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// An invite from the URL is checked BEFORE the form appears, so a disabled or
// used link explains itself instead of failing after five fields; the same
// refusal at submit time (403/404/410) lands on that screen too. Passwords
// must match and reach eight characters before anything is sent; success
// stores the token and goes to the profile picker.
const alerts = vi.hoisted(() => ({ calls: [] as string[][] }));
vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children, accessibilityRole }: any) => React.createElement("span", { role: accessibilityRole }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (...a: string[]) => { alerts.calls.push(a); } },
}));
vi.mock("../src/components/PasswordInput", () => ({
  PasswordInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { type: "password", value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
}));
const params = vi.hoisted(() => ({ current: {} as { invite?: string } }));
const router = vi.hoisted(() => ({ replace: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({ router, useLocalSearchParams: () => params.current }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const api = vi.hoisted(() => ({ fetch: vi.fn(), opts: null as any, register: vi.fn() }));
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => api.fetch(...a),
  useUserServiceRegisterWithInvite: (o: any) => {
    api.opts = o.mutation;
    return { mutate: api.register, isPending: false };
  },
}));
const setTokens = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { setTokens: typeof setTokens }) => unknown) => sel({ setTokens }) }));
vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", error: "#f33" }, radii: { md: 8 }, spacing: { md: 16, lg: 24, xl: 32 } }),
}));

import Register from "./register";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><Register /></QueryClientProvider>);
}
const field = (label: string) => screen.getByLabelText(`auth.register.${label}`) as HTMLInputElement;
const submit = () => screen.getByRole("button", { name: "auth.register.createAccountBtn" }) as HTMLButtonElement;
function fill(over: Record<string, string> = {}) {
  const values = { inviteCode: "INV", username: "bob", email: "b@x.io", password: "longenough", confirmPassword: "longenough", ...over };
  for (const [k, v] of Object.entries(values)) fireEvent.change(field(k), { target: { value: v } });
}

beforeEach(() => {
  vi.clearAllMocks();
  alerts.calls.length = 0;
  params.current = {};
  api.fetch.mockReset();
});

describe("register screen (mobile)", () => {
  it("validates locally before sending, then stores the token and opens the profile picker", () => {
    mount();
    expect(submit().disabled).toBe(true);
    fill({ confirmPassword: "different" });
    fireEvent.click(submit());
    expect(screen.getByRole("alert").textContent).toBe("auth.register.passwordMismatch");
    fill({ password: "short", confirmPassword: "short" });
    fireEvent.click(submit());
    expect(screen.getByRole("alert").textContent).toBe("auth.register.passwordMin");
    expect(api.register).not.toHaveBeenCalled();

    fill();
    fireEvent.click(submit());
    expect(api.register).toHaveBeenCalledWith({ data: { inviteCode: "INV", username: "bob", email: "b@x.io", password: "longenough" } });
    api.opts.onSuccess({ accessToken: "acc" });
    expect(setTokens).toHaveBeenCalledWith("acc");
    expect(router.replace).toHaveBeenCalledWith("/profile-select");
    api.opts.onError(new Error("taken"));
    expect(alerts.calls.at(-1)).toEqual(["auth.register.registrationFailed", "taken"]);
  });

  it("checks a link from the URL before showing the form, and explains an unusable one", async () => {
    params.current = { invite: "abc/def" };
    api.fetch.mockResolvedValueOnce({ valid: false });
    mount();
    expect(screen.getByTestId("invite-checking")).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId("invite-invalid")).toBeTruthy());
    expect(api.fetch).toHaveBeenCalledWith({ url: "/api/v1/invites/abc%2Fdef/validate", method: "GET" });
    expect(screen.queryByLabelText("auth.register.username")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "auth.register.backToLogin" }));
    expect(router.replace).toHaveBeenCalledWith("/login");
  });

  it("a valid link pre-fills the code; a refusal at submit time lands on the same screen", async () => {
    params.current = { invite: "good" };
    api.fetch.mockResolvedValueOnce({ valid: true });
    mount();
    await waitFor(() => expect(field("inviteCode").value).toBe("good"));
    fill({ inviteCode: "good" });
    fireEvent.click(submit());
    expect(api.register).toHaveBeenCalledTimes(1);
    api.opts.onError(Object.assign(new Error("gone"), { httpStatus: 410 }));
    await waitFor(() => expect(screen.getByTestId("invite-invalid")).toBeTruthy());
    expect(alerts.calls).toEqual([]);
  });
});
