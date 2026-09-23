import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const getMediaItem = vi.fn();
const listMediaItems = vi.fn();
const navigateMock = vi.fn();
const markWatched = vi.fn();
const markUnwatched = vi.fn();
const toggleFavorite = vi.fn();

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a) }));
vi.mock("../plugins/PluginSlot", () => ({ PluginSlot: () => null }));
const authState = { profileId: "prf-1", accessToken: "t" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceGetMediaItem: (...a: unknown[]) => getMediaItem(...a),
  useLibraryServiceListMediaItems: (...a: unknown[]) => listMediaItems(...a),
  usePlaybackServiceGetWatchProgress: () => ({ data: { positionMs: "0" } }),
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
});

describe("the title page", () => {
  it("plays the title itself for a film", () => {
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("common.play"));
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
    fireEvent.click(screen.getByText("common.play"));
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
    fireEvent.click(screen.getByText("fra - 2.0"));
    fireEvent.click(screen.getByText("fra"));
    fireEvent.click(screen.getByText("common.play"));
    expect(navigateMock).toHaveBeenCalledWith(expect.objectContaining({ name: "player", mediaId: "m-1", audio: 2, subtitle: 3 }));
  });

  // Measured in prod on 2026-09-23: the thumbs were emoji and painted as two
  // empty boxes on a browser without a colour-emoji font.
  it("draws the thumbs, and names them, without an emoji font", () => {
    const { container } = render(<MediaDetailScreen mediaId="m-1" />);
    expect(container.textContent).not.toMatch(/[\u{1F44D}\u{1F44E}]/u);
    expect(screen.getByLabelText("media.rating.like").querySelector("svg")).toBeTruthy();
    expect(screen.getByLabelText("media.rating.dislike").querySelector("svg")).toBeTruthy();
  });

  it("marks watched against the profile in front of the viewer", () => {
    render(<MediaDetailScreen mediaId="m-1" />);
    fireEvent.click(screen.getByText("media.detail.markWatched"));
    expect(markWatched).toHaveBeenCalledWith({ mediaItemId: "m-1", params: { profileId: "prf-1" } });
  });
});
