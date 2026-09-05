import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The checkmark button that confirms "create this playlist" had no
// accessibilityLabel/Role, and its `disabled` state wasn't mirrored into
// accessibilityState - a screen reader user had no way to know what the
// button did or whether it was actionable yet.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Modal: ({ children }: any) => React.createElement("div", {}, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      {},
      (data ?? []).map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item }))),
    ),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, accessibilityRole, accessibilityState, disabled, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: onPress,
        "aria-label": accessibilityLabel,
        role: accessibilityRole,
        "aria-disabled": accessibilityState?.disabled ?? disabled,
        disabled,
        ...p,
      },
      children,
    ),
  Pressable: ({ children, onPress, ...p }: any) => React.createElement("button", { onClick: onPress, ...p }, children),
  TextInput: ({ accessibilityLabel, onChangeText, value, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: () => React.createElement("span", {}),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", border: "#333", overlay: "#0008" },
    radii: { md: 8, lg: 16 },
    spacing: { md: 12 },
  }),
}));

vi.mock("../api/playlists", () => ({
  usePlaylistList: () => ({ data: [], isLoading: false, isError: false, refetch: vi.fn() }),
  useAddToPlaylist: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useCreatePlaylist: () => ({ mutateAsync: vi.fn(async () => ({ playlist: { id: "new-1" } })), isPending: false }),
}));

import { AddToPlaylistSheet } from "../components/AddToPlaylistSheet";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("AddToPlaylistSheet - create button accessibility", () => {
  it("labels the confirm-create button and reflects its disabled state", () => {
    render(<AddToPlaylistSheet visible mediaItemId="m1" onClose={vi.fn()} />);

    fireEvent.click(screen.getByText("media.music.newPlaylist"));

    const confirmBtn = screen.getByRole("button", { name: "media.playlist.createPlaylist" });
    // Empty name -> disabled.
    expect(confirmBtn.getAttribute("aria-disabled")).toBe("true");

    fireEvent.change(screen.getByLabelText("media.music.createPlaylistPrompt"), { target: { value: "Party mix" } });
    expect(confirmBtn.getAttribute("aria-disabled")).toBe("false");
  });
});
