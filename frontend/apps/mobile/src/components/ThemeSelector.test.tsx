import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "aria-selected": String(!!accessibilityState?.selected) }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => (typeof f === "string" ? f : _k) }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { accent: "#f00", surfaceRaised: "#222", border: "#333", text: "#fff", onAccent: "#000" }, spacing: { xs: 4, sm: 8, md: 12 }, radii: { md: 8 } }),
}));
vi.mock("@evey/theme", () => ({
  themes: { default: { colors: { accent: "#111" } }, amoled: { colors: { accent: "#000" } } },
  FONT_SIZE: { sm: 12 },
}));
const api = vi.hoisted(() => ({ fetch: vi.fn(), update: vi.fn(), user: { data: { id: "u1" } }, profiles: { data: undefined as unknown } }));
vi.mock("@evey/api", () => ({
  eveyFetch: (...a: unknown[]) => api.fetch(...a),
  useUserServiceGetCurrentUser: () => api.user,
  useUserServiceListProfiles: () => api.profiles,
  useUserServiceUpdateProfile: () => ({ mutate: api.update }),
}));
const store = vi.hoisted(() => ({ theme: "default", setTheme: vi.fn(), applyFromPreferences: vi.fn(), profileId: "p2" }));
vi.mock("@evey/store", () => ({
  useThemeStore: (sel: (s: { theme: string; setTheme: () => void; applyFromPreferences: () => void }) => unknown) =>
    sel({ theme: store.theme, setTheme: store.setTheme, applyFromPreferences: store.applyFromPreferences }),
  useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: store.profileId }),
  themePreferences: (current: Record<string, string> | undefined, theme: string, pluginId?: string) => ({ ...current, theme, ...(pluginId ? { theme_plugin: pluginId } : {}) }),
}));

import { ThemeSelector } from "./ThemeSelector";

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(ThemeSelector)));
}

beforeEach(() => {
  vi.clearAllMocks();
  api.fetch.mockResolvedValue({ themes: [{ id: "plg-1", name: "Neon" }] });
  api.profiles = { data: { profiles: [{ id: "p1", isDefault: true, preferences: { theme: "amoled" } }, { id: "p2", preferences: { theme: "default", audio: "fr" } }] } };
  store.theme = "default";
  store.profileId = "p2";
});

// The theme belongs to the ACTIVE profile: picking one applies it locally and
// writes it MERGED into that profile's preferences (never wiping the audio
// language sitting beside it); plugin themes come from the server and carry
// their plugin id; the pills re-sync from the profile's stored preferences.
describe("ThemeSelector", () => {
  it("lists built-in and plugin themes and marks the active one", async () => {
    mount();
    expect(await screen.findByLabelText("Neon")).toBeTruthy();
    expect(screen.getByLabelText("Dark").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByLabelText("AMOLED").getAttribute("aria-selected")).toBe("false");
    await waitFor(() => expect(store.applyFromPreferences).toHaveBeenCalledWith({ theme: "default", audio: "fr" }));
  });

  it("a pick applies locally and merges into the active profile's preferences", async () => {
    mount();
    await screen.findByLabelText("Neon");
    fireEvent.click(screen.getByLabelText("AMOLED"));
    expect(store.setTheme).toHaveBeenCalledWith("amoled", undefined);
    expect(api.update).toHaveBeenCalledWith({ id: "p2", data: { preferences: { theme: "amoled", audio: "fr" } } });
    fireEvent.click(screen.getByLabelText("Neon"));
    expect(store.setTheme).toHaveBeenCalledWith("plugin:Neon", "plg-1");
    expect(api.update).toHaveBeenLastCalledWith({ id: "p2", data: { preferences: { theme: "plugin:Neon", audio: "fr", theme_plugin: "plg-1" } } });
  });

  it("falls back to the default profile when the active id is unknown, and applies locally only without a profile", async () => {
    store.profileId = "ghost";
    mount();
    await screen.findByLabelText("Neon");
    fireEvent.click(screen.getByLabelText("AMOLED"));
    expect(api.update).toHaveBeenCalledWith({ id: "p1", data: { preferences: { theme: "amoled" } } });

    vi.clearAllMocks();
    api.profiles = { data: { profiles: [] } };
    mount();
    fireEvent.click(screen.getAllByLabelText("AMOLED").at(-1)!);
    expect(store.setTheme).toHaveBeenCalledWith("amoled", undefined);
    expect(api.update).not.toHaveBeenCalled();
  });
});
