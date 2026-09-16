import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// The reset request must not leak whether an address exists: success and
// refusal both land on the same "check your email" screen. A blank address
// sends nothing, the button freezes while the request is in flight, and the
// way back is the previous screen (login).
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
}));
const back = vi.fn();
vi.mock("expo-router", () => ({ router: { back } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const hook = { opts: null as any, mutate: vi.fn(), isPending: false };
vi.mock("@evey/api", () => ({
  useUserServiceRequestPasswordReset: (o: any) => {
    hook.opts = o.mutation;
    return { mutate: hook.mutate, isPending: hook.isPending };
  },
}));
vi.mock("../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", border: "#333", accent: "#f00", onAccent: "#000" }, spacing: { md: 16, lg: 24, xl: 32 }, radii: { md: 8 } }),
}));

async function renderScreen() {
  const mod = await import("./forgot-password");
  render(React.createElement(mod.default));
}

beforeEach(() => {
  vi.clearAllMocks();
  hook.isPending = false;
});

describe("forgot password screen (mobile)", () => {
  it("sends the address and lands on the same screen whether the server accepted or refused", async () => {
    await renderScreen();
    fireEvent.click(screen.getByRole("button", { name: "auth.forgotPassword.submit" }));
    expect(hook.mutate).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText("auth.forgotPassword.email"), { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: "auth.forgotPassword.submit" }));
    expect(hook.mutate).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("auth.forgotPassword.email"), { target: { value: "me@example.com" } });
    fireEvent.click(screen.getByRole("button", { name: "auth.forgotPassword.submit" }));
    expect(hook.mutate).toHaveBeenCalledWith({ data: { email: "me@example.com" } });

    act(() => hook.opts.onError(new Error("404")));
    expect(screen.getByText("auth.forgotPassword.checkEmail")).toBeTruthy();
    expect(screen.queryByLabelText("auth.forgotPassword.email")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "auth.forgotPassword.backToLogin" }));
    expect(back).toHaveBeenCalledTimes(1);
  });

  it("success lands on the same screen", async () => {
    await renderScreen();
    act(() => hook.opts.onSuccess());
    expect(screen.getByText("auth.forgotPassword.emailSent")).toBeTruthy();
  });

  it("freezes the button while the request is in flight", async () => {
    hook.isPending = true;
    await renderScreen();
    const btn = screen.getByRole("button", { name: "auth.forgotPassword.submit" }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });
});
