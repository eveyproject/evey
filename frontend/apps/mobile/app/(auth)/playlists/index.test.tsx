import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// A new playlist needs a non-blank name, sends the trimmed name and a
// description only when given, the chosen type, then opens the new list and
// resets the form; a row opens its list; a delete is confirmed by name and a
// refused one is told; loading, failure (retry) and "none" are kept apart.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement("div", null, (data ?? []).length === 0 ? ListEmptyComponent : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Modal: ({ visible, children }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", { value, placeholder, onChange: (e: any) => onChangeText(e.target.value) }),
  TouchableOpacity: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: (e: any) => { e.stopPropagation(); onPress?.(e); }, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message: string, buttons?: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({
  Stack: { Screen: ({ options }: any) => React.createElement("div", null, options.headerRight?.()) },
  useRouter: () => router,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: string | { name?: string }) => (typeof o === "string" ? o : o?.name ? `${k}:${o.name}` : k) }),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", danger: "#f33" }, radii: { md: 8, sm: 4 }, spacing: { sm: 8, md: 16 } }),
}));
vi.mock("../../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
const api = vi.hoisted(() => ({
  list: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  create: vi.fn(), del: vi.fn(),
}));
vi.mock("../../../src/api/playlists", () => ({
  usePlaylistList: () => api.list,
  useCreatePlaylist: () => ({ mutateAsync: api.create, isPending: false }),
  useDeletePlaylist: () => ({ mutateAsync: api.del }),
}));

import { useDemoStore } from "@evey/store";
import PlaylistsScreen from "./index";

const lists = [
  { id: "pl-1", name: "Mix", type: "manual", item_count: 3, created_at: "2026-09-01T00:00:00Z" },
  { id: "pl-2", name: "Auto", type: "smart", item_count: 0, created_at: "2026-09-02T00:00:00Z" },
];

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  api.list = { data: lists, isLoading: false, isError: false, refetch: vi.fn() };
  api.create.mockResolvedValue({ id: "pl-new" });
  api.del.mockResolvedValue({});
  useDemoStore.getState().setDemoMode(false);
});

describe("playlists screen (mobile)", () => {
  it("creates with a trimmed name, an optional description and the chosen type, then opens the new list", async () => {
    render(<PlaylistsScreen />);
    fireEvent.click(screen.getByRole("button", { name: "media.playlist.createPlaylist" }));
    const create = screen.getByText("common.create").closest("button") as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.change(screen.getByPlaceholderText("media.playlist.namePlaceholder"), { target: { value: "  Road trip " } });
    fireEvent.change(screen.getByPlaceholderText("media.playlist.descriptionPlaceholder"), { target: { value: "   " } });
    fireEvent.click(screen.getByText("media.playlist.typeSmart"));
    expect(create.disabled).toBe(false);
    fireEvent.click(create);
    await waitFor(() => expect(router.push).toHaveBeenCalledWith("/playlists/pl-new"));
    expect(api.create).toHaveBeenCalledWith({ name: "Road trip", description: undefined, type: "smart" });
    expect(screen.queryByTestId("modal")).toBeNull();
    // The form is reset for the next one.
    fireEvent.click(screen.getByRole("button", { name: "media.playlist.createPlaylist" }));
    expect((screen.getByPlaceholderText("media.playlist.namePlaceholder") as HTMLInputElement).value).toBe("");

    api.create.mockRejectedValueOnce(new Error("409"));
    fireEvent.change(screen.getByPlaceholderText("media.playlist.namePlaceholder"), { target: { value: "Dup" } });
    fireEvent.click(screen.getByText("common.create"));
    await waitFor(() => expect(alerts.last?.message).toBe("media.playlist.createError"));
  });

  it("opens a list on tap, confirms a delete by name, and tells a refused delete", async () => {
    render(<PlaylistsScreen />);
    fireEvent.click(screen.getByText("Mix").closest("button")!);
    expect(router.push).toHaveBeenCalledWith("/playlists/pl-1");
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1]);
    expect(alerts.last.message).toBe("media.playlist.confirmDelete:Auto");
    expect(api.del).not.toHaveBeenCalled();
    await alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.del).toHaveBeenCalledWith("pl-2");

    api.del.mockRejectedValueOnce(new Error("403"));
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[0]);
    await alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    await waitFor(() => expect(alerts.last.message).toBe("media.playlist.deleteError"));
  });

  // The demo's playlists are the shared account's and the server refuses
  // them: create and delete are greyed there, a press reaches nothing.
  it("greys create and delete on the demo, and still opens a list", () => {
    useDemoStore.getState().setDemoMode(true);
    render(<PlaylistsScreen />);
    const create = screen.getByRole("button", { name: "media.playlist.createPlaylist" }) as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.click(create);
    expect(screen.queryByTestId("modal")).toBeNull();
    for (const del of screen.getAllByRole("button", { name: "Delete" }) as HTMLButtonElement[]) {
      expect(del.disabled).toBe(true);
      fireEvent.click(del);
    }
    expect(alerts.last).toBeNull();
    expect(api.del).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("Mix").closest("button")!);
    expect(router.push).toHaveBeenCalledWith("/playlists/pl-1");
  });

  it("keeps loading, failure and none apart", () => {
    api.list = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    const first = render(<PlaylistsScreen />);
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    first.unmount();

    api.list = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    const second = render(<PlaylistsScreen />);
    fireEvent.click(screen.getByText("retry"));
    expect(api.list.refetch).toHaveBeenCalledTimes(1);
    second.unmount();

    api.list = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
    render(<PlaylistsScreen />);
    expect(screen.getByText("common.noPlaylists")).toBeTruthy();
  });
});
