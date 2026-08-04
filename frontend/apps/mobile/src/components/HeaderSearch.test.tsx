import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// The two search queries here used to be destructured for `data` only, so
// "no results" showed both mid-keystroke (still fetching) and on a dead
// server - and a genuine miss looked identical to a network failure.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ value, onChangeText, onFocus, onBlur, onSubmitEditing, placeholder }: any) =>
    React.createElement("input", {
      "aria-label": placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
      onFocus,
      onBlur,
      onKeyDown: (e: React.KeyboardEvent) => { if (e.key === "Enter") onSubmitEditing?.(); },
    }),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "search-loading", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("expo-image", () => ({
  Image: ({ source, ...p }: any) => React.createElement("img", { src: source?.uri, alt: "", ...p }),
}));
vi.mock("expo-router", () => ({ router: { push: vi.fn() } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#888", accent: "#f00", surfaceRaised: "#222", border: "#333", onAccent: "#fff", shadow: "#000" },
  }),
}));

vi.mock("../hooks/useDebounce", () => ({ useDebounce: (v: unknown) => v }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => undefined }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://test-server" }));

// --- @evey/api -------------------------------------------------------------------------
let mediaState: { data?: { items: unknown[] }; isLoading: boolean; isError: boolean; refetch: () => void };
let musicState: { data?: { tracks: unknown[] }; isLoading: boolean; isError: boolean; refetch: () => void };

vi.mock("@evey/api", () => ({
  useLibraryServiceSearchMedia: () => mediaState,
  useMusicServiceSearchMusic: () => musicState,
}));

import { HeaderSearch } from "./HeaderSearch";

function settled<T>(data: T): { data: T; isLoading: boolean; isError: boolean; refetch: () => void } {
  return { data, isLoading: false, isError: false, refetch: vi.fn() };
}

beforeEach(() => {
  vi.clearAllMocks();
  mediaState = settled({ items: [] });
  musicState = settled({ tracks: [] });
});

function typeAndFocus(query: string) {
  const input = screen.getByLabelText("Search movies, series…");
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: query } });
}

describe("HeaderSearch - media scope, loading vs failure vs no results", () => {
  it("shows a loading indicator while the query is in flight, not 'no results'", () => {
    mediaState = { data: undefined, isLoading: true, isError: false, refetch: vi.fn() };
    render(<HeaderSearch />);
    typeAndFocus("batman");

    expect(screen.getByTestId("search-loading")).toBeTruthy();
    expect(screen.queryByText("No results")).toBeNull();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });

  it("shows LoadFailed with a retry, not 'no results', when the search fails", () => {
    const refetch = vi.fn();
    mediaState = { data: undefined, isLoading: false, isError: true, refetch };
    render(<HeaderSearch />);
    typeAndFocus("batman");

    expect(screen.getByText("common.loadError")).toBeTruthy();
    expect(screen.queryByText("No results")).toBeNull();

    fireEvent.click(screen.getByLabelText("common.retry"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("shows 'no results' once the search has actually completed empty", () => {
    mediaState = settled({ items: [] });
    render(<HeaderSearch />);
    typeAndFocus("zzzznotfound");

    expect(screen.getByText("No results")).toBeTruthy();
    expect(screen.queryByTestId("search-loading")).toBeNull();
    expect(screen.queryByText("common.loadError")).toBeNull();
  });
});
