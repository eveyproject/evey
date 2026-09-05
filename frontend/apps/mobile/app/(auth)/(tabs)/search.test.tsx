import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A dead music search request used to render as "no results", exactly what a
// query with zero matches looks like - the music branch never destructured
// isError, unlike the media branch right above it in the same screen.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ accessibilityLabel, onChangeText, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      null,
      (data ?? []).length === 0
        ? (ListEmptyComponent ?? null)
        : data.map((item: any, i: number) =>
            React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item, i) : i }, renderItem({ item, index: i })),
          ),
    ),
  TouchableOpacity: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
  PixelRatio: { get: () => 2 },
}));
vi.mock("expo-image", () => ({ Image: (p: any) => React.createElement("img", { alt: "", ...p }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: (p: any) => React.createElement("span", p) }));
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: vi.fn() }),
  useLocalSearchParams: () => ({}),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("expo-secure-store", () => ({ getItem: () => null, setItem: () => undefined }));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#fff", surface: "#111", surfaceRaised: "#222", border: "#333" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 4, md: 8, lg: 12, xl: 16 },
  }),
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv" }));
vi.mock("../../../src/hooks/useDebounce", () => ({ useDebounce: (v: string) => v }));
vi.mock("../../../src/hooks/useAllGenres", () => ({ useAllGenres: () => ({ data: [] }) }));
vi.mock("../../../src/stores/musicQueue", () => ({
  useMusicQueueStore: (sel: (s: any) => unknown) => sel({ setQueue: vi.fn() }),
}));

let mediaError = false;
let musicError = false;
const mediaRefetch = vi.fn();
const musicRefetch = vi.fn();

vi.mock("@evey/api", () => ({
  displayTitle: (item: any) => item.title ?? "",
  useLibraryServiceSearchMedia: () => ({ data: { items: [], totalHits: 0 }, isLoading: false, isError: mediaError, refetch: mediaRefetch }),
  useMusicServiceSearchMusic: () => ({ data: { tracks: [] }, isLoading: false, isError: musicError, refetch: musicRefetch }),
}));

import SearchScreen from "./search";

beforeEach(() => {
  vi.clearAllMocks();
  mediaError = false;
  musicError = false;
});

describe("SearchScreen - music scope, loading vs failure vs empty", () => {
  it("shows LoadFailed with a retry, not 'no results', when the music search fails", () => {
    musicError = true;
    render(<SearchScreen />);

    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "abba" } });
    fireEvent.click(screen.getByText("media.search.sectionMusic"));

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.search.noResults")).toBeNull();

    fireEvent.click(screen.getByText("common.retry"));
    expect(musicRefetch).toHaveBeenCalledOnce();
  });
});
