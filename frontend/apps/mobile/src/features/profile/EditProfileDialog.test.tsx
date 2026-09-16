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
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => (typeof fallback === "string" ? fallback : _k), i18n: { language: "fr" } }) }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: vi.fn() }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { overlay: "#0008", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", bg: "#000", border: "#333", error: "#f33", accent: "#f00", onAccent: "#000" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../../components/BottomSheet", () => ({ BottomSheet: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null) }));
vi.mock("../../i18n", () => ({ supportedLanguages: [{ code: "fr", label: "Français" }], applyEditedProfileLocale: vi.fn() }));
vi.mock("../../lib/regionOptions", () => ({ regionOptions: () => [{ code: "FR", label: "France" }] }));
const mutateMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceUpdateProfile: () => ({ mutate: mutateMock, isPending: false }),
  getUserServiceListProfilesQueryKey: (id: string) => ["/api/v1/users", id, "profiles"],
}));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { EditProfileDialog } from "./EditProfileDialog";

const profile = { id: "p1", name: "Alice", isKids: false, locale: "", region: "" } as any;
const open = (over: Partial<typeof profile> = {}, hasKids = false) =>
  render(<EditProfileDialog open onClose={vi.fn()} profile={{ ...profile, ...over }} userId="u1" hasKidsProfile={hasKids} />);
const save = () => fireEvent.click(screen.getByLabelText("Save"));

beforeEach(() => {
  vi.clearAllMocks();
});

describe("EditProfileDialog (mobile)", () => {
  it("sends only what changed, and nothing at all when nothing did", () => {
    const onClose = vi.fn();
    render(<EditProfileDialog open onClose={onClose} profile={profile} userId="u1" hasKidsProfile={false} />);
    save();
    expect(mutateMock).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "  Alicia " } });
    save();
    expect(mutateMock).toHaveBeenCalledWith({ id: "p1", data: { name: "Alicia" } });
  });

  it("refuses an empty name and a short PIN without calling the server", () => {
    open();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "   " } });
    save();
    expect(screen.getByText("Name is required")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Alice" } });
    fireEvent.change(screen.getByLabelText("PIN (4 digits)"), { target: { value: "12x" } });
    save();
    expect(screen.getByText("PIN must be 4 digits")).toBeTruthy();
    expect(mutateMock).not.toHaveBeenCalled();
  });

  it("carries a 4-digit PIN, the kids switch and the age cap (blank = clear sentinel)", () => {
    const first = open({ isKids: true, maxAge: 12 });
    fireEvent.change(screen.getByLabelText("PIN (4 digits)"), { target: { value: "4321" } });
    fireEvent.click(screen.getByLabelText("Default (12+)"));
    save();
    expect(mutateMock).toHaveBeenCalledWith({ id: "p1", data: { pin: "4321", maxAge: -1 } });
    first.unmount();

    mutateMock.mockClear();
    open({ isKids: true, maxAge: 12 });
    fireEvent.click(screen.getByLabelText("16+"));
    save();
    expect(mutateMock).toHaveBeenCalledWith({ id: "p1", data: { maxAge: 16 } });
  });

  it("explains why a PIN is required when another profile is a kids profile", () => {
    open({}, true);
    expect(screen.getByText("A PIN is required because a kids profile exists.")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("kids"));
    expect(screen.queryByText("A PIN is required because a kids profile exists.")).toBeNull();
    save();
    expect(mutateMock).toHaveBeenCalledWith({ id: "p1", data: { isKids: true } });
  });
});
