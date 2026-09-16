import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: () => React.createElement("img", { alt: "" }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn(), replace: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
const secure = vi.hoisted(() => ({ setItemAsync: vi.fn().mockResolvedValue(undefined) }));
vi.mock("expo-secure-store", () => secure);
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => (typeof fallback === "string" ? fallback : _k) }) }));
const qc = vi.hoisted(() => ({ clear: vi.fn() }));
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => qc }));
vi.mock("@evey/api", () => ({ apiErrorMessage: (e: Error, _t: unknown, f: string) => e.message || f }));
const lib = vi.hoisted(() => ({ signOutDevice: vi.fn().mockResolvedValue(undefined), switchProfileToken: vi.fn().mockResolvedValue(undefined) }));
vi.mock("../../lib/sign-out", () => ({ signOutDevice: (...a: unknown[]) => lib.signOutDevice(...a) }));
vi.mock("../../lib/profile-switch", () => ({ switchProfileToken: (...a: unknown[]) => lib.switchProfileToken(...a) }));
const store = vi.hoisted(() => ({ toast: { success: vi.fn(), error: vi.fn() }, logout: vi.fn(), isAdmin: false }));
vi.mock("@evey/store", () => ({
  toast: store.toast,
  useAuthStore: (sel: (s: { accessToken: string; isAdmin: boolean; logout: () => void }) => unknown) => sel({ accessToken: "tok", isAdmin: store.isAdmin, logout: store.logout }),
}));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", onAccent: "#fff", bg: "#000", overlay: "rgba(0,0,0,.5)", error: "#f33" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
vi.mock("../../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("../../features/profile/PinPromptDialog", () => ({
  PinPromptDialog: ({ target, onVerified, onCancel }: any) =>
    React.createElement("div", null, `pin-for:${target.id}`, React.createElement("button", { onClick: () => onVerified(target, "pin-token") }, "verify"), React.createElement("button", { onClick: onCancel }, "cancel-pin")),
}));

import { ProfileSwitcher } from "./ProfileSwitcher";

const profiles = [
  { id: "p-adult", name: "Salon", isDefault: true },
  { id: "p-kid", name: "Kid", isKids: true },
  { id: "p-other", name: "Other" },
] as any[];

async function openSwitch() {
  fireEvent.click(screen.getByLabelText("User menu"));
  fireEvent.click(screen.getByLabelText("Switch profile"));
  await waitFor(() => expect(screen.getByLabelText("Other")).toBeTruthy());
}

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  store.isAdmin = false;
  lib.switchProfileToken.mockResolvedValue(undefined);
  lib.signOutDevice.mockResolvedValue(undefined);
});

// Switching profiles drops the WHOLE query cache (never a stale kids-filtered
// screen), leaving a kids profile asks for the PIN first, a 403 from the
// server means "prove the PIN" and retries with it, and sign-out clears the
// persisted refresh token loudly when it cannot.
describe("ProfileSwitcher", () => {
  it("switches to another adult profile, remembers it, and clears the cache", async () => {
    render(<ProfileSwitcher profiles={profiles} activeProfileId="p-adult" />);
    await openSwitch();
    fireEvent.click(screen.getByLabelText("Other"));
    await waitFor(() => expect(lib.switchProfileToken).toHaveBeenCalledWith("p-other", undefined));
    await waitFor(() => expect(qc.clear).toHaveBeenCalled());
    expect(secure.setItemAsync).toHaveBeenCalledWith("evey:last_profile_id", "p-other");
    expect(screen.queryByText(/pin-for/)).toBeNull();
  });

  it("leaving a kids profile asks for the PIN before any switch, and the PIN proof completes it", async () => {
    render(<ProfileSwitcher profiles={profiles} activeProfileId="p-kid" />);
    await openSwitch();
    fireEvent.click(screen.getByLabelText("Salon"));
    expect(await screen.findByText("pin-for:p-adult")).toBeTruthy();
    expect(lib.switchProfileToken).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("verify"));
    await waitFor(() => expect(lib.switchProfileToken).toHaveBeenCalledWith("p-adult", "pin-token"));
  });

  it("a 403 on the switch means the server wants the PIN; any other failure is reported", async () => {
    lib.switchProfileToken.mockRejectedValueOnce(Object.assign(new Error("forbidden"), { httpStatus: 403 }));
    render(<ProfileSwitcher profiles={profiles} activeProfileId="p-adult" />);
    await openSwitch();
    fireEvent.click(screen.getByLabelText("Other"));
    expect(await screen.findByText("pin-for:p-other")).toBeTruthy();
    expect(store.toast.error).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("cancel-pin"));
    await waitFor(() => expect(screen.queryByText(/pin-for/)).toBeNull());

    vi.spyOn(console, "warn").mockImplementation(() => {});
    lib.switchProfileToken.mockRejectedValueOnce(Object.assign(new Error("boom"), { httpStatus: 500 }));
    fireEvent.click(screen.getByLabelText("Other"));
    await waitFor(() => expect(store.toast.error).toHaveBeenCalledWith("boom"));
    expect(screen.queryByText(/pin-for/)).toBeNull();
  });

  it("signs out: cache cleared, session dropped, refresh token wiped - loudly when it cannot be", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    lib.signOutDevice.mockRejectedValueOnce(new Error("keychain locked"));
    render(<ProfileSwitcher profiles={profiles} activeProfileId="p-adult" />);
    fireEvent.click(screen.getByLabelText("User menu"));
    expect(screen.queryByLabelText("Admin")).toBeNull();
    fireEvent.click(screen.getByLabelText("Sign out"));
    expect(qc.clear).toHaveBeenCalled();
    expect(store.logout).toHaveBeenCalled();
    expect(lib.signOutDevice).toHaveBeenCalled();
    await waitFor(() => expect(store.toast.error).toHaveBeenCalledWith(expect.stringContaining("Sign-out may be incomplete")));
    await waitFor(() => expect(router.replace).toHaveBeenCalledWith("/login"));
  });

  it("offers the admin entry to an admin and navigates after closing the menu", async () => {
    store.isAdmin = true;
    render(<ProfileSwitcher profiles={profiles} activeProfileId="p-adult" />);
    fireEvent.click(screen.getByLabelText("User menu"));
    fireEvent.click(screen.getByLabelText("Admin"));
    await waitFor(() => expect(router.push).toHaveBeenCalledWith("/admin"));
  });
});
