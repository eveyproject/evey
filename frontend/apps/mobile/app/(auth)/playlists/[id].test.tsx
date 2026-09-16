import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// A manual playlist's rows can be reordered (a swap renumbers every item and
// is sent whole) and removed after a confirmation; a smart one has neither,
// only its rules editor whose save writes the rules; deleting the playlist
// is confirmed by name and goes back; a failed fetch is a retry and an
// absent playlist a plain notice.
const alerts = vi.hoisted(() => ({ last: null as any }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement("div", null, (data ?? []).length === 0 ? ListEmptyComponent : (data as any[]).map((item, index) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item, index })))),
  TouchableOpacity: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: (e: any) => { e.stopPropagation(); onPress?.(e); }, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (title: string, message: string, buttons: any[]) => { alerts.last = { title, message, buttons }; } },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn(), back: vi.fn() }));
vi.mock("expo-router", () => ({
  Stack: { Screen: ({ options }: any) => React.createElement("div", null, options.headerRight?.()) },
  useLocalSearchParams: () => ({ id: "pl-1" }),
  useRouter: () => router,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o?.name ? `${k}:${o.name}` : k) }),
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", danger: "#f33" }, radii: { md: 8, sm: 4 } }),
}));
vi.mock("../../../src/components/LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));
vi.mock("../../../src/components/SmartRulesEditor", () => ({
  SmartRulesEditor: ({ visible, onSave }: any) => (visible ? React.createElement("button", { onClick: () => onSave([{ field: "genre", operator: "eq", value: "Drama" }]) }, "save-rules") : null),
}));
const api = vi.hoisted(() => ({
  detail: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  del: vi.fn(), remove: vi.fn(), reorder: vi.fn(), update: vi.fn(),
}));
vi.mock("../../../src/api/playlists", () => ({
  usePlaylistDetail: () => api.detail,
  useDeletePlaylist: () => ({ mutateAsync: api.del }),
  useRemoveFromPlaylist: () => ({ mutate: api.remove }),
  useReorderPlaylistItems: () => ({ mutate: api.reorder }),
  useUpdatePlaylist: () => ({ mutate: api.update }),
}));

import PlaylistDetailScreen from "./[id]";

const manual = { id: "pl-1", name: "Mix", type: "manual", items: [{ media_item_id: "m1", title: "One" }, { media_item_id: "m2", title: "Two" }, { media_item_id: "m3", title: "Three" }] };

beforeEach(() => {
  vi.clearAllMocks();
  alerts.last = null;
  api.detail = { data: manual, isLoading: false, isError: false, refetch: vi.fn() };
  api.del.mockResolvedValue({});
});

describe("playlist detail screen (mobile)", () => {
  it("reorders by swapping and renumbering every row, removes after confirmation, opens a title", () => {
    render(<PlaylistDetailScreen />);
    const ups = screen.getAllByRole("button", { name: "media.playlist.moveItemUp" }) as HTMLButtonElement[];
    const downs = screen.getAllByRole("button", { name: "media.playlist.moveItemDown" }) as HTMLButtonElement[];
    expect(ups[0].disabled).toBe(true);
    expect(downs[2].disabled).toBe(true);
    fireEvent.click(downs[0]);
    expect(api.reorder).toHaveBeenCalledWith({
      playlistId: "pl-1",
      items: [{ mediaItemId: "m2", sortOrder: 0 }, { mediaItemId: "m1", sortOrder: 1 }, { mediaItemId: "m3", sortOrder: 2 }],
    });

    fireEvent.click(screen.getAllByRole("button", { name: "media.playlist.removeItem" })[2]);
    expect(api.remove).not.toHaveBeenCalled();
    alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.remove).toHaveBeenCalledWith({ playlistId: "pl-1", mediaItemId: "m3" });

    fireEvent.click(screen.getByRole("button", { name: "Two" }));
    expect(router.push).toHaveBeenCalledWith("/media/m2");
  });

  it("deletes the playlist after a confirmation by name and goes back; a refused delete is told", async () => {
    render(<PlaylistDetailScreen />);
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    expect(alerts.last.message).toBe("media.playlist.confirmDelete:Mix");
    await alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    expect(api.del).toHaveBeenCalledWith("pl-1");
    expect(router.back).toHaveBeenCalledTimes(1);

    api.del.mockRejectedValueOnce(new Error("403"));
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    await alerts.last.buttons.find((b: any) => b.style === "destructive").onPress();
    await waitFor(() => expect(alerts.last.title).toBe("common.error"));
    expect(router.back).toHaveBeenCalledTimes(1);
  });

  it("a smart playlist has no reorder or remove, only its rules editor", () => {
    api.detail.data = { ...manual, type: "smart", rules: [{ field: "year", operator: "gte", value: "2000" }] };
    render(<PlaylistDetailScreen />);
    expect(screen.queryByRole("button", { name: "media.playlist.moveItemUp" })).toBeNull();
    expect(screen.queryByRole("button", { name: "media.playlist.removeItem" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "media.playlist.editRules" }));
    fireEvent.click(screen.getByText("save-rules"));
    expect(api.update).toHaveBeenCalledWith({ id: "pl-1", rules: [{ field: "genre", operator: "eq", value: "Drama" }] });
  });

  it("keeps failure, absence and loading apart", () => {
    api.detail = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    const first = render(<PlaylistDetailScreen />);
    expect(screen.queryByText("media.playlist.notFound")).toBeNull();
    fireEvent.click(screen.getByText("retry"));
    expect(api.detail.refetch).toHaveBeenCalledTimes(1);
    first.unmount();

    api.detail = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
    const second = render(<PlaylistDetailScreen />);
    expect(screen.getByText("media.playlist.notFound")).toBeTruthy();
    second.unmount();

    api.detail = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<PlaylistDetailScreen />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
  });
});
