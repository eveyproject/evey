import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// The region is written onto the ACTIVE profile (the one the parental age
// check reads), never the household default; "Automatic" is the empty code at
// the top; the search matches names and codes; a refused save is a toast.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement("div", { "data-testid": "list" }, (data ?? []).map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel, "aria-checked": accessibilityState?.selected }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k, i18n: { language: "fr" } }),
}));
const invalidate = vi.fn();
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const mutateAsync = vi.fn();
const mutationOpts = { current: null as any };
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
  useUserServiceUpdateProfile: (o: any) => {
    mutationOpts.current = o.mutation;
    return { mutateAsync };
  },
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));
const toast = { error: vi.fn() };
vi.mock("@evey/store", () => ({ useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }), isDemoMode: () => false, toast }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", border: "#333", surfaceRaised: "#222", accent: "#f00" }, spacing: { sm: 8, md: 16 }, radii: { md: 8 } }),
}));
vi.mock("../../../src/lib/regionOptions", () => ({
  regionOptions: () => [{ code: "FR", label: "France" }, { code: "DE", label: "Allemagne" }, { code: "US", label: "États-Unis" }],
}));
const active = { profile: { id: "p-active", region: "FR" } as { id?: string; region?: string } | undefined };
vi.mock("../../../src/hooks/useActiveProfile", () => ({ useActiveProfile: () => ({ profile: active.profile }) }));

async function renderScreen() {
  const mod = await import("./region");
  render(React.createElement(mod.default));
}

beforeEach(() => {
  vi.clearAllMocks();
  mutateAsync.mockResolvedValue({});
  active.profile = { id: "p-active", region: "FR" };
});

describe("region screen (mobile)", () => {
  it("lists Automatic first, marks the active profile's region, and writes a pick onto that profile", async () => {
    await renderScreen();
    const rows = screen.getAllByRole("radio");
    expect(rows.map((r) => r.getAttribute("aria-label"))).toEqual(["common.region.auto", "France", "Allemagne", "États-Unis"]);
    expect(rows[1].getAttribute("aria-checked")).toBe("true");
    expect(rows[0].getAttribute("aria-checked")).toBe("false");
    fireEvent.click(rows[2]);
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({ id: "p-active", data: { region: "DE" } }));
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] }));
    fireEvent.click(rows[0]);
    await waitFor(() => expect(mutateAsync).toHaveBeenLastCalledWith({ id: "p-active", data: { region: "" } }));
  });

  it("filters by name or code, keeping Automatic", async () => {
    await renderScreen();
    fireEvent.change(screen.getByPlaceholderText("common.search"), { target: { value: "us" } });
    expect(screen.getAllByRole("radio").map((r) => r.getAttribute("aria-label"))).toEqual(["common.region.auto", "États-Unis"]);
    fireEvent.change(screen.getByPlaceholderText("common.search"), { target: { value: "alle" } });
    expect(screen.getAllByRole("radio").map((r) => r.getAttribute("aria-label"))).toEqual(["common.region.auto", "Allemagne"]);
  });

  it("writes nothing without an active profile, and a refused save toasts without throwing", async () => {
    active.profile = undefined;
    await renderScreen();
    fireEvent.click(screen.getAllByRole("radio")[1]);
    expect(mutateAsync).not.toHaveBeenCalled();

    active.profile = { id: "p-active" };
    mutateAsync.mockRejectedValueOnce(new Error("403"));
    await renderScreen();
    fireEvent.click(screen.getAllByRole("radio").at(-1)!);
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledTimes(1));
    expect(invalidate).not.toHaveBeenCalled();
    mutationOpts.current.onError();
    expect(toast.error).toHaveBeenCalledWith("Failed to update region");
  });
});
