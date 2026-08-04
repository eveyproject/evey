import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A failed playlist fetch used to render as "you have no playlists yet",
// which invited the user to create one that already existed - the create
// flow further down produced a real duplicate.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      { "data-testid": "playlist-list" },
      (data ?? []).map((item: any) =>
        React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
      ),
    ),
  TouchableOpacity: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "playlist-loading", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", onAccent: "#fff", overlay: "rgba(0,0,0,.5)" },
    radii: { lg: 12 },
  }),
}));

// --- ../api/playlists --------------------------------------------------------------------
let playlistListState: { data?: unknown[]; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("../api/playlists", () => ({
  usePlaylistList: () => playlistListState,
  useAddToPlaylist: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useCreatePlaylist: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
}));

import { AddToPlaylistSheet } from "./AddToPlaylistSheet";

beforeEach(() => {
  vi.clearAllMocks();
  playlistListState = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
});

describe("AddToPlaylistSheet - loading vs failure vs genuinely no playlists", () => {
  it("shows a spinner while the playlist list is loading", () => {
    playlistListState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<AddToPlaylistSheet visible mediaItemId="m1" onClose={vi.fn()} />);

    expect(screen.getByTestId("playlist-loading")).toBeTruthy();
    expect(screen.queryByText("common.noManualPlaylists")).toBeNull();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });

  it("shows LoadFailed with a retry - not 'no playlists' - when the fetch fails", () => {
    const refetch = vi.fn();
    playlistListState = { data: undefined, isLoading: false, isError: true, refetch };
    render(<AddToPlaylistSheet visible mediaItemId="m1" onClose={vi.fn()} />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("common.noManualPlaylists")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("shows 'no playlists yet' only once the list is confirmed empty", () => {
    playlistListState = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
    render(<AddToPlaylistSheet visible mediaItemId="m1" onClose={vi.fn()} />);

    expect(screen.getByText("common.noManualPlaylists")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
    expect(screen.queryByTestId("playlist-loading")).toBeNull();
  });
});
