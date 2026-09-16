import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// The filmography walks the keyset cursor (30 a page), appending, and stops
// when the server hands no cursor back; the heading keeps the first honest
// total; a failed person fetch is a retry for the header, a failed
// filmography a retry inside the list; the biography expands on demand.
const list = vi.hoisted(() => ({ onEndReached: null as null | (() => void) }));
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Dimensions: { get: () => ({ width: 400 }) },
}));
vi.mock("@legendapp/list/react-native", () => ({
  LegendList: ({ data, renderItem, keyExtractor, ListHeaderComponent, ListEmptyComponent, ListFooterComponent, onEndReached }: any) => {
    list.onEndReached = onEndReached;
    return React.createElement("div", null, ListHeaderComponent, (data ?? []).length === 0
      ? ListEmptyComponent
      : (data as any[]).map((item) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item }))), ListFooterComponent);
  },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router, Stack: { Screen: () => null }, useLocalSearchParams: () => ({ id: "p1" }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const api = vi.hoisted(() => ({
  person: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() },
  pages: {} as Record<string, { items: unknown[]; pagination?: Record<string, unknown> }>,
  mediaError: false,
  mediaRefetch: vi.fn(),
  lastArgs: null as any,
}));
vi.mock("@tanstack/react-query", () => ({ useQuery: () => api.person }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  eveyFetch: vi.fn(),
  personDepartmentKey: () => null,
  useLibraryServiceListMediaItems: (p: Record<string, unknown>) => {
    api.lastArgs = p;
    const key = String(p["pagination.cursor"] ?? "");
    return { data: api.mediaError ? undefined : api.pages[key], isLoading: false, isFetching: false, isError: api.mediaError, refetch: api.mediaRefetch };
  },
}));
vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888", accent: "#f00", border: "#333" }, radii: { md: 8, sm: 4 }, spacing: { sm: 8, md: 16, lg: 24, xl: 32 } }),
}));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("../../../src/lib/artwork", () => ({ artworkUrl: () => "https://s/art" }));
vi.mock("../../../src/components/LoadFailed", () => ({
  LoadFailed: ({ onRetry, inline }: any) => React.createElement("button", { onClick: onRetry }, inline ? "retry-list" : "retry-person"),
}));

import PersonScreen from "./[id]";

const items = (prefix: string, n: number) => Array.from({ length: n }, (_, i) => ({ id: `${prefix}${i}`, title: `Film ${prefix}${i}`, type: "MEDIA_ITEM_TYPE_MOVIE" }));

beforeEach(() => {
  vi.clearAllMocks();
  api.person = { data: { id: "p1", name: "Jane Doe", biography: "A long life." }, isLoading: false, isError: false, refetch: vi.fn() };
  api.pages = {};
  api.mediaError = false;
  list.onEndReached = null;
});

describe("person screen (mobile)", () => {
  it("walks the cursor, appends pages, keeps the first honest total, and stops without a cursor", () => {
    api.pages = {
      "": { items: items("a", 30), pagination: { nextCursor: "c1", totalItems: 45 } },
      c1: { items: items("b", 15), pagination: { totalItems: 0 } },
    };
    render(<PersonScreen />);
    expect(api.lastArgs).toEqual({ personId: "p1", "pagination.pageSize": 30 });
    expect(screen.getByText("media.person.filmography (45)")).toBeTruthy();
    expect(screen.getAllByText(/^Film /)).toHaveLength(30);
    act(() => list.onEndReached?.());
    expect(api.lastArgs["pagination.cursor"]).toBe("c1");
    expect(screen.getAllByText(/^Film /)).toHaveLength(45);
    expect(screen.getByText("media.person.filmography (45)")).toBeTruthy();
    act(() => list.onEndReached?.());
    expect(api.lastArgs["pagination.cursor"]).toBe("c1");
    fireEvent.click(screen.getByText("Film b3").closest("button")!);
    expect(router.push).toHaveBeenCalledWith("/(auth)/media/b3");
  });

  it("expands the biography on demand", () => {
    api.pages = { "": { items: [], pagination: {} } };
    render(<PersonScreen />);
    expect(screen.getByText("media.person.noFilmography")).toBeTruthy();
    fireEvent.click(screen.getByText("common.readMore"));
    expect(screen.getByText("common.showLess")).toBeTruthy();
  });

  it("retries the header when the person failed, the list when the filmography failed", () => {
    api.person = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    const first = render(<PersonScreen />);
    fireEvent.click(screen.getByText("retry-person"));
    expect(api.person.refetch).toHaveBeenCalledTimes(1);
    first.unmount();

    api.person = { data: { id: "p1", name: "Jane Doe" }, isLoading: false, isError: false, refetch: vi.fn() };
    api.mediaError = true;
    render(<PersonScreen />);
    expect(screen.getByText("Jane Doe")).toBeTruthy();
    fireEvent.click(screen.getByText("retry-list"));
    expect(api.mediaRefetch).toHaveBeenCalledTimes(1);
  });
});
