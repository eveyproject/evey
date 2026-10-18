import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen, waitFor } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ onChangeText, value, accessibilityLabel, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value,
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) => {
    if (!data || data.length === 0) return null;
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
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

// --- @expo/vector-icons mock ------------------------------------------------
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: (p: any) => React.createElement("span", { "data-icon": p.name }),
}));

// --- expo-image mock -----------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- server url / auth headers hooks mocks --------------------------------------
vi.mock("../../../src/hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv.test",
}));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => undefined,
}));

// --- expo-router mock --------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

// --- react-i18next mock -------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

// --- ThemeProvider mock --------------------------------------------------------
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
      shadow: "#000",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// --- useDebounce mock: no debounce delay in tests ------------------------------
vi.mock("../../../src/hooks/useDebounce", () => ({
  useDebounce: (v: string) => v,
}));

// --- @evey/api mock ------------------------------------------------------------
const searchMediaMock = vi.fn();
const searchMusicMock = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceSearchMedia: (...args: unknown[]) => searchMediaMock(...args),
  useMusicServiceSearchMusic: (...args: unknown[]) => searchMusicMock(...args),
}));

// --- @evey/voice-search mock -----------------------------------------------
const startVoiceSearchMock = vi.fn();
vi.mock("@evey/voice-search", () => ({
  startVoiceSearch: (...args: unknown[]) => startVoiceSearchMock(...args),
}));

import SearchScreen, { selectSearchResultsView } from "./search";

const MEDIA_ITEM = {
  id: "mi-1",
  title: "Movie One",
  year: 2020,
  type: "MEDIA_ITEM_TYPE_MOVIE",
  genres: ["Action", "Thriller", "Drama"],
};

beforeEach(() => {
  vi.clearAllMocks();
  searchMediaMock.mockReturnValue({ data: { items: [] }, isLoading: false, isError: false, refetch: vi.fn() });
  searchMusicMock.mockReturnValue({ data: { tracks: [] }, isLoading: false });
  startVoiceSearchMock.mockResolvedValue({ status: "unavailable" });
});

describe("selectSearchResultsView (initial TV remote focus)", () => {
  const base = {
    searchKind: "media" as const,
    isError: false,
    activeLoading: false,
    hasQuery: true,
    musicItemsCount: 0,
    mediaItemsCount: 0,
  };

  it("shows the error view (with its own retry focus target) before anything else, for the media scope", () => {
    expect(selectSearchResultsView({ ...base, isError: true })).toBe("error");
  });

  it("shows the error view for a failed MUSIC search too - a failure must not pose as 'no results'", () => {
    expect(selectSearchResultsView({ ...base, searchKind: "music", isError: true })).toBe("error");
  });

  it("shows the prompt view when there is no query yet, regardless of scope", () => {
    expect(selectSearchResultsView({ ...base, hasQuery: false })).toBe("prompt");
    expect(selectSearchResultsView({ ...base, searchKind: "music", hasQuery: false })).toBe("prompt");
  });

  it("shows the loading view while the active scope's query is in flight", () => {
    expect(selectSearchResultsView({ ...base, activeLoading: true })).toBe("loading");
  });

  it("picks exactly one results/empty view for the active scope - never both scopes at once", () => {
    expect(selectSearchResultsView({ ...base, mediaItemsCount: 3 })).toBe("mediaResults");
    expect(selectSearchResultsView({ ...base, mediaItemsCount: 0 })).toBe("mediaEmpty");
    expect(selectSearchResultsView({ ...base, searchKind: "music", musicItemsCount: 2 })).toBe("musicResults");
    expect(selectSearchResultsView({ ...base, searchKind: "music", musicItemsCount: 0 })).toBe("musicEmpty");
  });
});

describe("SearchScreen (TV) error state", () => {
  it("renders a focusable retry action so the remote is never stuck on a load failure", () => {
    const refetchMock = vi.fn();
    searchMediaMock.mockReturnValue({ data: { items: [] }, isLoading: false, isError: true, refetch: refetchMock });
    render(<SearchScreen />);
    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "abc" } });
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetchMock).toHaveBeenCalled();
  });

  it("uses the translated placeholder as the search input's accessibility label", () => {
    render(<SearchScreen />);
    expect(screen.getByLabelText("media.search.placeholder")).toBeTruthy();
  });
});

describe("SearchScreen (TV) results", () => {
  it("renders media results", () => {
    searchMediaMock.mockReturnValue({ data: { items: [MEDIA_ITEM] }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<SearchScreen />);
    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "movie" } });
    expect(screen.getByText("Movie One")).toBeTruthy();
  });

  it("shows year, a type badge and up to two genres per row, dropdown-style", () => {
    searchMediaMock.mockReturnValue({ data: { items: [MEDIA_ITEM] }, isLoading: false, isError: false, refetch: vi.fn() });
    render(<SearchScreen />);
    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "movie" } });
    expect(screen.getByText("2020")).toBeTruthy();
    expect(screen.getByText("media.itemType.movie")).toBeTruthy();
    expect(screen.getByText("Action, Thriller")).toBeTruthy();
    expect(screen.queryByText("Action, Thriller, Drama")).toBeNull();
  });

  it("renders the poster thumbnail against the configured server", () => {
    searchMediaMock.mockReturnValue({ data: { items: [MEDIA_ITEM] }, isLoading: false, isError: false, refetch: vi.fn() });
    const { container } = render(<SearchScreen />);
    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "movie" } });
    const poster = container.querySelector("img") as HTMLImageElement;
    expect(poster?.src).toContain("http://srv.test/api/v1/media/mi-1/artwork/poster");
  });
});

describe("SearchScreen (TV) voice search", () => {
  it("renders a focusable voice search target next to the keyboard input", () => {
    render(<SearchScreen />);
    expect(screen.getByLabelText("media.search.voiceSearch")).toBeTruthy();
    // The keyboard path is untouched by the voice button merely existing.
    expect(screen.getByLabelText("media.search.placeholder")).toBeTruthy();
  });

  it("does nothing when there is no recognizer - module absent, no device support, or the OS permission dialog was declined", async () => {
    startVoiceSearchMock.mockResolvedValue({ status: "unavailable" });
    render(<SearchScreen />);
    fireEvent.click(screen.getByLabelText("media.search.voiceSearch"));
    await waitFor(() => expect(startVoiceSearchMock).toHaveBeenCalledTimes(1));
    expect(screen.queryByText("Movie One")).toBeNull();
    // The keyboard still works after a failed voice attempt.
    fireEvent.change(screen.getByLabelText("media.search.placeholder"), { target: { value: "movie" } });
    expect((screen.getByLabelText("media.search.placeholder") as HTMLInputElement).value).toBe("movie");
  });

  it("does nothing on cancellation or an unrecognized result", async () => {
    startVoiceSearchMock.mockResolvedValue({ status: "empty" });
    render(<SearchScreen />);
    fireEvent.click(screen.getByLabelText("media.search.voiceSearch"));
    await waitFor(() => expect(startVoiceSearchMock).toHaveBeenCalledTimes(1));
    expect((screen.getByLabelText("media.search.placeholder") as HTMLInputElement).value).toBe("");
  });

  it("fills the search field with the recognized text and lets the existing search run", async () => {
    searchMediaMock.mockReturnValue({ data: { items: [MEDIA_ITEM] }, isLoading: false, isError: false, refetch: vi.fn() });
    startVoiceSearchMock.mockResolvedValue({ status: "recognized", text: "movie one" });
    render(<SearchScreen />);
    fireEvent.click(screen.getByLabelText("media.search.voiceSearch"));
    await waitFor(() => {
      expect((screen.getByLabelText("media.search.placeholder") as HTMLInputElement).value).toBe("movie one");
    });
    expect(screen.getByText("Movie One")).toBeTruthy();
  });
});
