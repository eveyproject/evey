import { describe, it, expect, beforeEach, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// A prolific actor's filmography used to .map() every credited title straight
// into the grid - up to 500 posters (the fetch cap) mounted on first paint.
// This guards the fix: the render is paged (slice + "Show more"), matching
// the pattern already applied to the TV filmography grid
// (frontend/apps/tv/app/(auth)/person/[id].tsx).

// --- React Native mock ----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ScrollView: ({ children, ...p }: any) => React.createElement("div", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Dimensions: { get: () => ({ width: 390, height: 844 }) },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- expo-image -------------------------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) => React.createElement("img", { ...props, alt: "" }),
}));

// --- expo-router ---------------------------------------------------------------------
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: PERSON_ID }),
  router: { push: vi.fn() },
  Stack: { Screen: () => null },
}));

// --- i18next --------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

// --- ThemeProvider --------------------------------------------------------------------
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

// --- misc hooks -------------------------------------------------------------------------
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));

// --- @evey/api -------------------------------------------------------------------------
const PERSON_ID = "person-1";
const FILM_COUNT = 60;

const person = { id: PERSON_ID, name: "Jane Actor" };
const allFilms = Array.from({ length: FILM_COUNT }, (_, i) => ({
  id: `film-${i + 1}`,
  title: `Film ${i + 1}`,
  year: 2000 + i,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: () => Promise.resolve(person),
  useLibraryServiceListMediaItems: () => ({
    data: { items: allFilms },
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
}));

// -------------------------------------------------------------------------------------
import PersonDetailScreen from "../../app/(auth)/person/[id]";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PersonDetailScreen filmography", () => {
  it("renders only the first page of a large filmography", async () => {
    render(<PersonDetailScreen />, { wrapper: makeWrapper() });

    expect(await screen.findByText("Film 1")).toBeTruthy();
    expect(screen.queryByText("Film 25")).toBeNull();
  });

  it("reveals more titles on 'Show more' tap, up to the full filmography", async () => {
    render(<PersonDetailScreen />, { wrapper: makeWrapper() });
    await screen.findByText("Film 1");

    fireEvent.click(screen.getByText("Show more"));

    expect(screen.getByText("Film 48")).toBeTruthy();
    expect(screen.queryByText("Film 49")).toBeNull();

    fireEvent.click(screen.getByText("Show more"));
    expect(screen.getByText(`Film ${FILM_COUNT}`)).toBeTruthy();
    expect(screen.queryByText("Show more")).toBeNull();
  });
});
