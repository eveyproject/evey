import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement("div", null, (data ?? []).map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  TouchableOpacity: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  Pressable: ({ children, onPress }: any) => React.createElement("div", { onClick: onPress }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, opts?: unknown) => (opts && typeof opts === "object" && "count" in (opts as object) ? `${key}:${(opts as { count: number }).count}` : key) }),
}));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", overlay: "rgba(0,0,0,.5)" }, radii: { md: 8, lg: 12 } }),
}));
vi.mock("../LoadFailed", () => ({ LoadFailed: ({ onRetry }: any) => React.createElement("button", { onClick: onRetry }, "retry") }));

const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
const addCollectionItem = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ toast }));
vi.mock("@evey/api", () => ({ collectionServiceAddCollectionItem: (...a: unknown[]) => addCollectionItem(...a) }));

const playlists = vi.hoisted(() => ({ state: { data: [] as unknown[], isLoading: false, isError: false, refetch: vi.fn() }, addTo: vi.fn() }));
vi.mock("../../api/playlists", () => ({
  usePlaylistList: () => playlists.state,
  useAddToPlaylist: () => ({ mutateAsync: (...a: unknown[]) => playlists.addTo(...a) }),
}));
const collections = vi.hoisted(() => ({ state: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() } }));
vi.mock("../../api/collections", () => ({ useAllCollections: () => collections.state }));

import { BulkAddToPlaylistSheet, BulkAddToCollectionSheet } from "./BulkAddSheets";

beforeEach(() => {
  vi.clearAllMocks();
  playlists.state = { data: [], isLoading: false, isError: false, refetch: vi.fn() };
  playlists.addTo.mockResolvedValue(undefined);
  collections.state = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
  addCollectionItem.mockResolvedValue(undefined);
});

// "Select all" hands these sheets hundreds of ids: every id is added, ONE AT
// A TIME (not a Promise.all storm against the NAS), smart playlists are not
// offered (they compute their own members), a fetch failure says so instead
// of "you have none", and a failed add leaves the sheet open.
describe("BulkAddToPlaylistSheet", () => {
  it("adds the whole selection sequentially to a manual playlist and closes", async () => {
    playlists.state.data = [
      { id: "pl-manual", name: "Soirée", type: "manual" },
      { id: "pl-smart", name: "Auto", type: "smart" },
    ];
    let inFlight = 0;
    let maxInFlight = 0;
    playlists.addTo.mockImplementation(async () => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((r) => setTimeout(r, 0));
      inFlight--;
    });
    const onClose = vi.fn();
    render(<BulkAddToPlaylistSheet visible mediaItemIds={["m1", "m2", "m3"]} onClose={onClose} />);
    expect(screen.queryByText("Auto")).toBeNull();
    fireEvent.click(screen.getByText("Soirée"));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(playlists.addTo).toHaveBeenCalledTimes(3);
    expect(playlists.addTo.mock.calls.map((c) => (c[0] as { mediaItemId: string }).mediaItemId)).toEqual(["m1", "m2", "m3"]);
    expect(maxInFlight).toBe(1);
    expect(toast.success).toHaveBeenCalledWith("media.library.itemsAddedToPlaylist:3");
  });

  it("a failed add reports and keeps the sheet open; a failed fetch offers a retry", async () => {
    playlists.state.data = [{ id: "pl-manual", name: "Soirée", type: "manual" }];
    playlists.addTo.mockRejectedValue(new Error("nope"));
    const onClose = vi.fn();
    render(<BulkAddToPlaylistSheet visible mediaItemIds={["m1"]} onClose={onClose} />);
    fireEvent.click(screen.getByText("Soirée"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.library.failedAddPlaylist"));
    expect(onClose).not.toHaveBeenCalled();

    playlists.state = { data: undefined as unknown as unknown[], isLoading: false, isError: true, refetch: vi.fn() };
    render(<BulkAddToPlaylistSheet visible mediaItemIds={["m1"]} onClose={vi.fn()} />);
    fireEvent.click(screen.getByText("retry"));
    expect(playlists.state.refetch).toHaveBeenCalled();
    expect(screen.queryByText("media.library.noPlaylists")).toBeNull();
  });

  it("renders nothing while hidden and the empty message with no manual playlist", () => {
    const { container } = render(<BulkAddToPlaylistSheet visible={false} mediaItemIds={["m1"]} onClose={vi.fn()} />);
    expect(container.innerHTML).toBe("");
    playlists.state.data = [{ id: "pl-smart", name: "Auto", type: "smart" }];
    render(<BulkAddToPlaylistSheet visible mediaItemIds={["m1"]} onClose={vi.fn()} />);
    expect(screen.getByText("media.library.noPlaylists")).toBeTruthy();
  });
});

describe("BulkAddToCollectionSheet", () => {
  it("adds every id to the collection in order and closes", async () => {
    collections.state.data = { collections: [{ id: "c1", name: "Saga" }] };
    const onClose = vi.fn();
    render(<BulkAddToCollectionSheet visible mediaItemIds={["m1", "m2"]} onClose={onClose} />);
    fireEvent.click(screen.getByText("Saga"));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(addCollectionItem.mock.calls).toEqual([
      ["c1", { mediaItemId: "m1", sortOrder: 0 }],
      ["c1", { mediaItemId: "m2", sortOrder: 0 }],
    ]);
    expect(toast.success).toHaveBeenCalledWith("media.collections.itemsAdded:2");
  });

  it("says when there is no collection and reports a failed add", async () => {
    collections.state.data = { collections: [] };
    render(<BulkAddToCollectionSheet visible mediaItemIds={["m1"]} onClose={vi.fn()} />);
    expect(screen.getByText("media.collections.noCollectionsAvailable")).toBeTruthy();

    collections.state.data = { collections: [{ id: "c1", name: "Saga" }] };
    addCollectionItem.mockRejectedValue(new Error("nope"));
    const onClose = vi.fn();
    render(<BulkAddToCollectionSheet visible mediaItemIds={["m1"]} onClose={onClose} />);
    fireEvent.click(screen.getByText("Saga"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("media.collections.addFailed"));
    expect(onClose).not.toHaveBeenCalled();
  });
});
