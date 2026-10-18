import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The header counts what the server holds and the grid pages towards it as
// it scrolls (a 312-item filter used to stop dead at 50); the rules are
// shown as badges; only an admin gets the rules editor, whose save writes
// the rules and refetches; an empty result is worded by whether any rule
// exists; a failed fetch is a retry.
const list = vi.hoisted(() => ({ onEndReached: null as null | (() => void) }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListEmptyComponent, onEndReached }: any) => {
    list.onEndReached = onEndReached;
    return React.createElement("div", null, ListHeaderComponent, (data ?? []).length === 0
      ? ListEmptyComponent
      : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item }))));
  },
  TouchableOpacity: ({ children, onPress, disabled, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
  Pressable: ({ children, onPress }: any) => React.createElement("button", { onClick: onPress }, children),
  Dimensions: { get: () => ({ width: 360 }) },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: ({ source }: any) => React.createElement("img", { src: source?.uri, alt: "" }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
const header = vi.hoisted(() => ({ title: "", right: null as React.ReactNode }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({
  Stack: {
    Screen: ({ options }: any) => {
      header.title = options.title;
      return React.createElement("div", { "data-testid": "header" }, options.headerRight?.());
    },
  },
  useLocalSearchParams: () => ({ filterId: "f1" }),
  useRouter: () => router,
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: string | { count?: number }) => (typeof o === "string" ? o : o ? `${k}:${o.count}` : k) }),
}));
const auth = vi.hoisted(() => ({ isAdmin: false, demoMode: false }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: typeof auth) => unknown) => sel(auth),
  useDemoStore: (sel: (s: typeof auth) => unknown) => sel(auth),
  toast: { success: vi.fn(), error: vi.fn() },
}));
vi.mock("../../../../src/components/Skeleton", () => ({ Skeleton: () => React.createElement("div", { "data-testid": "skeleton" }) }));
vi.mock("../../../../src/components/SmartRulesEditor", () => ({
  COLLECTION_FIELDS: ["genre", "year"],
  SmartRulesEditor: ({ rules, onSave, onClose }: any) =>
    React.createElement("div", { "data-testid": "editor" }, JSON.stringify(rules),
      React.createElement("button", { onClick: () => onSave([{ field: "year", operator: "gte", value: "2000" }]) }, "save-rules"),
      React.createElement("button", { onClick: onClose }, "close-editor")),
}));
vi.mock("../../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00" }, radii: { sm: 4 } }),
}));
vi.mock("../../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));
vi.mock("../../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("../../../../src/lib/artwork", () => ({ artworkUrl: (s: string, id: string) => `${s}/art/${id}` }));
const api = vi.hoisted(() => ({
  col: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn(), fetchNextPage: vi.fn(), hasNextPage: false, isFetchingNextPage: false },
  update: vi.fn(),
}));
vi.mock("../../../../src/api/collections", () => ({
  useCollection: () => api.col,
  useUpdateCollection: () => ({ mutate: api.update }),
}));

import SmartFilterScreen from "./[filterId]";

beforeEach(() => {
  vi.clearAllMocks();
  auth.isAdmin = false;
  auth.demoMode = false;
  list.onEndReached = null;
  api.col = { data: undefined, isLoading: false, isError: false, refetch: vi.fn(), fetchNextPage: vi.fn(), hasNextPage: true, isFetchingNextPage: false };
});

describe("smart collection screen (mobile)", () => {
  it("counts the server total, pages as the grid scrolls, shows rule badges and opens a title", () => {
    api.col.data = {
      collection: { name: "Nineties", rules: [{ field: "year", operator: "gte", value: "1990" }, { field: "genre", operator: "weird", value: "Drama" }] },
      items: [{ id: "m1", title: "Some Movie", year: 1995 }, { id: "m2", title: "No year" }],
      total: 312,
    };
    const { rerender } = render(<SmartFilterScreen />);
    expect(header.title).toBe("Nineties");
    expect(screen.getByText("common.item:312")).toBeTruthy();
    expect(screen.getByText("year >= 1990")).toBeTruthy();
    expect(screen.getByText("genre weird Drama")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit rules" })).toBeNull();

    list.onEndReached?.();
    expect(api.col.fetchNextPage).toHaveBeenCalledTimes(1);
    api.col = { ...api.col, isFetchingNextPage: true };
    rerender(<SmartFilterScreen />);
    list.onEndReached?.();
    expect(api.col.fetchNextPage).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByText("Some Movie"));
    expect(router.push).toHaveBeenCalledWith("/media/m1");
  });

  it("lets an admin edit the rules, saving them and refetching", () => {
    auth.isAdmin = true;
    api.col.data = { collection: { name: "Nineties", rules: [{ field: "year", value: "1990" }] }, items: [], total: 0 };
    render(<SmartFilterScreen />);
    expect(screen.getByText("No items match these rules")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Edit rules" }));
    expect(screen.getByTestId("editor").textContent).toContain('"operator":"eq"');
    fireEvent.click(screen.getByText("save-rules"));
    expect(api.update).toHaveBeenCalledWith(
      { id: "f1", name: "Nineties", rules: [{ field: "year", operator: "gte", value: "2000" }] },
      expect.any(Object),
    );
    api.update.mock.calls[0][1].onSuccess();
    expect(api.col.refetch).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByText("close-editor"));
    expect(screen.queryByTestId("editor")).toBeNull();
  });

  // The demo's collections are the shared account's and the server refuses
  // them: an admin there sees the rules editor greyed, a press opens nothing.
  it("greys the rules editor on the demo", () => {
    auth.isAdmin = true;
    auth.demoMode = true;
    api.col.data = { collection: { name: "Nineties", rules: [{ field: "year", value: "1990" }] }, items: [], total: 0 };
    render(<SmartFilterScreen />);
    const edit = screen.getByRole("button", { name: "Edit rules" }) as HTMLButtonElement;
    expect(edit.disabled).toBe(true);
    fireEvent.click(edit);
    expect(screen.queryByTestId("editor")).toBeNull();
    expect(api.update).not.toHaveBeenCalled();
  });

  it("words an empty result by whether rules exist, skeletons while loading, retries on failure", () => {
    api.col.data = { collection: { name: "Blank", rules: [] }, items: [], total: 0 };
    const first = render(<SmartFilterScreen />);
    expect(screen.getByText("No filter rules defined")).toBeTruthy();
    first.unmount();

    api.col.isLoading = true;
    const second = render(<SmartFilterScreen />);
    expect(screen.getAllByTestId("skeleton").length).toBeGreaterThan(0);
    second.unmount();

    api.col.isLoading = false;
    api.col.isError = true;
    api.col.data = undefined;
    render(<SmartFilterScreen />);
    expect(header.title).toBe("Smart Filter");
    fireEvent.click(screen.getByText("common.retry"));
    expect(api.col.refetch).toHaveBeenCalledTimes(1);
  });
});
