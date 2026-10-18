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
  TouchableOpacity: ({ children, onPress, onLongPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, onContextMenu: onLongPress, disabled, "aria-label": accessibilityLabel }, children),
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
  Stack: { Screen: ({ options }: any) => React.createElement("div", null, options?.headerRight?.() ?? null) },
  useRouter: () => ({ push: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const store = vi.hoisted(() => ({ isAdmin: false, demoMode: false }));
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector(store),
  useDemoStore: (selector: (s: any) => unknown) => selector(store),
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
const deleteCollection = vi.hoisted(() => vi.fn());

vi.mock("../../../src/api/collections", () => ({
  useCollections: () => collectionsState,
  useCreateCollection: () => ({ mutate: vi.fn(), mutateAsync: vi.fn(), isPending: false }),
  useDeleteCollection: () => ({ mutate: deleteCollection, isPending: false }),
}));

import CollectionsScreen from "./index";

beforeEach(() => {
  vi.clearAllMocks();
  collectionsState = { data: { collections: [] }, isLoading: false, isError: false, refetch: vi.fn() };
  store.isAdmin = false;
  store.demoMode = false;
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

// The demo's collections are the shared account's and the server refuses
// them: an admin there sees create greyed and the long-press delete gone.
describe("CollectionsScreen - the demo", () => {
  const sagas = { collections: [{ id: "c1", name: "Dune saga", kind: "manual", itemCount: 2 }] };

  it("greys create and drops the delete gesture on the demo", async () => {
    const { Alert } = await import("react-native");
    store.isAdmin = true;
    store.demoMode = true;
    collectionsState = { data: sagas, isLoading: false, isError: false, refetch: vi.fn() };
    render(<CollectionsScreen />);
    const create = screen.getByLabelText("media.collections.newCollection") as HTMLButtonElement;
    expect(create.disabled).toBe(true);
    fireEvent.click(create);
    expect(screen.queryByTestId("modal")).toBeNull();
    fireEvent.contextMenu(screen.getByText("Dune saga").closest("button")!);
    expect(Alert.alert).not.toHaveBeenCalled();
    expect(deleteCollection).not.toHaveBeenCalled();
  });

  it("keeps both for an admin off the demo", async () => {
    const { Alert } = await import("react-native");
    store.isAdmin = true;
    collectionsState = { data: sagas, isLoading: false, isError: false, refetch: vi.fn() };
    render(<CollectionsScreen />);
    const create = screen.getByLabelText("media.collections.newCollection") as HTMLButtonElement;
    expect(create.disabled).toBeFalsy();
    fireEvent.click(create);
    expect(screen.getByTestId("modal")).toBeTruthy();
    fireEvent.contextMenu(screen.getByText("Dune saga").closest("button")!);
    expect(Alert.alert).toHaveBeenCalledWith("media.collections.deleteConfirm", "Dune saga", expect.any(Array));
  });
});
