import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// The "I have a code" entry point: what is typed is folded to the stored
// form (case, separators, the look-alike letters), Join is refused under
// five characters, a joined room replaces this screen with the player, and
// a room that names no media or a refused join is told, not swallowed.
const alerts = vi.hoisted(() => ({ messages: [] as string[] }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText, accessibilityLabel, editable }: any) =>
    React.createElement("input", { value, "aria-label": accessibilityLabel, disabled: editable === false, onChange: (e: any) => onChangeText(e.target.value) }),
  Pressable: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Alert: { alert: (_t: string, m: string) => { alerts.messages.push(m); } },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const replace = vi.hoisted(() => vi.fn());
vi.mock("expo-router", () => ({ Stack: { Screen: () => null }, useRouter: () => ({ replace }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, fallback?: string) => fallback ?? k }) }));
const joinByCode = vi.hoisted(() => vi.fn());
const createRoomFromQueue = vi.hoisted(() => vi.fn());
const syncState = vi.hoisted(() => ({ roomId: null as string | null }));
vi.mock("../../src/hooks/useSyncPlay", () => ({ useSyncPlay: () => ({ joinByCode, createRoomFromQueue, leaveRoom: vi.fn(), state: syncState }) }));
vi.mock("@evey/api", async (orig) => ({
  ...(await orig<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "lib-music", type: "LIBRARY_TYPE_MUSIC" }] } }),
}));
vi.mock("../../src/stores/musicQueue", () => ({ useMusicQueueStore: { setState: vi.fn() } }));
const builder = vi.hoisted(() => ({ onCreate: null as null | ((items: { id: string; title: string; artist: string }[]) => void) }));
vi.mock("../../src/features/syncplay/RoomBuilder", () => ({
  RoomBuilder: (p: { onCreate: (items: { id: string; title: string; artist: string }[]) => void }) => { builder.onCreate = p.onCreate; return React.createElement("div", { "data-testid": "room-builder" }); },
}));
vi.mock("../../src/features/syncplay/ListenTogetherRoom", () => ({
  ListenTogetherRoom: () => React.createElement("div", { "data-testid": "listen-room" }),
}));

vi.mock("../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333" }, radii: { md: 8 }, spacing: { sm: 8, md: 16, lg: 24, xl: 32 } }),
}));

import SyncPlayScreen from "./syncplay";

const input = () => screen.getByLabelText("Room code") as HTMLInputElement;
const join = () => screen.getByRole("button", { name: "Join" }) as HTMLButtonElement;

beforeEach(() => {
  vi.clearAllMocks();
  alerts.messages.length = 0;
  joinByCode.mockResolvedValue({ mediaId: "m1", mediaType: "video" });
  syncState.roomId = null;
});

describe("syncplay join screen (mobile)", () => {
  it("folds the typed code, refuses a short one, and lands in the player once joined", async () => {
    render(<SyncPlayScreen />);
    expect(join().disabled).toBe(true);
    fireEvent.change(input(), { target: { value: "ab-c1" } });
    expect(input().value).toBe("ABC1");
    expect(join().disabled).toBe(true);
    fireEvent.change(input(), { target: { value: "olive2" } });
    // O -> 0, L and I -> 1: the look-alikes the code alphabet leaves out.
    expect(input().value).toBe("011VE2");
    expect(join().disabled).toBe(false);
    fireEvent.click(join());
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/player/m1"));
    expect(joinByCode).toHaveBeenCalledWith("011VE2");
    expect(alerts.messages).toEqual([]);
  });

  it("tells the viewer when the room names no media or the join is refused, and re-enables the form", async () => {
    joinByCode.mockResolvedValueOnce({ mediaId: "" });
    render(<SyncPlayScreen />);
    fireEvent.change(input(), { target: { value: "ABC123" } });
    fireEvent.click(join());
    await waitFor(() => expect(alerts.messages).toEqual(["Could not join the room."]));
    expect(replace).not.toHaveBeenCalled();
    expect(join().disabled).toBe(false);

    joinByCode.mockRejectedValueOnce(new Error("404"));
    fireEvent.click(join());
    await waitFor(() => expect(alerts.messages).toHaveLength(2));
    expect(input().disabled).toBe(false);
  });

  // A music room is listened to on this screen, not opened in the video player.
  it("stays here and opens the listening room when the joined room is a music room", async () => {
    joinByCode.mockImplementationOnce(async () => { syncState.roomId = "room-1"; return { mediaId: "t1", mediaType: "music" }; });
    render(<SyncPlayScreen />);
    fireEvent.change(input(), { target: { value: "ABC123" } });
    fireEvent.click(join());
    expect(await screen.findByTestId("listen-room")).toBeTruthy();
    expect(replace).not.toHaveBeenCalled();
  });

  it("creates a listening room from the builder's queue", async () => {
    createRoomFromQueue.mockImplementationOnce(async () => { syncState.roomId = "room-2"; return "room-2"; });
    render(<SyncPlayScreen />);
    expect(screen.getByTestId("room-builder")).toBeTruthy();
    builder.onCreate?.([{ id: "t1", title: "Some Track", artist: "Some Artist" }, { id: "t2", title: "Other Track", artist: "Some Artist" }]);
    await waitFor(() => expect(createRoomFromQueue).toHaveBeenCalledWith(["t1", "t2"]));
    expect(await screen.findByTestId("listen-room")).toBeTruthy();
  });
});
