import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  TextInput: ({ value, onChangeText, accessibilityLabel, maxLength }: any) =>
    React.createElement("input", { "aria-label": accessibilityLabel, value: value ?? "", maxLength, onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => (typeof f === "string" ? f : _k) }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", surfaceRaised: "#222", bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", overlay: "#0008" }, radii: { md: 8 } }),
}));
const playlists = vi.hoisted(() => ({ data: [] as unknown[] }));
vi.mock("../api/playlists", () => ({ usePlaylistList: () => ({ data: playlists.data }) }));

import { SyncPlayModal } from "./SyncPlayModal";

function mount(over: Partial<React.ComponentProps<typeof SyncPlayModal>> = {}) {
  const h = { onClose: vi.fn(), onCreateRoom: vi.fn(), onJoinRoom: vi.fn(), onCreateRoomFromPlaylist: vi.fn() };
  render(<SyncPlayModal visible {...h} {...over} />);
  return h;
}

beforeEach(() => {
  vi.clearAllMocks();
  playlists.data = [];
});

// The join code is folded as typed (case, separators, the misread letters),
// Join stays dead until a plausible code is in, and the playlist tab only
// exists when the caller allows it AND the user owns a playlist.
describe("SyncPlayModal", () => {
  it("normalises the code as typed and joins only with a plausible one", () => {
    const h = mount();
    const input = screen.getByLabelText("Join with code") as HTMLInputElement;
    const join = screen.getByText("Join").closest("button") as HTMLButtonElement;
    expect(join.disabled).toBe(true);
    fireEvent.change(input, { target: { value: "ab-1" } });
    expect(input.value).toBe("AB1");
    expect(join.disabled).toBe(true);
    fireEvent.change(input, { target: { value: "ab c1 lo" } });
    expect(input.value).toBe("ABC110");
    expect(join.disabled).toBe(false);
    fireEvent.click(join);
    expect(h.onJoinRoom).toHaveBeenCalledWith("ABC110");
    fireEvent.click(screen.getByText("Create room").closest("button")!);
    expect(h.onCreateRoom).toHaveBeenCalled();
  });

  it("offers the playlist tab only with permission and a playlist, and creates from the picked one", () => {
    mount({ showPlaylistOption: true });
    expect(screen.queryByText("Or pick a playlist")).toBeNull();

    playlists.data = [{ id: "pl1", name: "Soirée", type: "manual" }];
    const h = mount({ showPlaylistOption: true });
    fireEvent.click(screen.getByText("Or pick a playlist").closest("button")!);
    fireEvent.click(screen.getByText("Soirée").closest("button")!);
    expect(h.onCreateRoomFromPlaylist).toHaveBeenCalledWith("pl1", "Soirée");
  });

  it("renders nothing while hidden and disables creation while a room is being made", () => {
    const { container } = render(<SyncPlayModal visible={false} onClose={vi.fn()} onCreateRoom={vi.fn()} onJoinRoom={vi.fn()} />);
    expect(container.innerHTML).toBe("");
    mount({ creating: true });
    expect((screen.getByText("Create room").closest("button") as HTMLButtonElement).disabled).toBe(true);
  });
});
