import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const search = vi.hoisted(() => ({ code: undefined as string | undefined }));
const navigate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => ({ ...opts, useSearch: () => search }),
  useNavigate: () => navigate,
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const hook = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn() }));
vi.mock("@evey/api", () => ({
  useUserServiceAuthorizeQuickConnect: (o: any) => {
    hook.opts = o.mutation;
    return { mutate: hook.mutate, isPending: false };
  },
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, type, ...p }: any) =>
    React.createElement("button", { onClick, disabled: disabled || loading, type, "data-testid": p["data-testid"] }, children),
}));
vi.mock("@/components/ui/Input", () => ({
  Input: ({ label, value, onChange, maxLength, ...p }: any) =>
    React.createElement("input", { "aria-label": label, value, onChange, maxLength, "data-testid": p["data-testid"] }),
}));

import { Route } from "./quickconnect.lazy";
const Page = (Route as unknown as { component: React.ComponentType }).component;

const input = () => screen.getByTestId("quick-connect-code-input") as HTMLInputElement;
const submit = () => screen.getByTestId("quick-connect-authorize-btn") as HTMLButtonElement;

beforeEach(() => {
  vi.clearAllMocks();
  search.code = undefined;
});

// A typed code is upper-cased and must be at least four characters; a full
// code arriving from a scanned QR link is shown as its last six characters
// but sent whole; success swaps to the confirmation, a refusal is a toast.
describe("quick connect page", () => {
  it("upper-cases a typed code, refuses fewer than four characters, and sends the trimmed code", () => {
    render(<Page />);
    expect(submit().disabled).toBe(true);
    fireEvent.change(input(), { target: { value: "ab" } });
    expect(input().value).toBe("AB");
    expect(submit().disabled).toBe(true);
    fireEvent.change(input(), { target: { value: "a1b2c3" } });
    expect(submit().disabled).toBe(false);
    fireEvent.click(submit());
    expect(hook.mutate).toHaveBeenCalledWith({ data: { code: "A1B2C3" } });
  });

  it("shows the tail of a scanned code but authorises the whole of it", () => {
    search.code = "full-quick-connect-code-xyz123";
    render(<Page />);
    expect(input().value).toBe("XYZ123");
    fireEvent.click(submit());
    expect(hook.mutate).toHaveBeenCalledWith({ data: { code: "full-quick-connect-code-xyz123" } });
  });

  it("confirms on success and returns home; toasts a refused code", () => {
    render(<Page />);
    fireEvent.change(input(), { target: { value: "A1B2C3" } });
    hook.opts.onError();
    expect(toast.error).toHaveBeenCalledWith("auth.settings.quickConnect.invalidCode");
    expect(screen.queryByTestId("quick-connect-code-input")).toBeTruthy();

    fireEvent.click(submit());
    act(() => hook.opts.onSuccess());
    expect(toast.success).toHaveBeenCalledWith("auth.settings.quickConnect.authorized");
    expect(screen.queryByTestId("quick-connect-code-input")).toBeNull();
    expect(screen.getByRole("heading")).toHaveTextContent("auth.settings.quickConnect.authorized");
    fireEvent.click(screen.getByText("common.back"));
    expect(navigate).toHaveBeenCalledWith({ to: "/home" });
  });
});
