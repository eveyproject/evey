import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: () => React.createElement("img", { alt: "" }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, fallback?: string, opts?: { name?: string }) => (typeof fallback === "string" ? fallback.replace("{{name}}", opts?.name ?? "") : _k) }),
}));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", onAccent: "#fff", danger: "#f33", onDanger: "#fff", overlay: "rgba(0,0,0,.5)", success: "#0a0" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("./EditProfileDialog", () => ({ EditProfileDialog: ({ open, profile }: any) => (open ? React.createElement("div", null, `editing:${profile?.id}`) : null) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }) }));
const mut = vi.hoisted(() => ({ del: vi.fn(), setDef: vi.fn(), delOpts: null as any, setDefOpts: null as any }));
vi.mock("@evey/api", () => ({
  useUserServiceDeleteProfile: (opts: any) => {
    mut.delOpts = opts;
    return { mutate: mut.del, isPending: false };
  },
  useUserServiceSetDefaultProfile: (opts: any) => {
    mut.setDefOpts = opts;
    return { mutate: mut.setDef, isPending: false, variables: undefined };
  },
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
  apiErrorMessage: (e: Error, _t: unknown, fallback: string) => e.message || fallback,
}));

import { ProfileManagerList } from "./ProfileManagerList";

const profiles = [
  { id: "p1", name: "Salon", isDefault: true },
  { id: "p2", name: "Kid", isKids: true },
] as any[];

beforeEach(() => {
  vi.clearAllMocks();
});

// The default profile can never be deleted (its button is dead), a delete is
// confirmed by name first, and "set as default" only appears on non-default
// profiles when the caller is on the admin flow the RPC is gated to.
describe("ProfileManagerList", () => {
  it("guards the default profile, confirms a delete by name, and refreshes the list", () => {
    render(<ProfileManagerList userId="u1" profiles={profiles} />);
    expect(screen.getByText("Default")).toBeTruthy();
    expect(screen.getByText("Kids")).toBeTruthy();
    const deletes = screen.getAllByLabelText("Delete") as HTMLButtonElement[];
    expect(deletes[0].disabled).toBe(true);
    expect(screen.queryByLabelText("Set as default")).toBeNull();

    fireEvent.click(deletes[1]);
    expect(screen.getByText('Delete profile "Kid"? This cannot be undone.')).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Cancel"));
    expect(screen.queryByTestId("modal")).toBeNull();
    expect(mut.del).not.toHaveBeenCalled();

    fireEvent.click(screen.getAllByLabelText("Delete")[1]);
    const confirm = screen.getAllByLabelText("Delete").at(-1)!;
    fireEvent.click(confirm);
    expect(mut.del).toHaveBeenCalledWith({ id: "p2" });
    mut.delOpts.mutation.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
    expect(toast.success).toHaveBeenCalledWith("Profile deleted");
    mut.delOpts.mutation.onError(new Error("kids profile needs a PIN"));
    expect(toast.error).toHaveBeenCalledWith("kids profile needs a PIN");
  });

  it("opens the editor for a profile and offers set-as-default on the admin flow only", () => {
    render(<ProfileManagerList userId="u1" profiles={profiles} canSetDefault />);
    expect(screen.getAllByLabelText("Set as default")).toHaveLength(1);
    fireEvent.click(screen.getByLabelText("Set as default"));
    expect(mut.setDef).toHaveBeenCalledWith({ id: "p2", data: {} });
    mut.setDefOpts.mutation.onSuccess();
    expect(toast.success).toHaveBeenCalledWith("Default profile updated");
    fireEvent.click(screen.getAllByLabelText("Edit")[0]);
    expect(screen.getByText("editing:p1")).toBeTruthy();
  });
});
