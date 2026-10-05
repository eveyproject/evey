import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaDetailActions } from "./MediaDetailActions";

const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
  useNavigate: () => mockNavigate,
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn(), isPending: false }),
  usePlaybackServiceGetWatchProgress: (...args: unknown[]) => {
    mockWatchProgressArgs = args;
    return { data: mockWatchProgress };
  },
  useLibraryServiceListMediaItems: () => ({ data: undefined }),
  useLibraryServiceGetMediaItem: () => ({ data: undefined }),
}));

let mockWatchProgress: { positionMs?: string; played?: boolean } | undefined;
let mockWatchProgressArgs: unknown[] = [];
let mockIsAdmin = false;
vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (sel: (s: { accessToken: string; profileId: string | null; isAdmin: boolean }) => unknown) =>
    sel({ accessToken: "tok", profileId: "profile-1", isAdmin: mockIsAdmin }),
}));

vi.mock("@/hooks/useParentalGate", () => ({
  useParentalGate: () => ({ needsPin: () => false }),
  markUnlocked: vi.fn(),
}));

vi.mock("@/components/ParentalPINDialog", () => ({
  ParentalPINDialog: () => null,
}));

vi.mock("../playback/DownloadButton", () => ({
  DownloadButton: () => <button data-testid="download-btn">Download</button>,
}));

vi.mock("./FavoriteButton", () => ({
  FavoriteButton: () => <button data-testid="favorite-btn">Favorite</button>,
}));

vi.mock("./ShareLinkButton", () => ({
  ShareLinkButton: () => <button data-testid="share-link-btn">Share</button>,
}));

vi.mock("./RatingButtons", () => ({
  RatingButtons: () => <div data-testid="rating-buttons" />,
}));

vi.mock("./StarRating", () => ({
  StarRating: () => <div data-testid="star-rating" />,
}));

vi.mock("../playlist/AddToPlaylistModal", () => ({
  AddToPlaylistModal: () => null,
}));

vi.mock("../collection/AddToCollectionModal", () => ({
  AddToCollectionModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="add-to-collection-modal" /> : null,
}));

vi.mock("./MediaDetailTrailer", () => ({
  TrailerSection: () => null,
}));

vi.mock("./MediaDetailSeries", () => ({
  PlaySeriesButton: ({ onPlay }: { onPlay: (id: string) => void }) => (
    <button data-testid="play-series-btn" onClick={() => onPlay("ep-1")}>Play Series</button>
  ),
  PlaySeasonButton: ({ onPlay }: { onPlay: (id: string) => void }) => (
    <button data-testid="play-season-btn" onClick={() => onPlay("ep-1")}>Play Season</button>
  ),
}));

vi.mock("./MediaDetailEdit", () => ({
  MetadataEditPanel: () => <div data-testid="metadata-edit" />,
}));

vi.mock("@/components/ui/Button", () => ({
  // `loading` and `variant`/`size` are Button's own props, not DOM attributes.
  Button: ({ children, loading, variant: _variant, size: _size, ...props }: { children: React.ReactNode; [k: string]: unknown }) => (
    <button disabled={Boolean(loading)} {...props}>{children}</button>
  ),
}));

vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ open, children }: { open: boolean; children: React.ReactNode }) =>
    open ? <div data-testid="modal">{children}</div> : null,
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

const baseItem = {
  id: "m1",
  title: "Some Movie",
  overview: "A thief who steals secrets through dreams.",
  tagline: "Nothing is as it seems.",
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
  genres: ["Action"],
  studios: ["Some Studio"],
  filePath: "/movies/some-movie.mkv",
  people: [],
};

const directors = [{ id: "d1", name: "Director One", type: "PERSON_TYPE_DIRECTOR" as const }];
const writers = [{ id: "w1", name: "Writer One", type: "PERSON_TYPE_WRITER" as const }];

const defaultProps = {
  item: baseItem,
  isSeries: false,
  isSeason: false,
  directors,
  writers,
  editing: false,
  setEditing: vi.fn(),
  onMetadataUpdated: vi.fn(),
  streamToken: "tok-123",
};

describe("MediaDetailActions", () => {
  beforeEach(() => {
    mockIsAdmin = false;
    mockWatchProgress = undefined;
  });

  // 27-09: a Resume label served from cache outlives what the player (autoplay)
  // or another device did since; on the web a return within the 5-minute
  // staleTime, or a tab left open, showed it. Read on every mount and focus.
  it("reads the watch progress with no stale window, refetched on focus", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    const opts = mockWatchProgressArgs[2] as { query?: { staleTime?: number; refetchOnWindowFocus?: boolean } };
    expect(opts?.query?.staleTime).toBe(0);
    expect(opts?.query?.refetchOnWindowFocus).toBe(true);
  });

  it("renders tagline", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText("Nothing is as it seems.")).toBeInTheDocument();
  });

  it("renders overview", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("media-overview")).toHaveTextContent("A thief who steals secrets through dreams.");
  });

  it("renders directors", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText("Director One")).toBeInTheDocument();
  });

  it("renders studios", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByText(/Some Studio/)).toBeInTheDocument();
  });

  it("shows play button for movie", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-button")).toBeInTheDocument();
  });

  it("hides play button for series", () => {
    render(<MediaDetailActions {...defaultProps} isSeries />, { wrapper: Wrapper });
    expect(screen.queryByTestId("play-button")).not.toBeInTheDocument();
  });

  it("shows PlaySeasonButton for season", () => {
    render(<MediaDetailActions {...defaultProps} isSeason />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-season-btn")).toBeInTheDocument();
    expect(screen.queryByTestId("play-button")).not.toBeInTheDocument();
  });

  it("shows PlaySeriesButton for series", () => {
    render(<MediaDetailActions {...defaultProps} isSeries />, { wrapper: Wrapper });
    expect(screen.getByTestId("play-series-btn")).toBeInTheDocument();
  });

  it("shows mark-watched button", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("mark-watched-btn")).toBeInTheDocument();
  });

  it("hides mark-watched button for a series (server does not cascade to episodes)", () => {
    render(<MediaDetailActions {...defaultProps} isSeries />, { wrapper: Wrapper });
    expect(screen.queryByTestId("mark-watched-btn")).not.toBeInTheDocument();
  });

  it("hides mark-watched button for a season", () => {
    render(<MediaDetailActions {...defaultProps} isSeason />, { wrapper: Wrapper });
    expect(screen.queryByTestId("mark-watched-btn")).not.toBeInTheDocument();
  });

  it("shows download button when filePath present", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("download-btn")).toBeInTheDocument();
  });

  // A non-admin viewer never receives item.filePath (the server path is redacted
  // for non-admins), but does receive fileSize. The download button must show for
  // them - gating it on filePath (the old behaviour) hid it from every non-admin.
  it("shows download button for a non-admin (filePath redacted, fileSize present)", () => {
    const item = { ...baseItem, filePath: undefined, fileSize: "1073741824" };
    render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.getByTestId("download-btn")).toBeInTheDocument();
  });

  it("hides download button for a container (no file at all)", () => {
    // No filePath AND no real fileSize (proto sends a container's zero as "0").
    const item = { ...baseItem, filePath: undefined, fileSize: "0" };
    render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("download-btn")).not.toBeInTheDocument();
  });

  it("shows favorite button", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("favorite-btn")).toBeInTheDocument();
  });

  it("shows rating buttons", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.getByTestId("rating-buttons")).toBeInTheDocument();
  });

  it("opens the collection picker from the detail page for an admin", () => {
    mockIsAdmin = true;
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("add-to-collection-modal")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("add-to-collection-button"));
    expect(screen.getByTestId("add-to-collection-modal")).toBeInTheDocument();
  });

  // AddCollectionItem is RoleAdmin: a viewer offered the button would pick a
  // collection and watch every choice fail.
  it("gives a viewer no way to add to a collection", () => {
    mockIsAdmin = false;
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("add-to-collection-button")).not.toBeInTheDocument();
    expect(screen.queryByTestId("add-to-collection-modal")).not.toBeInTheDocument();
  });

  it("shows edit modal when editing=true and user is admin", () => {
    mockIsAdmin = true;
    render(<MediaDetailActions {...defaultProps} editing />, { wrapper: Wrapper });
    expect(screen.getByTestId("modal")).toBeInTheDocument();
    expect(screen.getByTestId("metadata-edit")).toBeInTheDocument();
  });

  it("hides edit modal when editing=false", () => {
    mockIsAdmin = true;
    render(<MediaDetailActions {...defaultProps} editing={false} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
  });

  // Defense in depth: editing=true alone must not be enough - a non-admin
  // (or an admin demoted mid-session, leaving a stale editing=true) must never
  // get the metadata editor mounted, matching the mobile detail screen.
  it("hides edit modal when editing=true but user is not admin", () => {
    mockIsAdmin = false;
    render(<MediaDetailActions {...defaultProps} editing />, { wrapper: Wrapper });
    expect(screen.queryByTestId("modal")).not.toBeInTheDocument();
  });

  it("hides tagline when missing", () => {
    const item = { ...baseItem, tagline: undefined };
    render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
    expect(screen.queryByText("Nothing is as it seems.")).not.toBeInTheDocument();
  });

  it("hides directors when empty", () => {
    render(<MediaDetailActions {...defaultProps} directors={[]} />, { wrapper: Wrapper });
    expect(screen.queryByText("Director One")).not.toBeInTheDocument();
    expect(screen.getByText("Writer One")).toBeInTheDocument();
  });

  it("navigates to player on play click", () => {
    render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
    fireEvent.click(screen.getByTestId("play-button"));
    expect(mockNavigate).toHaveBeenCalledWith({ to: "/player/$mediaId", params: { mediaId: "m1" } });
  });

  // Resume vs start over, offered as one segmented control rather than two
  // rival Play buttons.
  describe("reprise", () => {
    it("n'offre rien a recommencer quand il n'y a pas de progression", () => {
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      expect(screen.queryByTestId("play-from-start-button")).not.toBeInTheDocument();
    });

    it("annonce la position sur le bouton principal", () => {
      mockWatchProgress = { positionMs: "754000" };
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      expect(screen.getByTestId("play-button")).toHaveTextContent("12:34");
    });

    it("reprend par defaut, sans demander au lecteur de repartir de zero", () => {
      mockWatchProgress = { positionMs: "754000" };
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      fireEvent.click(screen.getByTestId("play-button"));
      expect(mockNavigate).toHaveBeenCalledWith({ to: "/player/$mediaId", params: { mediaId: "m1" } });
    });

    it("la moitie recommencer dit au lecteur de repartir de zero", () => {
      mockWatchProgress = { positionMs: "754000" };
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      fireEvent.click(screen.getByTestId("play-from-start-button"));
      expect(mockNavigate).toHaveBeenCalledWith({
        to: "/player/$mediaId",
        params: { mediaId: "m1" },
        search: { restart: true },
      });
    });

    // A finished item has nothing to resume: offering "Resume 1:58:00" on the
    // credits would be worse than useless.
    it("ne propose pas de reprise sur un media deja vu", () => {
      mockWatchProgress = { positionMs: "754000", played: true };
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      expect(screen.queryByTestId("play-from-start-button")).not.toBeInTheDocument();
    });
  });

  describe("segment editor entry point", () => {
    it("is hidden from non-admins", () => {
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      expect(screen.queryByTestId("edit-segments-button")).not.toBeInTheDocument();
    });

    it("navigates an admin to the segment editor for this item", () => {
      mockIsAdmin = true;
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      fireEvent.click(screen.getByTestId("edit-segments-button"));
      expect(mockNavigate).toHaveBeenCalledWith({ to: "/admin/media/$mediaId", params: { mediaId: "m1" } });
    });

    // Unlike the share link, the entry point is not restricted to playable
    // items: a series page is a legitimate way into the segment editor.
    it("is shown on a series, which has no playable file of its own", () => {
      mockIsAdmin = true;
      const item = { ...baseItem, type: "MEDIA_ITEM_TYPE_SERIES" as const, filePath: undefined };
      render(<MediaDetailActions {...defaultProps} item={item} />, { wrapper: Wrapper });
      expect(screen.getByTestId("edit-segments-button")).toBeInTheDocument();
    });
  });

  // The public link serves a season or a series as a ZIP: an admin shares a
  // whole show, not only one file.
  describe("share link", () => {
    const series = { ...baseItem, type: "MEDIA_ITEM_TYPE_SERIES" as const, filePath: undefined };
    const season = { ...baseItem, type: "MEDIA_ITEM_TYPE_SEASON" as const, filePath: undefined };

    it("is offered to an admin on a series, a season and a file", () => {
      mockIsAdmin = true;
      const { unmount } = render(<MediaDetailActions {...defaultProps} item={series} isSeries />, { wrapper: Wrapper });
      expect(screen.getByTestId("share-link-btn")).toBeInTheDocument();
      unmount();
      const second = render(<MediaDetailActions {...defaultProps} item={season} isSeason />, { wrapper: Wrapper });
      expect(screen.getByTestId("share-link-btn")).toBeInTheDocument();
      second.unmount();
      render(<MediaDetailActions {...defaultProps} />, { wrapper: Wrapper });
      expect(screen.getByTestId("share-link-btn")).toBeInTheDocument();
    });

    it("is never offered to a non-admin", () => {
      mockIsAdmin = false;
      render(<MediaDetailActions {...defaultProps} item={series} isSeries />, { wrapper: Wrapper });
      expect(screen.queryByTestId("share-link-btn")).not.toBeInTheDocument();
    });
  });
});
