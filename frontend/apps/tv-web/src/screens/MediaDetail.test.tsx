import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const getMediaItem = vi.fn();
const listMediaItems = vi.fn();
const navigateMock = vi.fn();
const markWatched = vi.fn();
const markUnwatched = vi.fn();
const toggleFavorite = vi.fn();

const needsPin = vi.fn(() => false);

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  useBackHandler: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k, i18n: { language: "en" } }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a), replace: vi.fn(), goBack: vi.fn() }));
vi.mock("../components/ParentalPINModal", () => ({
  ParentalPINModal: ({ onVerified }: any) => <button type="button" onClick={() => onVerified()}>pin-modal</button>,
}));
vi.mock("../plugins/PluginSlot", () => ({ PluginSlot: () => null }));
const authState = { profileId: "prf-1", accessToken: "t" };
const demoState = { demoMode: false };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
  useDemoStore: (sel: any) => sel(demoState),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: (...a: unknown[]) => getMediaItem(...a),
  useLibraryServiceListMediaItems: (...a: unknown[]) => listMediaItems(...a),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => {
    watchProgressArgs = args;
    return { data: { positionMs: "0" } };
  },
  usePlaybackServiceMarkAsWatched: () => ({ mutate: markWatched, isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: markUnwatched, isPending: false }),
  useFeedServiceSimilarItems: () => ({ data: { items: [] } }),
  useRatingServiceGetMyThumbRating: () => ({ data: 0 }),
  ratingServicePutThumbRating: vi.fn(),
  getRatingServiceGetMyThumbRatingQueryKey: () => ["rating"],
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useIsFavorite: () => ({ data: false }),
  useToggleFavorite: () => ({ mutate: toggleFavorite }),
  useIsWatched: () => false,
  usePlaylistList: () => ({ data: [] }),
  useAddToPlaylist: () => ({ mutate: vi.fn() }),
  useMediaItemCollections: () => ({ data: { collections: [] } }),
  useParentalGate: () => ({ needsPin, profileId: "prf-1", settingsLoading: false }),
}));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
  useMutation: () => ({ mutate: vi.fn() }),
}));

import { MediaDetailScreen } from "./MediaDetail";

const MOVIE = { id: "m-1", title: "A film", type: "MEDIA_ITEM_TYPE_MOVIE", streams: [] };
const SERIES = { id: "s-1", title: "A series", type: "MEDIA_ITEM_TYPE_SERIES", streams: [] };

beforeEach(() => {
  navigateMock.mockReset();
  getMediaItem.mockReturnValue({ data: MOVIE, isLoading: false, isError: false });
  listMediaItems.mockReturnValue({ data: { items: [] } });
  needsPin.mockReturnValue(false);
});

let watchProgressArgs: unknown[] = [];

describe("the title page", () => {
  // 27-09: a Resume label served from cache outlived the autoplay that moved
  // on from its episode; it is read again every time the page is shown.
  it("reads the watch progress with no stale window", () => {
    render(<MediaDetailScreen mediaId="m-1" />);
    const opts = watchProgressArgs[2] as { query?: { staleTime?: number } };
    expect(opts?.query?.staleTime).toBe(0);
  });

  // The demo refuses favourites, watched marks, playlists and ratings on the
  // shared account: the page does not offer them there, and Play stays.
  it("offers only what the demo allows", () => {
    demoState.demoMode = true;
    try {
      render(<MediaDetailScreen mediaId="m-1" />);
      expect(screen.getByText("player.controls.play")).toBeTruthy();
      for (const label of ["media.detail.favorite", "media.detail.markWatched", "media.detail.addToPlaylist", "media.rating.like", "media.rating.dislike"]) {
        expect(screen.queryByText(label)).toBeNull();
      }
    } finally {
      demoState.demoMode = false;
    }
  });

  it("plays the title itself for a film", () => {
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("player.controls.play"));
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "player", mediaId: "m-1" }));
  });

  // A series owns no file: streaming its own id can only fail, which is what
  // the televisions learned on a real device.
  it("plays the first episode of a series, in episode order", () => {
    getMediaItem.mockReturnValue({ data: SERIES, isLoading: false, isError: false });
    listMediaItems.mockImplementation((params: any) => {
      if (params?.parentId === "s-1") return { data: { items: [{ id: "season-1", indexNumber: 1 }] } };
      return { data: { items: [
        { id: "ep-2", indexNumber: 2, title: "Second" },
        { id: "ep-1", indexNumber: 1, title: "First" },
      ] } };
    });
    render(<MediaDetailScreen mediaId="s-1" />);
    fireEvent.click(screen.getByText("player.controls.play"));
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "player", mediaId: "ep-1" }));
  });

  it("carries the audio and subtitle chosen before playing", () => {
    getMediaItem.mockReturnValue({
      data: {
        ...MOVIE,
        streams: [
          { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng" },
          { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", language: "fra", channels: 2 },
          { index: 3, type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "fra" },
        ],
      },
      isLoading: false,
      isError: false,
    });
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("French - 2.0"));
    fireEvent.click(screen.getByText("French"));
    fireEvent.click(screen.getByText("player.controls.play"));
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "player", mediaId: "m-1", audio: 2, subtitle: 3 }));
  });

  // The thumbs were emoji, two empty boxes on a browser without a colour-emoji
  // font (prod, 2026-09-23); the Android television names them in words.
  it("names the thumbs in words, with no emoji", () => {
    const { container } = render(<MediaDetailScreen mediaId="m-1" />);
    expect(container.textContent).not.toMatch(/[\u{1F44D}\u{1F44E}]/u);
    expect(screen.getByText("media.rating.like")).toBeTruthy();
    expect(screen.getByText("media.rating.dislike")).toBeTruthy();
  });

  it("asks for the parental PIN before a restricted title plays", () => {
    needsPin.mockReturnValue(true);
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("player.controls.play"));
    expect(navigateMock).not.toHaveBeenCalled();
    fireEvent.click(screen.getByText("pin-modal"));
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "player", mediaId: "m-1" }));
  });

  it("marks watched against the profile in front of the viewer", () => {
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("media.detail.markWatched"));
    expect(markWatched).toHaveBeenCalledWith({ mediaItemId: "m-1", params: { profileId: "prf-1" } });
  });
});
