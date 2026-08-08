import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops a boolean value passed through an unrecognized DOM prop, so
  // spreading it through as-is would make it unobservable in jsdom.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, ...p }: any) =>
    React.createElement(
      "button",
      { onClick: onPress, "aria-label": accessibilityLabel, "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined, ...p },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(React.Fragment, null, ListEmptyComponent) : null;
    }
    return React.createElement(
      "div",
      null,
      data.map((item: any, index: number) =>
        React.createElement(
          React.Fragment,
          { key: keyExtractor ? keyExtractor(item, index) : index },
          renderItem({ item, index }),
        ),
      ),
    );
  },
  StyleSheet: { create: (s: any) => s },
}));

// --- expo-router mock ---------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

// --- react-i18next mock --------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- ThemeProvider mock ---------------------------------------------------------
vi.mock("../../../src/providers/ThemeProvider", () => ({
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
    },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- @evey/api mock: importOriginal keeps hasMediaItemCount (and every other
// pure helper) real, so this screen cannot pass against a stub that
// disagrees with shipped dedicated-route rules. Only the hooks/fetchers are
// faked. -------------------------------------------------------------------
const { listLibrariesMock, listGenresMock, genreRefetchMock } = vi.hoisted(() => ({
  genreRefetchMock: vi.fn(),
  listLibrariesMock: vi.fn(),
  listGenresMock: vi.fn(),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: (...args: unknown[]) => listLibrariesMock(...args),
  libraryServiceListGenres: (...args: unknown[]) => listGenresMock(...args),
}));

// --- @tanstack/react-query: only useQueries needs faking here, so a plain
// synchronous resolver stands in for the per-library ListGenres fan-out
// without pulling a QueryClientProvider into every test. -------------------
// A queryFn that throws stands for a failed request: the fake used to return
// only { data, isLoading }, so a failure was indistinguishable from an empty
// result and the screen's error path could not be exercised at all - which is
// precisely how a failing library came to vanish from the picker unnoticed.
vi.mock("@tanstack/react-query", () => ({
  useQueries: ({ queries }: { queries: Array<{ queryFn: () => any }> }) =>
    queries.map((q) => {
      try {
        return { data: q.queryFn(), isLoading: false, isError: false, refetch: genreRefetchMock };
      } catch {
        return { data: undefined, isLoading: false, isError: true, refetch: genreRefetchMock };
      }
    }),
}));

import GenresScreen from "./index";

const LIBRARIES = [
  { id: "lib-1", name: "Movies", type: "LIBRARY_TYPE_MOVIES" },
  { id: "lib-2", name: "Shows", type: "LIBRARY_TYPE_SHOWS" },
  { id: "lib-3", name: "Songs", type: "LIBRARY_TYPE_MUSIC" },
];

beforeEach(() => {
  vi.clearAllMocks();
});

describe("GenresScreen (TV) loading/error", () => {
  it("gives the remote a focusable retry action instead of leaving it with nowhere to go", () => {
    const refetchMock = vi.fn();
    listLibrariesMock.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch: refetchMock });
    render(<GenresScreen />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetchMock).toHaveBeenCalled();
  });
});

describe("GenresScreen (TV) empty state", () => {
  // A ListGenres failure for ONE library used to be swallowed: mergeGenres sees
  // an undefined list and treats it exactly like "this library has no genres",
  // so the picker silently came up short with no error and no way to retry.
  it("reports a failure instead of quietly dropping that library's genres", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: LIBRARIES }, isLoading: false, isError: false, refetch: vi.fn() });
    listGenresMock.mockImplementation((libId: string) => {
      if (libId === "lib-2") throw new Error("network down");
      return { genres: ["Comedy"] };
    });

    render(<GenresScreen />);

    expect(screen.getByText("common.loadError")).toBeTruthy();
    // And the genres that DID load must not be presented as the whole set.
    expect(screen.queryByText("Comedy")).toBeNull();
  });

  it("retries the per-library genre calls, not just the library list", () => {
    genreRefetchMock.mockClear();
    const librariesRefetch = vi.fn();
    listLibrariesMock.mockReturnValue({ data: { libraries: LIBRARIES }, isLoading: false, isError: false, refetch: librariesRefetch });
    listGenresMock.mockImplementation(() => { throw new Error("network down"); });

    render(<GenresScreen />);
    fireEvent.click(screen.getByText("common.retry"));

    expect(genreRefetchMock).toHaveBeenCalled();
  });

  it("stays focusable when no library has any genre yet", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: [LIBRARIES[0]] }, isLoading: false, isError: false, refetch: vi.fn() });
    listGenresMock.mockReturnValue({ genres: [] });
    render(<GenresScreen />);
    expect(screen.getByText("common.noGenres")).toBeTruthy();
    const libBtn = screen.getByText("media.library.libraries").closest("button");
    expect(libBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });

  it("stays focusable when there are zero browsable libraries at all", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: [] }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<GenresScreen />);
    expect(screen.getByText("common.noGenres")).toBeTruthy();
    const libBtn = screen.getByText("media.library.libraries").closest("button");
    expect(libBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("GenresScreen (TV) genre picker", () => {
  it("merges genres across every browsable library, skips music, dedupes, and sorts", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: LIBRARIES }, isLoading: false, isError: false, refetch: vi.fn() });
    listGenresMock.mockImplementation((libId: string) => {
      if (libId === "lib-1") return { genres: ["Drama", "Sci-Fi"] };
      if (libId === "lib-2") return { genres: ["drama", "Comedy"] };
      // A music library must never be asked for genres (hasMediaItemCount is false for it).
      throw new Error(`should not fetch genres for ${libId}`);
    });
    render(<GenresScreen />);
    const tiles = screen.getAllByText(/./, { selector: "span" });
    const labels = tiles.map((el) => el.textContent);
    expect(labels).toContain("Comedy");
    expect(labels).toContain("Drama");
    expect(labels).toContain("Sci-Fi");
    // deduped: "Drama" from lib-1 and "drama" from lib-2 collapse into one tile.
    expect(labels.filter((l) => l?.toLowerCase() === "drama")).toHaveLength(1);
  });

  it("puts the initial remote focus on the first genre tile", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: [LIBRARIES[0]] }, isLoading: false, isError: false, refetch: vi.fn() });
    listGenresMock.mockReturnValue({ genres: ["Comedy", "Action"] });
    render(<GenresScreen />);
    const firstTile = screen.getByText("Action").closest("button");
    expect(firstTile?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });

  it("navigates to the encoded genre route on press", () => {
    listLibrariesMock.mockReturnValue({ data: { libraries: [LIBRARIES[0]] }, isLoading: false, isError: false, refetch: vi.fn() });
    listGenresMock.mockReturnValue({ genres: ["Sci-Fi"] });
    render(<GenresScreen />);
    fireEvent.click(screen.getByText("Sci-Fi"));
    expect(pushMock).toHaveBeenCalledWith("/(auth)/genres/Sci-Fi");
  });
});
