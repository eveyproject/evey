import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const { policy, mine, movies, series, music, createMutate, navigateMock } = vi.hoisted(() => ({
  policy: vi.fn(), mine: vi.fn(), movies: vi.fn(), series: vi.fn(), music: vi.fn(),
  createMutate: vi.fn(), navigateMock: vi.fn(),
}));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a), goBack: vi.fn() }));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() }, useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }) }));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useMediaRequestServiceGetRequestPolicy: () => policy(),
  useMediaRequestServiceListMyRequests: () => mine(),
  useMediaRequestServiceSearchTitles: (params: any, opts: any) => {
    if (params.mediaType === "movie") return movies(opts);
    if (params.mediaType === "tv") return series(opts);
    return music(opts);
  },
  useMediaRequestServiceCreateRequest: () => ({ mutate: createMutate }),
  getMediaRequestServiceListMyRequestsQueryKey: () => ["requests"],
}));

import { RequestsScreen } from "./Requests";

const idle = { data: undefined, isFetching: false };

// The field's Enter searches, as the Android television's does.
function search(text: string) {
  const field = screen.getByPlaceholderText("requests.searchPlaceholder");
  fireEvent.change(field, { target: { value: text } });
  fireEvent.keyDown(field, { key: "Enter", keyCode: 13 });
}

beforeEach(() => {
  createMutate.mockReset();
  navigateMock.mockReset();
  policy.mockReturnValue({ data: { canRequest: true }, isLoading: false });
  mine.mockReturnValue({ data: { items: [] }, isLoading: false });
  movies.mockReturnValue({ data: { results: [{ provider: "tmdb", providerId: "1", title: "Film one" }] }, isFetching: false });
  series.mockReturnValue({ data: { results: [{ provider: "tmdb", providerId: "2", title: "Series one" }] }, isFetching: false });
  music.mockReturnValue(idle);
});

describe("media requests", () => {
  it("says so when the server does not allow asking, and asks nothing", () => {
    policy.mockReturnValue({ data: { canRequest: false }, isLoading: false });
    render(<RequestsScreen />);
    expect(screen.getByText("requests.disabledTitle")).toBeTruthy();
    fireEvent.click(screen.getByText("media.library.libraries"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "libraries" });
    expect(screen.queryByText("requests.searchPrompt")).toBeNull();
  });

  it("interleaves films and series so neither owns the top of the list", () => {
    render(<RequestsScreen />);
    search("one");
    const titles = Array.from(document.querySelectorAll(".req-title")).map((c) => c.textContent);
    expect(titles).toEqual(["Film one", "Series one"]);
  });

  it("marks what is already in the library and opens it instead of asking again", () => {
    movies.mockReturnValue({ data: { results: [{ provider: "tmdb", providerId: "1", title: "Owned", inLibrary: true, mediaItemId: "m-9" }] }, isFetching: false });
    series.mockReturnValue({ data: { results: [] }, isFetching: false });
    render(<RequestsScreen />);
    search("owned");
    expect(screen.getByText("requests.inLibrary")).toBeTruthy();
    fireEvent.click(screen.getByText("requests.watchNow"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "media", mediaId: "m-9" });
    expect(createMutate).not.toHaveBeenCalled();
  });

  it("does not ask twice for the same title", () => {
    mine.mockReturnValue({ data: { items: [{ mediaType: "movie", provider: "tmdb", providerId: "1", title: "Film one" }] }, isLoading: false });
    series.mockReturnValue({ data: { results: [] }, isFetching: false });
    render(<RequestsScreen />);
    search("one");
    fireEvent.click(screen.getByText("requests.requested"));
    expect(createMutate).not.toHaveBeenCalled();
  });

  it("sends what the provider gave, so the server can fetch the right title", () => {
    series.mockReturnValue({ data: { results: [] }, isFetching: false });
    render(<RequestsScreen />);
    search("one");
    fireEvent.click(screen.getByText("requests.request"));
    expect(createMutate).toHaveBeenCalledWith({ data: expect.objectContaining({ mediaType: "movie", provider: "tmdb", providerId: "1", title: "Film one" }) });
  });
});
