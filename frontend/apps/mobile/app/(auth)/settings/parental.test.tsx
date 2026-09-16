import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Everything here reads and writes the ACTIVE profile (the one the gate
// enforces), never the household default. The age tick moves only once the
// server has taken the cap (a failed save must not show a restriction that
// is not in force). A first PIN needs the account password, a rotation the
// current PIN; digits only, four of them, matching.
const alerts = vi.hoisted(() => ({ calls: [] as string[][] }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, onChange: (e: any) => onChangeText(e.target.value) }),
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", checked: value, onChange: () => onValueChange(!value) }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  Alert: { alert: (...a: string[]) => { alerts.calls.push(a); } },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const api = vi.hoisted(() => ({ parental: undefined as unknown, update: vi.fn(), getArgs: null as any }));
vi.mock("@evey/api", () => ({
  useParentalServiceGetParentalSettings: (id: string, o: any) => {
    api.getArgs = { id, enabled: o.query.enabled };
    return { data: api.parental };
  },
  parentalServiceUpdateParentalSettings: (...a: unknown[]) => api.update(...a),
  getParentalServiceGetParentalSettingsQueryKey: (id: string) => ["parental", id],
  apiErrorMessage: (e: Error) => e.message,
}));
const active = vi.hoisted(() => ({ profile: { id: "p-active" } as { id?: string } | undefined }));
vi.mock("../../../src/hooks/useActiveProfile", () => ({ useActiveProfile: () => ({ profile: active.profile }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333" }, radii: { md: 8 }, spacing: { xs: 4, sm: 8, md: 16, lg: 24 } }),
}));

import ParentalSettingsScreen from "./parental";

let qc: QueryClient;
function mount() {
  qc = new QueryClient({ defaultOptions: { mutations: { retry: false } } });
  return render(<QueryClientProvider client={qc}><ParentalSettingsScreen /></QueryClientProvider>);
}
const ageButton = (label: string) => screen.getByText(label).closest("button") as HTMLButtonElement;

beforeEach(() => {
  vi.clearAllMocks();
  alerts.calls.length = 0;
  active.profile = { id: "p-active" };
  api.parental = { enabled: true, hasPin: false, maxAge: 12 };
  api.update.mockResolvedValue({});
});

describe("parental settings screen (mobile)", () => {
  it("reads and writes the active profile, and moves the age tick only once the server took it", async () => {
    mount();
    expect(api.getArgs).toEqual({ id: "p-active", enabled: true });
    expect(ageButton("12+").textContent).toContain("✓");

    const spy = vi.spyOn(qc, "invalidateQueries");
    fireEvent.click(screen.getByRole("switch"));
    await waitFor(() => expect(api.update).toHaveBeenCalledWith("p-active", { enabled: false }));
    await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: ["parental", "p-active"] }));

    api.update.mockRejectedValueOnce(new Error("locked"));
    fireEvent.click(ageButton("16+"));
    await waitFor(() => expect(alerts.calls.at(-1)).toEqual(["Error", "locked"]));
    expect(ageButton("12+").textContent).toContain("✓");
    expect(ageButton("16+").textContent).not.toContain("✓");

    fireEvent.click(ageButton("No restriction"));
    await waitFor(() => expect(api.update).toHaveBeenCalledWith("p-active", { unsetMaxAge: true }));
    await waitFor(() => expect(ageButton("No restriction").textContent).toContain("✓"));
  });

  it("a first PIN needs the account password, digits only, four of them, matching", async () => {
    mount();
    const save = screen.getByText("Enregistrer le PIN").closest("button") as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    const pin = screen.getByLabelText("Nouveau PIN") as HTMLInputElement;
    fireEvent.change(pin, { target: { value: "12a345" } });
    expect(pin.value).toBe("1234");
    fireEvent.change(screen.getByLabelText("Confirmer le PIN"), { target: { value: "1235" } });
    fireEvent.change(screen.getByLabelText("Mot de passe du compte"), { target: { value: "pw" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(alerts.calls.at(-1)).toEqual(["Les PIN ne correspondent pas"]);
    expect(api.update).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Confirmer le PIN"), { target: { value: "1234" } });
    fireEvent.click(save);
    await waitFor(() => expect(api.update).toHaveBeenCalledWith("p-active", { newPin: "1234", currentPassword: "pw" }));
    await waitFor(() => expect(alerts.calls.at(-1)).toEqual(["PIN enregistré"]));
    expect(pin.value).toBe("");
  });

  it("a rotation needs the current PIN instead, and nothing is written without an active profile", async () => {
    api.parental = { enabled: true, hasPin: true, maxAge: null };
    mount();
    expect(screen.queryByLabelText("Mot de passe du compte")).toBeNull();
    fireEvent.change(screen.getByLabelText("Code PIN actuel"), { target: { value: "0000" } });
    fireEvent.change(screen.getByLabelText("Nouveau PIN"), { target: { value: "9876" } });
    fireEvent.change(screen.getByLabelText("Confirmer le PIN"), { target: { value: "9876" } });
    fireEvent.click(screen.getByText("Mettre à jour le code PIN"));
    await waitFor(() => expect(api.update).toHaveBeenCalledWith("p-active", { newPin: "9876", currentPin: "0000" }));

    active.profile = undefined;
    api.update.mockClear();
    const second = mount();
    fireEvent.click(screen.getAllByRole("switch").at(-1)!);
    fireEvent.click(screen.getAllByText("12+").at(-1)!.closest("button")!);
    await new Promise((r) => setTimeout(r, 10));
    expect(api.update).not.toHaveBeenCalled();
    second.unmount();
  });
});
