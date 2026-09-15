import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A failed collections fetch used to render as "you have no collections",
// which is indistinguishable from an actually empty library.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "collections-list" },
      (data ?? []).length === 0
        ? ListEmptyComponent ?? null
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
    ),
  TouchableOpacity: ({ children, onPress, onLongPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, onContextMenu: onLongPress, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  Alert: { alert: vi.fn() },
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  TextInput: ({ value, onChangeText, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    }),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "modal-spinner", ...p }),
  KeyboardAvoidingView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Platform: { OS: "android" },
}));

vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: false }),
}));

vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", onAccent: "#fff" },
    radii: { sm: 4, md: 8 },
  }),
}));

vi.mock("../../../src/components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

// --- ../../../src/api/collections ---------------------------------------------------------
let collectionsState: { data?: { collections: unknown[] }; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("../../../src/api/collections", () => ({
  useCollections: () => collectionsState,
  useCreateCollection: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useDeleteCollection: () => ({ mutate: vi.fn(), isPending: false }),
}));

import CollectionsScreen from "./index";

beforeEach(() => {
  vi.clearAllMocks();
  collectionsState = { data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() };
});

describe("CollectionsScreen - loading vs failure vs genuinely no collections", () => {
  it("shows a skeleton grid while loading, not the failure or the empty message", () => {
    collectionsState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<CollectionsScreen />);

    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    expect(screen.queryByText("media.collections.noCollections")).toBeNull();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });

  it("shows LoadFailed with a retry - not 'no collections' - when the fetch fails", () => {
    const refetch = vi.fn();
    collectionsState = { data: undefined, isLoading: false, isError: true, refetch };
    render(<CollectionsScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("media.collections.noCollections")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("shows 'no collections' only once the list is confirmed empty", () => {
    collectionsState = { data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() };
    render(<CollectionsScreen />);

    expect(screen.getByText("media.collections.noCollections")).toBeTruthy();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });
});
