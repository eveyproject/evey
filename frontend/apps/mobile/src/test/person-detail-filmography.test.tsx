import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A prolific actor's filmography used to fetch up to 500 posters at once and
// .map() them straight into the grid, freezing mid-range phones on first paint.
// This guards the rewrite: a LegendList streams the DB by opaque cursor
// (keyset), 30 per page, and only mounts the ~visible rows.

// --- LegendList mock: render header + every data item + expose onEndReached ---
vi.mock("@legendapp/list/react-native", () => ({
  LegendList: ({ data, renderItem, ListHeaderComponent, ListFooterComponent, ListEmptyComponent, onEndReached }: any) =>
    React.createElement(
      "div",
      {},
      ListHeaderComponent,
      data.length === 0
        ? ListEmptyComponent
        : data.map((item: any, index: number) =>
            React.createElement("div", { key: item.id }, renderItem({ item, index })),
          ),
      ListFooterComponent,
      React.createElement("button", { "data-testid": "end-reached", onClick: () => onEndReached?.() }, "end"),
    ),
}));

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: PERSON_ID }),
  router: { push: vi.fn() },
  Stack: { Screen: () => null },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff",
      textMuted: "#888", accent: "#f00", onAccent: "#fff", border: "#333",
    },
    radii: { sm: 4, md: 8 },
    spacing: { sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));

vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));

const PERSON_ID = "person-1";
const FILM_COUNT = 60;
const person = { id: PERSON_ID, name: "Jane Actor" };
const allFilms = Array.from({ length: FILM_COUNT }, (_, i) => ({
  id: `film-${i + 1}`,
  title: `Film ${i + 1}`,
  year: 2000 + i,
}));
const PAGE1 = allFilms.slice(0, 30);
const PAGE2 = allFilms.slice(30, 60);

// STABLE responses per cursor: the real react-query hook memoizes `data`, so a
// fresh object every render would make the accumulation effect loop forever
// (prevDataRef compares by reference). Build each page's result once.
const REFETCH = vi.fn();
const PAGE1_RESULT = {
  data: { items: PAGE1, pagination: { totalItems: FILM_COUNT, nextCursor: "c1" } },
  isLoading: false, isFetching: false, isError: false, refetch: REFETCH,
};
const PAGE2_RESULT = {
  data: { items: PAGE2, pagination: { totalItems: FILM_COUNT, nextCursor: "" } },
  isLoading: false, isFetching: false, isError: false, refetch: REFETCH,
};

let eveyFetchImpl: () => Promise<unknown> = () => Promise.resolve(person);
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: () => eveyFetchImpl(),
  useLibraryServiceListMediaItems: (params: Record<string, unknown>) =>
    params["pagination.cursor"] === "c1" ? PAGE2_RESULT : PAGE1_RESULT,
}));

import PersonDetailScreen from "../../app/(auth)/person/[id]";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetchImpl = () => Promise.resolve(person);
});

describe("PersonDetailScreen filmography", () => {
  it("streams the first keyset page and shows the honest total in the heading", async () => {
    render(<PersonDetailScreen />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Film 1")).toBeTruthy();
    // Heading count uses the server total, not the loaded length.
    expect(screen.getByText(`media.person.filmography (${FILM_COUNT})`)).toBeTruthy();
    expect(screen.getByText("Film 30")).toBeTruthy();
    // Page 2 not loaded until the grid reaches its end.
    expect(screen.queryByText("Film 31")).toBeNull();
  });

  it("appends the next page when the grid scrolls to its end", async () => {
    render(<PersonDetailScreen />, { wrapper: makeWrapper() });
    await screen.findByText("Film 1");

    fireEvent.click(screen.getByTestId("end-reached"));

    await waitFor(() => expect(screen.getByText(`Film ${FILM_COUNT}`)).toBeTruthy());
    // Earlier page stays mounted (accumulated, not replaced).
    expect(screen.getByText("Film 1")).toBeTruthy();
    expect(screen.getByText("Film 31")).toBeTruthy();
  });

  it("offers a retry instead of an empty profile when the person fetch fails", async () => {
    eveyFetchImpl = () => Promise.reject(new Error("boom"));

    render(<PersonDetailScreen />, { wrapper: makeWrapper() });

    expect(await screen.findByLabelText("common.retry")).toBeTruthy();
    // Not silently rendered as a personless "Unknown" profile.
    expect(screen.queryByText("Film 1")).toBeNull();
  });
});
