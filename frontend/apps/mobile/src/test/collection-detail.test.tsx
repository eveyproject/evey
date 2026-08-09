import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The audit found two viewer-facing gaps on this screen: member tiles never
// called artworkUrl (always the noPoster placeholder, even for members with
// an id), and the saga overview was only ever read to pre-fill the admin
// edit form - a normal viewer never saw it rendered anywhere.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      { "data-testid": "collection-list" },
      (data ?? []).length === 0
        ? ListEmptyComponent ?? null
        : data.map((item: any) =>
            React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })),
          ),
    ),
  TouchableOpacity: ({ children, onPress, ...p }: any) =>
    React.createElement("button", { onClick: onPress, ...p }, children),
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

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("expo-router", () => ({
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ id: "col-1" }),
  useRouter: () => ({ push: vi.fn(), back: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: any) => unknown) => selector({ isAdmin: false }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000",
      text: "#fff",
      textMuted: "#888",
      accent: "#f00",
      onAccent: "#fff",
      surface: "#111",
      surfaceRaised: "#222",
      border: "#333",
      danger: "#e00",
    },
    radii: { sm: 4, md: 8 },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));

vi.mock("../components/Skeleton", () => ({
  Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }),
}));

// --- ../api/collections ------------------------------------------------------
let collectionState: {
  data?: { collection: { id: string; name: string; overview?: string }; items: unknown[]; total: number };
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
  fetchNextPage: () => void;
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
};

vi.mock("../api/collections", () => ({
  useCollection: () => collectionState,
  useUpdateCollection: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteCollection: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useRemoveCollectionItem: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useReorderCollectionItems: () => ({ mutate: vi.fn(), isPending: false }),
}));

import CollectionDetailScreen from "../../app/(auth)/collections/[id]";

beforeEach(() => {
  vi.clearAllMocks();
});

describe("CollectionDetailScreen (mobile) artwork", () => {
  const MEMBER = { id: "m-9", title: "Some Movie", type: "MEDIA_ITEM_TYPE_MOVIE", year: 1999 };

  it("renders a poster image for a member with an id instead of the placeholder", () => {
    collectionState = {
      data: { collection: { id: "col-1", name: "Some Collection" }, items: [MEMBER], total: 1 },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
    render(<CollectionDetailScreen />);

    // querySelector, not getByRole("img"): the mock renders alt="" which
    // removes the element from the accessibility tree (role presentation).
    const img = document.querySelector("img");
    expect(img).toBeTruthy();
    expect(img!.getAttribute("src")).toContain(`/media/${MEMBER.id}/artwork/poster`);
    expect(screen.queryByText("common.noPoster")).toBeNull();
  });
});

describe("CollectionDetailScreen (mobile) overview", () => {
  it("shows the saga overview to a non-admin viewer when present", () => {
    collectionState = {
      data: { collection: { id: "col-1", name: "Some Collection", overview: "A saga about something." }, items: [], total: 0 },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
    render(<CollectionDetailScreen />);

    expect(screen.getByText("A saga about something.")).toBeTruthy();
  });

  it("renders no overview text when the collection has none", () => {
    collectionState = {
      data: { collection: { id: "col-1", name: "Some Collection" }, items: [], total: 0 },
      isLoading: false,
      isError: false,
      refetch: vi.fn(),
      fetchNextPage: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
    };
    render(<CollectionDetailScreen />);

    expect(screen.getByTestId("collection-list")).toBeTruthy();
    expect(screen.queryByText(/saga/i)).toBeNull();
  });
});
