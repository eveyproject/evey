import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A playlist that fails to load is not a playlist that does not exist. This
// screen used to render "playlist not found" for both, so a server that did not
// answer told the user their playlist was gone - with nothing to retry. The web
// twin carried the exact same defect and was fixed the same day.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).map((item: any) =>
        React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
      ),
    ),
  TouchableOpacity: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
  Alert: { alert: vi.fn() },
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  TextInput: () => React.createElement("input"),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => React.createElement("span") }));
vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
  useLocalSearchParams: () => ({ id: "pl-1" }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222" },
    radii: { sm: 4, md: 8 },
    spacing: { md: 8, xl: 16 },
  }),
}));
vi.mock("../../../src/components/SmartRulesEditor", () => ({ SmartRulesEditor: () => null }));

let detailState: { data?: unknown; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("../../../src/api/playlists", () => ({
  usePlaylistDetail: () => detailState,
  useDeletePlaylist: () => ({ mutate: vi.fn(), isPending: false }),
  useRemoveFromPlaylist: () => ({ mutate: vi.fn(), isPending: false }),
  useReorderPlaylistItems: () => ({ mutate: vi.fn(), isPending: false }),
  useUpdatePlaylist: () => ({ mutate: vi.fn(), isPending: false }),
}));

import PlaylistDetailScreen from "./[id]";

beforeEach(() => {
  vi.clearAllMocks();
  detailState = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("Mobile playlist detail - a failed load is not a missing playlist", () => {
  it("offers a retry and never claims the playlist is missing when the fetch fails", () => {
    const refetch = vi.fn();
    detailState = { data: undefined, isLoading: false, isError: true, refetch };

    render(<PlaylistDetailScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    // The lie the fix removes.
    expect(screen.queryByText("media.playlist.notFound")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("still says 'not found' when the server answered and the playlist is genuinely absent", () => {
    detailState = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };

    render(<PlaylistDetailScreen />);

    expect(screen.getByText("media.playlist.notFound")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });
});
