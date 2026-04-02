import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
import { MediaCardWithModals } from "./MediaCardWithModals";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, ...props }: Record<string, unknown>) => (
    <a {...props}>{children as React.ReactNode}</a>
  ),
  useNavigate: () => vi.fn(),
}));

vi.mock("@tanstack/react-query", () => ({
  useQueryClient: () => ({}),
}));

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
  libraryServiceGetMediaItem: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
  useAuthStore: (sel: (s: Record<string, unknown>) => unknown) =>
    sel({ accessToken: "tok" }),
}));

vi.mock("@/hooks/useFavorite", () => ({
  useIsFavorite: () => false,
  useToggleFavorite: () => vi.fn(),
}));

vi.mock("@/hooks/useWatchedMap", () => ({
  useWatchedMap: () => ({}),
}));

let capturedPlaylistCb: ((id: string) => void) | undefined;
let capturedCollectionCb: ((id: string) => void) | undefined;

vi.mock("./MediaCard", () => ({
  MediaCard: (props: {
    item: { title?: string };
    onRequestAddToPlaylist?: (id: string) => void;
    onRequestAddToCollection?: (id: string) => void;
  }) => {
    capturedPlaylistCb = props.onRequestAddToPlaylist;
    capturedCollectionCb = props.onRequestAddToCollection;
    return <div data-testid="media-card">{props.item.title}</div>;
  },
}));

vi.mock("../playlist/AddToPlaylistModal", () => ({
  AddToPlaylistModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="playlist-modal">playlist</div> : null,
}));

vi.mock("../collection/AddToCollectionModal", () => ({
  AddToCollectionModal: ({ open }: { open: boolean }) =>
    open ? <div data-testid="collection-modal">collection</div> : null,
}));

const item = { id: "m1", title: "Test Movie", artwork: [] };

describe("MediaCardWithModals", () => {
  it("renders the media card with title", () => {
    render(<MediaCardWithModals item={item} />);
    expect(screen.getByText("Test Movie")).toBeInTheDocument();
  });

  it("does not show modals initially", () => {
    render(<MediaCardWithModals item={item} />);
    expect(screen.queryByTestId("playlist-modal")).not.toBeInTheDocument();
    expect(screen.queryByTestId("collection-modal")).not.toBeInTheDocument();
  });

  it("opens playlist modal when onRequestAddToPlaylist fires", () => {
    render(<MediaCardWithModals item={item} />);
    expect(capturedPlaylistCb).toBeDefined();
    act(() => {
      capturedPlaylistCb!("m1");
    });
    expect(screen.getByTestId("playlist-modal")).toBeInTheDocument();
  });

  it("opens collection modal when onRequestAddToCollection fires", () => {
    render(<MediaCardWithModals item={item} />);
    expect(capturedCollectionCb).toBeDefined();
    act(() => {
      capturedCollectionCb!("m1");
    });
    expect(screen.getByTestId("collection-modal")).toBeInTheDocument();
  });
});
