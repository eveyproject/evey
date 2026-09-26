import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  TextInput: () => null,
  ActivityIndicator: () => null,
  Pressable: ({ children, onPress, disabled, testID, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", surface: "#111", error: "#f00" }, radii: { sm: 4, md: 8 } }),
}));
vi.mock("../../hooks/useDebounce", () => ({ useDebounce: (v: string) => v }));
vi.mock("@evey/api", () => ({
  useMusicServiceGetAlbums: () => ({ data: { albums: [{ id: "al1", title: "Some Album", artist: "Some Artist" }] } }),
  useMusicServiceListTracks: () => ({ data: { tracks: [] } }),
}));
const albumItems = vi.hoisted(() => vi.fn());
vi.mock("@evey/client", async (orig) => ({
  ...(await orig<typeof import("@evey/client")>()),
  albumRoomQueueItems: (...a: unknown[]) => albumItems(...a),
  usePlaylistList: () => ({ data: [] }),
}));
vi.mock("@evey/store", () => ({ toast: { error: vi.fn() } }));

import { RoomBuilder } from "./RoomBuilder";

beforeEach(() => vi.clearAllMocks());

describe("RoomBuilder (mobile)", () => {
  it("adds a whole album and creates the room with its tracks in order", async () => {
    albumItems.mockResolvedValue([{ id: "t1", title: "One", artist: "" }, { id: "t2", title: "Two", artist: "" }]);
    const onCreate = vi.fn();
    render(<RoomBuilder libraryId="lib" creating={false} onCreate={onCreate} />);
    expect((screen.getByTestId("room-create") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("room-add-album-al1"));
    await waitFor(() => expect(screen.getByTestId("room-queue-t2")).toBeTruthy());
    expect(albumItems).toHaveBeenCalledWith("lib", "al1");
    fireEvent.click(screen.getByTestId("room-create"));
    expect(onCreate).toHaveBeenCalledWith([{ id: "t1", title: "One", artist: "" }, { id: "t2", title: "Two", artist: "" }]);
  });
});
