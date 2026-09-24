import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const searchMedia = vi.fn();
const searchMusic = vi.fn();
const libraries = vi.fn();
const replaceMock = vi.fn();
const navigateMock = vi.fn();
const setQueue = vi.fn();

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a), replace: (...a: unknown[]) => replaceMock(...a) }));
const authState = { accessToken: "t" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListLibraries: () => libraries(),
  useLibraryServiceSearchMedia: (...a: unknown[]) => searchMedia(...a),
  useMusicServiceSearchMusic: (...a: unknown[]) => searchMusic(...a),
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useDebounce: (v: string) => v,
}));
vi.mock("../music/queue", () => ({
  useMusicQueueStore: (sel: any) => sel({ setQueue }),
  trackToQueueTrack: (t: any) => ({ id: t.id, title: t.title, artist: t.artist ?? "", album: "", duration: 0 }),
}));

import { SearchScreen } from "./Search";

beforeEach(() => {
  setQueue.mockReset();
  libraries.mockReturnValue({ data: { libraries: [{ id: "lib-m", type: "LIBRARY_TYPE_MUSIC" }] } });
  searchMedia.mockReturnValue({ data: { items: [{ id: "m-1", title: "A film" }] }, isLoading: false, isError: false });
  searchMusic.mockReturnValue({ data: { tracks: [{ id: "t-1", title: "A song", artist: "Someone" }] }, isLoading: false, isError: false });
});

describe("search", () => {
  it("does not ask the server before there is something to look for", () => {
    render(<SearchScreen />);
    expect(searchMedia).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ query: expect.objectContaining({ enabled: false }) }));
  });

  it("asks only the scope in front of the viewer", () => {
    render(<SearchScreen initialQuery="blade" />);
    expect(searchMedia).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ query: expect.objectContaining({ enabled: true }) }));
    expect(searchMusic).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ query: expect.objectContaining({ enabled: false }) }));
    fireEvent.click(screen.getByText("media.search.sectionMusic"));
    expect(searchMusic).toHaveBeenLastCalledWith(expect.anything(), expect.objectContaining({ query: expect.objectContaining({ enabled: true }) }));
  });

  it("offers both scopes, as the Android television does", () => {
    libraries.mockReturnValue({ data: { libraries: [] } });
    render(<SearchScreen initialQuery="blade" />);
    expect(screen.getByText("media.search.sectionMedia")).toBeTruthy();
    expect(screen.getByText("media.search.sectionMusic")).toBeTruthy();
  });

  it("a failed search says so and offers to try again, rather than posing as no results", () => {
    const refetch = vi.fn();
    searchMedia.mockReturnValue({ data: undefined, isLoading: false, isError: true, refetch });
    render(<SearchScreen initialQuery="blade" />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetch).toHaveBeenCalled();
  });

  it("opens a title from its row", () => {
    render(<SearchScreen initialQuery="film" />);
    fireEvent.click(screen.getByLabelText("A film"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "media", mediaId: "m-1" });
  });

  it("playing a track from the results starts the queue on it", () => {
    render(<SearchScreen initialQuery="song" />);
    fireEvent.click(screen.getByText("media.search.sectionMusic"));
    fireEvent.click(screen.getByLabelText("A song"));
    expect(setQueue).toHaveBeenCalledWith([expect.objectContaining({ id: "t-1" })], 0);
  });
});
