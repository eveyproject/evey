import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// A new key is created with a trimmed name and the ticked scopes, shown ONCE
// in its own sheet (the server never returns it again) with a share action,
// and the list refreshed; a revoke is confirmed by name before the call, and
// a refused revoke is reported instead of leaving the key looking active.
const alerts = vi.hoisted(() => ({ last: null as any }));
const share = vi.hoisted(() => vi.fn(async () => ({})));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement("div", null, (data ?? []).length === 0 ? ListEmptyComponent : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel, "aria-checked": accessibilityState?.checked }, children),
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
  Share: { share: (...a: unknown[]) => share(...(a as [])) },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("expo-router", () => ({
  Stack: { Screen: ({ options }: any) => React.createElement("div", null, options.headerRight?.()) },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: string | { defaultValue?: string; date?: string }) => (typeof o === "string" ? o : o?.defaultValue ? o.defaultValue.replace("{{date}}", o.date ?? "") : k) }),
}));
const api = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  createOpts: null as any, create: vi.fn(),
  revokeOpts: null as any, revoke: vi.fn(),
}));
vi.mock("@evey/api", () => ({
  useUserServiceListAPIKeys: () => api.list,
  useUserServiceCreateAPIKey: (o: any) => {
    api.createOpts = o.mutation;
    return { mutateAsync: api.create, isPending: false };
  },
  useUserServiceRevokeAPIKey: (o: any) => {
    api.revokeOpts = o.mutation;
    return { mutate: api.revoke };
  },
  getUserServiceListAPIKeysQueryKey: () => ["keys"],
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", danger: "#f33" }, radii: { md: 8, lg: 12 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

import AdminAPIKeysScreen from "./api-keys";

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  api.list = { data: { apiKeys: [{ id: "k1", name: "CI", scopes: ["media:read"], createdAt: "2026-09-01T00:00:00Z" }, { id: "k2", name: "Wide", scopes: [] }] }, isLoading: false, isError: false, refetch: vi.fn() };
  api.create.mockImplementation(async (vars: unknown) => {
    const resp = { key: "sk_live_secret" };
    await api.createOpts.onSuccess(resp, vars);
    return resp;
  });
});

describe("admin api keys screen (mobile)", () => {
  it("lists keys with their scopes and confirms a revoke by name before calling", async () => {
    render(<AdminAPIKeysScreen />);
    expect(screen.getByText("CI")).toBeTruthy();
    expect(screen.getByText("media:read")).toBeTruthy();
    expect(screen.getByText("auth.apiKeys.scopesAll")).toBeTruthy();
    expect(screen.getByText(/Created: /)).toBeTruthy();
    expect(screen.getAllByText(/Never used/)).toHaveLength(2);

    fireEvent.click(screen.getAllByRole("button", { name: "auth.apiKeys.revokeTitle" })[1]);
    expect(alerts.last.message).toBe('Revoke "Wide"?');
    expect(api.revoke).not.toHaveBeenCalled();
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.revoke).toHaveBeenCalledWith({ id: "k2" });
    await api.revokeOpts.onSuccess();
    expect(toast.success).toHaveBeenCalledWith("auth.apiKeys.keyRevoked");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["keys"] });
    api.revokeOpts.onError();
    expect(toast.error).toHaveBeenCalledWith("Failed to revoke key");
  });

  it("creates a key with a trimmed name and the ticked scopes, then shows it once with a share action", async () => {
    render(<AdminAPIKeysScreen />);
    expect(screen.queryByTestId("modal")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "auth.apiKeys.newKeyTitle" }));
    const create = screen.getByText("common.create").closest("button") as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.change(screen.getByPlaceholderText("auth.apiKeys.name"), { target: { value: "  Deploy  " } });
    expect(create.disabled).toBe(false);
    const boxes = screen.getAllByRole("checkbox");
    expect(boxes).toHaveLength(8);
    fireEvent.click(boxes[0]);
    fireEvent.click(boxes[3]);
    fireEvent.click(boxes[0]);
    expect(boxes[3].getAttribute("aria-checked")).toBe("true");
    expect(boxes[0].getAttribute("aria-checked")).toBe("false");

    fireEvent.click(create);
    await waitFor(() => expect(screen.getByText("sk_live_secret")).toBeTruthy());
    expect(api.create).toHaveBeenCalledWith({ data: { name: "Deploy", scopes: ["admin:write"] } });
    expect(toast.success).toHaveBeenCalledWith("auth.apiKeys.keyCreated");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["keys"] });
    fireEvent.click(screen.getByRole("button", { name: "Copy" }));
    expect(share).toHaveBeenCalledWith({ message: "sk_live_secret" });
    fireEvent.click(screen.getByText("common.done"));
    expect(screen.queryByText("sk_live_secret")).toBeNull();
    // The form is reset for the next key.
    fireEvent.click(screen.getByRole("button", { name: "auth.apiKeys.newKeyTitle" }));
    expect((screen.getByPlaceholderText("auth.apiKeys.name") as HTMLInputElement).value).toBe("");
  });

  it("keeps loading, failure and no-keys apart", () => {
    api.list = { data: { apiKeys: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    const first = render(<AdminAPIKeysScreen />);
    expect(screen.getByText("auth.apiKeys.noKeys")).toBeTruthy();
    first.unmount();
    api.list = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    render(<AdminAPIKeysScreen />);
    fireEvent.click(screen.getByText("retry"));
    expect(api.list.refetch).toHaveBeenCalledTimes(1);
  });
});
