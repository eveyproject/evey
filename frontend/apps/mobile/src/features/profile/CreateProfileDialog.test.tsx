import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => null,
  Switch: ({ value, onValueChange }: any) =>
    React.createElement("input", { type: "checkbox", "aria-label": "kids", checked: value, onChange: (e: React.ChangeEvent<HTMLInputElement>) => onValueChange(e.target.checked) }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  TextInput: ({ value, onChangeText, placeholder, accessibilityLabel }: any) =>
    React.createElement("input", { "aria-label": accessibilityLabel, placeholder, value: value ?? "", onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => (typeof fallback === "string" ? fallback : _k), i18n: { language: "fr" } }) }));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { overlay: "#0008", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", bg: "#000", border: "#333", error: "#f33", accent: "#f00", onAccent: "#000" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../../components/BottomSheet", () => ({ BottomSheet: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null) }));
vi.mock("../../i18n", () => ({ supportedLanguages: [{ code: "fr", label: "Français" }] }));
vi.mock("../../lib/regionOptions", () => ({ regionOptions: () => [{ code: "FR", label: "France" }] }));
const mut = vi.hoisted(() => ({ mutate: vi.fn(), opts: null as any }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceCreateProfile: (opts: any) => {
    mut.opts = opts;
    return { mutate: mut.mutate, isPending: false };
  },
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));

import { CreateProfileDialog } from "./CreateProfileDialog";

function open(hasKids = false) {
  const onClose = vi.fn();
  render(<CreateProfileDialog open onClose={onClose} userId="u1" hasKidsProfile={hasKids} />);
  return onClose;
}
const type = (label: string, v: string) => fireEvent.change(screen.getByLabelText(label), { target: { value: v } });
const save = () => fireEvent.click(screen.getByLabelText("Save"));

beforeEach(() => vi.clearAllMocks());

// A new profile needs a name; once a kids profile exists every NON-kids
// profile needs a 4-digit PIN (the kids invariant); a PIN typed before
// ticking "Kids" must not leak onto the kids profile; the max age only
// travels with a kids profile; and success refreshes the list and closes.
describe("CreateProfileDialog (mobile)", () => {
  it("refuses an empty name and, with a kids profile around, a missing PIN", () => {
    open(true);
    save();
    expect(screen.getByText("Name is required")).toBeTruthy();
    expect(mut.mutate).not.toHaveBeenCalled();
    type("Name", "  Salon ");
    save();
    expect(screen.getAllByText("A PIN is required because a kids profile exists.").length).toBeGreaterThan(0);
    expect(mut.mutate).not.toHaveBeenCalled();
    type("PIN (4 digits)", "1234");
    save();
    expect(mut.mutate).toHaveBeenCalledWith({ userId: "u1", data: { name: "Salon", pin: "1234" } });
  });

  it("a kids profile never carries the PIN typed before the tick, and the PIN field only shows when one is required", () => {
    open(true);
    type("Name", "Kid");
    type("PIN (4 digits)", "12a34");
    expect((screen.getByLabelText("PIN (4 digits)") as HTMLInputElement).value).toBe("1234");
    fireEvent.click(screen.getByLabelText("kids"));
    expect(screen.queryByLabelText("PIN (4 digits)")).toBeNull();
    save();
    expect(mut.mutate).toHaveBeenCalledWith({ userId: "u1", data: { name: "Kid", isKids: true } });

    vi.clearAllMocks();
    open(false);
    expect(screen.queryByLabelText("PIN (4 digits)")).toBeNull();
  });

  it("success refreshes the profile list and closes; a failure is reported", () => {
    const onClose = open(false);
    type("Name", "Salon");
    save();
    mut.opts.mutation.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
    expect(toast.success).toHaveBeenCalledWith("Profile created");
    expect(onClose).toHaveBeenCalled();
    mut.opts.mutation.onError(new Error("name taken"));
    expect(toast.error).toHaveBeenCalledWith("name taken");
  });
});
