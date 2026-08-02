import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  FlatList: ({ data, renderItem }: any) =>
    React.createElement("div", null, (data ?? []).map((item: unknown, index: number) =>
      React.createElement("div", { key: index }, renderItem({ item, index })),
    )),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#aaa", accent: "#88f", border: "#333", onAccent: "#000" },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 16, lg: 24, xl: 32 },
  }),
}));

vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        // Booleans vanish through unknown DOM props: surface it as data-* so
        // the focus claim is observable.
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : "false",
      },
      children,
    ),
}));

const playlistsMock = vi.fn();
const addMock = vi.fn();
vi.mock("../api/playlists", () => ({
  usePlaylistList: () => playlistsMock(),
  useAddToPlaylist: () => ({ mutate: addMock, isPending: false }),
}));

vi.mock("@evey/store", () => ({ toast: { error: vi.fn(), success: vi.fn() } }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, md: 16, lg: 20, xl: 24 } }));

import { AddToPlaylistSheet } from "./AddToPlaylistSheet";

const closeFocus = () =>
  screen.getByLabelText("Close").getAttribute("data-tv-preferred-focus");

beforeEach(() => {
  vi.clearAllMocks();
});

// A TV screen with nothing focused stops receiving key events altogether, so
// an empty sheet without a focus claim traps the viewer inside it.
describe("AddToPlaylistSheet keeps the remote alive", () => {
  it("gives the close button the initial focus while the playlists load", () => {
    playlistsMock.mockReturnValue({ data: undefined, isLoading: true });
    render(<AddToPlaylistSheet visible mediaItemId="mi-1" onClose={() => {}} />);
    expect(closeFocus()).toBe("true");
  });

  it("gives the close button the initial focus when there is no manual playlist", () => {
    playlistsMock.mockReturnValue({ data: [], isLoading: false });
    render(<AddToPlaylistSheet visible mediaItemId="mi-1" onClose={() => {}} />);
    expect(closeFocus()).toBe("true");
  });

  it("leaves the initial focus to the list once there is something to pick", () => {
    playlistsMock.mockReturnValue({
      data: [{ id: "pl-1", name: "Soirée", type: "manual" }],
      isLoading: false,
    });
    render(<AddToPlaylistSheet visible mediaItemId="mi-1" onClose={() => {}} />);
    expect(closeFocus()).toBe("false");
    expect(screen.getByText("Soirée")).toBeTruthy();
  });
});
