import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// PlaylistItemRow used to hardcode "Move up" / "Move down" / "Remove" as the
// accessibilityLabel - the only text a screen reader gets for these buttons -
// in English regardless of the app's language.

vi.mock("../hooks/usePlaylistDownloadAll", () => ({ usePlaylistDownloadAll: () => ({ downloadAll: vi.fn(), downloading: false }) }));
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, disabled, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement(
      "div",
      {},
      data.map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item, index: 0 }))),
    ),
  Alert: { alert: vi.fn() },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: () => React.createElement("span", {}),
}));

vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ id: "pl-1" }),
  useRouter: () => ({ back: vi.fn(), push: vi.fn() }),
}));

const FR_LABELS: Record<string, string> = {
  "media.playlist.moveItemUp": "Monter",
  "media.playlist.moveItemDown": "Descendre",
  "media.playlist.removeItem": "Retirer",
};
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => FR_LABELS[key] ?? key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333", error: "#f00", surface: "#111" },
    radii: { md: 8 },
    spacing: { md: 12 },
  }),
}));

const PLAYLIST = {
  id: "pl-1",
  name: "My Mix",
  description: "",
  type: "manual",
  items: [
    { media_item_id: "m1", title: "Track One", sort_order: 0 },
    { media_item_id: "m2", title: "Track Two", sort_order: 1 },
  ],
};

vi.mock("../api/playlists", () => ({
  usePlaylistDetail: () => ({ data: PLAYLIST, isLoading: false, isError: false, refetch: vi.fn() }),
  useDeletePlaylist: () => ({ mutateAsync: vi.fn() }),
  useRemoveFromPlaylist: () => ({ mutate: vi.fn() }),
  useReorderPlaylistItems: () => ({ mutate: vi.fn() }),
  useUpdatePlaylist: () => ({ mutate: vi.fn() }),
}));

vi.mock("../components/SmartRulesEditor", () => ({ SmartRulesEditor: () => null }));

import PlaylistDetailScreen from "../../app/(auth)/playlists/[id]";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("playlist item row - localized accessibility labels", () => {
  it("labels move/remove buttons in the active language instead of hardcoded English", () => {
    render(<PlaylistDetailScreen />);

    expect(screen.getAllByLabelText("Monter").length).toBeGreaterThan(0);
    expect(screen.getAllByLabelText("Descendre").length).toBeGreaterThan(0);
    expect(screen.getAllByLabelText("Retirer").length).toBeGreaterThan(0);
    expect(screen.queryByLabelText("Move up")).toBeNull();
    expect(screen.queryByLabelText("Remove")).toBeNull();
  });
});
