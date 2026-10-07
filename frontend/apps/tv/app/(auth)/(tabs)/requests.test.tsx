import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, screen } from "@testing-library/react";
import React from "react";

// --- React Native mock -----------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ onChangeText, onSubmitEditing, value, accessibilityLabel, ...p }: any) =>
    React.createElement("input", {
      "aria-label": accessibilityLabel,
      value,
      onChange: (e: any) => onChangeText?.(e.target.value),
      // RN's onSubmitEditing fires on the remote's on-screen keyboard "done"/
      // "search" action, mirrored here as Enter so it stays testable in jsdom.
      onKeyDown: (e: any) => { if (e.key === "Enter") onSubmitEditing?.(); },
      ...p,
    }),
  // hasTVPreferredFocus is surfaced as a data-* attribute: React silently
  // drops boolean values passed through an unrecognized DOM prop, so passing
  // it through the ...p spread as-is would make it unobservable in jsdom -
  // a real gap this suite needs to assert on.
  Pressable: ({ children, onPress, accessibilityLabel, hasTVPreferredFocus, disabled, ...p }: any) =>
    React.createElement(
      "button",
      {
        onClick: disabled ? undefined : onPress,
        disabled,
        "aria-label": accessibilityLabel,
        "data-tv-preferred-focus": hasTVPreferredFocus ? "true" : undefined,
        ...p,
      },
      children,
    ),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) => {
    if (!data || data.length === 0) {
      return ListEmptyComponent ? React.createElement(ListEmptyComponent) : null;
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
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {} },
}));

// --- expo-image mock ---------------------------------------------------------
vi.mock("expo-image", () => ({
  Image: (props: any) => React.createElement("img", { alt: "", src: props.source?.uri }),
}));

// --- @expo/vector-icons mock ------------------------------------------------
vi.mock("@expo/vector-icons", () => ({
  Ionicons: (p: any) => React.createElement("span", { "data-icon": p.name }),
}));

// --- expo-router mock --------------------------------------------------------
const pushMock = vi.fn();
vi.mock("expo-router", () => ({
  useRouter: () => ({ push: pushMock }),
}));

// --- react-i18next mock -------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, opts?: any) => (typeof opts === "string" ? opts : key) }),
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
      overlay: "rgba(0,0,0,0.5)",
      success: "#0f0",
      danger: "#f33",
      warning: "#ff0",
    },
    radii: { sm: 4, md: 8 },
  }),
}));

// --- @tanstack/react-query mock ------------------------------------------------
const invalidateQueriesMock = vi.fn();
vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({ invalidateQueries: invalidateQueriesMock }),
}));

// --- @evey/store mock ------------------------------------------------------------
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }),
}));

// --- @evey/api mock --------------------------------------------------------------
const policyMock = vi.fn();
const myRequestsMock = vi.fn();
const searchTitlesMock = vi.fn();
const createRequestMock = vi.fn();
const mutateMock = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  // The date and rating formatters are pure: keep the real ones.
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMediaRequestServiceGetRequestPolicy: (...args: unknown[]) => policyMock(...args),
  useMediaRequestServiceListMyRequests: (...args: unknown[]) => myRequestsMock(...args),
  useMediaRequestServiceSearchTitles: (...args: unknown[]) => searchTitlesMock(...args),
  useMediaRequestServiceCreateRequest: (...args: unknown[]) => createRequestMock(...args),
  getMediaRequestServiceListMyRequestsQueryKey: () => ["media-requests", "mine"],
}));

import RequestsScreen from "./requests";

const MOVIE_CANDIDATE = { provider: "tmdb", providerId: "42", title: "Movie One", year: 2020, mediaType: "movie" };

beforeEach(() => {
  vi.clearAllMocks();
  policyMock.mockReturnValue({ data: { canRequest: true }, isLoading: false });
  myRequestsMock.mockReturnValue({ data: { items: [] }, isLoading: false });
  searchTitlesMock.mockReturnValue({ data: { results: [] }, isFetching: false });
  createRequestMock.mockReturnValue({ mutate: mutateMock, isPending: false });
});

describe("RequestsScreen (TV) disabled state", () => {
  it("gives the remote a focusable way out instead of a dead end when requests are disabled", () => {
    policyMock.mockReturnValue({ data: { canRequest: false }, isLoading: false });
    render(<RequestsScreen />);
    expect(screen.getByText("requests.disabledTitle")).toBeTruthy();
    const libraryBtn = screen.getByText("media.library.libraries").closest("button");
    expect(libraryBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");
    fireEvent.click(libraryBtn!);
    expect(pushMock).toHaveBeenCalledWith("/(auth)/(tabs)/library");
  });
});

describe("RequestsScreen (TV) initial focus", () => {
  it("claims the initial remote focus on the Search tab, the topmost interactive element", () => {
    render(<RequestsScreen />);
    const searchTab = screen.getByText("requests.tabSearch").closest("button");
    expect(searchTab?.getAttribute("data-tv-preferred-focus")).toBe("true");
  });
});

describe("RequestsScreen (TV) search flow", () => {
  it("submits a search on the remote's enter action and renders a focusable request button", () => {
    searchTitlesMock.mockImplementation(({ mediaType }: { mediaType: string }) => {
      if (mediaType === "movie") return { data: { results: [MOVIE_CANDIDATE] }, isFetching: false };
      return { data: { results: [] }, isFetching: false };
    });
    render(<RequestsScreen />);

    const input = screen.getByLabelText("requests.searchPlaceholder");
    fireEvent.change(input, { target: { value: "movie" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(screen.getByText("Movie One (2020)")).toBeTruthy();
    const requestBtn = screen.getByText("requests.request").closest("button");
    expect(requestBtn?.getAttribute("data-tv-preferred-focus")).toBe("true");

    fireEvent.click(requestBtn!);
    expect(mutateMock).toHaveBeenCalledWith({
      data: {
        mediaType: "movie",
        provider: "tmdb",
        providerId: "42",
        title: "Movie One",
        year: 2020,
        posterUrl: undefined,
      },
    });
  });

  it("marks an already-requested candidate as requested and disables the action", () => {
    searchTitlesMock.mockImplementation(({ mediaType }: { mediaType: string }) => {
      if (mediaType === "movie") return { data: { results: [MOVIE_CANDIDATE] }, isFetching: false };
      return { data: { results: [] }, isFetching: false };
    });
    myRequestsMock.mockReturnValue({
      data: { items: [{ mediaType: "movie", provider: "tmdb", providerId: "42" }] },
      isLoading: false,
    });
    render(<RequestsScreen />);

    fireEvent.change(screen.getByLabelText("requests.searchPlaceholder"), { target: { value: "movie" } });
    fireEvent.keyDown(screen.getByLabelText("requests.searchPlaceholder"), { key: "Enter" });

    const requestedBtn = screen.getByText("requests.requested").closest("button");
    expect(requestedBtn?.hasAttribute("disabled")).toBe(true);
  });
});

describe("RequestsScreen (TV) my requests tab", () => {
  it("switches to the Mine tab and lists the user's own requests with their status", () => {
    myRequestsMock.mockReturnValue({
      data: { items: [{ id: "req-1", title: "My Show", status: "approved" }] },
      isLoading: false,
    });
    render(<RequestsScreen />);

    fireEvent.click(screen.getByText("requests.tabMine"));

    expect(screen.getByText("My Show")).toBeTruthy();
    expect(screen.getByText("requests.statusApproved")).toBeTruthy();
  });
});
