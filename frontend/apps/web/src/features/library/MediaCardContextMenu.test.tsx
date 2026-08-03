import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CardContextMenu } from "./MediaCardContextMenu";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockMarkWatchedMutate = vi.fn();
const mockMarkUnwatchedMutate = vi.fn();
const mockToggleFavMutate = vi.fn();
const mockNavigate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

// Capture onSuccess/onError from mutation option bags
let watchedOnSuccess: (() => void) | undefined;
let watchedOnError: (() => void) | undefined;
let unwatchedOnSuccess: (() => void) | undefined;
let unwatchedOnError: (() => void) | undefined;

let mockIsFav = false;
let mockProfileId: string | null = "prof-1";
let mockIsAdmin = true;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", () => ({
  usePlaybackServiceMarkAsWatched: (opts: {
    mutation?: { onSuccess?: () => void; onError?: () => void };
  }) => {
    watchedOnSuccess = opts?.mutation?.onSuccess;
    watchedOnError = opts?.mutation?.onError;
    return { mutate: mockMarkWatchedMutate };
  },
  usePlaybackServiceMarkAsUnwatched: (opts: {
    mutation?: { onSuccess?: () => void; onError?: () => void };
  }) => {
    unwatchedOnSuccess = opts?.mutation?.onSuccess;
    unwatchedOnError = opts?.mutation?.onError;
    return { mutate: mockMarkUnwatchedMutate };
  },
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (fn: (s: { profileId: string | null; isAdmin: boolean }) => unknown) =>
    fn({ profileId: mockProfileId, isAdmin: mockIsAdmin }),
}));

vi.mock("@/hooks/useFavorite", () => ({
  useIsFavorite: () => ({ data: mockIsFav }),
  useToggleFavorite: () => ({ mutate: mockToggleFavMutate }),
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

const DEFAULT_POSITION = { x: 100, y: 200 };

function renderMenu(props: {
  mediaId?: string;
  onClose?: () => void;
  onAddToPlaylist?: () => void;
  onAddToCollection?: () => void;
  position?: { x: number; y: number };
}) {
  const onClose = props.onClose ?? vi.fn();
  const onAddToPlaylist = props.onAddToPlaylist ?? vi.fn();
  const onAddToCollection = props.onAddToCollection ?? vi.fn();
  return render(
    <CardContextMenu
      mediaId={props.mediaId ?? "mi-movie-1"}
      onClose={onClose}
      position={props.position ?? DEFAULT_POSITION}
      onAddToPlaylist={onAddToPlaylist}
      onAddToCollection={onAddToCollection}
    />,
    { wrapper: createWrapper() },
  );
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("CardContextMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockIsFav = false;
    mockProfileId = "prof-1";
    mockIsAdmin = true;
    watchedOnSuccess = undefined;
    watchedOnError = undefined;
    unwatchedOnSuccess = undefined;
    unwatchedOnError = undefined;
  });

  // ── Rendering: all items present ─────────────────────────────────────────────

  it("renders the menu container with role=menu", () => {
    renderMenu({});
    expect(screen.getByRole("menu")).toBeInTheDocument();
  });

  it("renders the edit-metadata item", () => {
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-edit-metadata")).toBeInTheDocument();
  });

  it("renders the add-to-playlist item", () => {
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-add-to-playlist")).toBeInTheDocument();
  });

  it("renders the add-to-collection item", () => {
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-add-to-collection")).toBeInTheDocument();
  });

  it("hides the two admin-only items from a viewer, keeping the rest", () => {
    mockIsAdmin = false;
    renderMenu({});
    expect(screen.queryByTestId("media-context-menu-edit-metadata")).not.toBeInTheDocument();
    expect(screen.queryByTestId("media-context-menu-add-to-collection")).not.toBeInTheDocument();
    expect(screen.getByTestId("media-context-menu-add-to-playlist")).toBeInTheDocument();
    expect(screen.getByTestId("media-context-menu-mark-watched")).toBeInTheDocument();
    expect(screen.getByTestId("media-context-menu-add-to-favorites")).toBeInTheDocument();
  });

  it("renders the mark-watched item", () => {
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-mark-watched")).toBeInTheDocument();
  });

  it("renders the mark-unwatched item", () => {
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-mark-unwatched")).toBeInTheDocument();
  });

  // ── Favorite toggle: conditional label ───────────────────────────────────────

  it("shows 'Add to Favorites' label when item is not favorited", () => {
    mockIsFav = false;
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-add-to-favorites")).toBeInTheDocument();
    expect(screen.queryByTestId("media-context-menu-remove-from-favorites")).not.toBeInTheDocument();
  });

  it("shows 'Remove from Favorites' label when item is favorited", () => {
    mockIsFav = true;
    renderMenu({});
    expect(screen.getByTestId("media-context-menu-remove-from-favorites")).toBeInTheDocument();
    expect(screen.queryByTestId("media-context-menu-add-to-favorites")).not.toBeInTheDocument();
  });

  it("favorite item displays 'Add to Favorites' text when not favorited", () => {
    mockIsFav = false;
    renderMenu({});
    expect(screen.getByText("Add to Favorites")).toBeInTheDocument();
  });

  it("favorite item displays 'Remove from Favorites' text when favorited", () => {
    mockIsFav = true;
    renderMenu({});
    expect(screen.getByText("Remove from Favorites")).toBeInTheDocument();
  });

  // ── Favorite toggle: mutation payload ────────────────────────────────────────

  it("clicking favorite (not fav) calls toggleFav.mutate with false", () => {
    mockIsFav = false;
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-add-to-favorites"));
    expect(mockToggleFavMutate).toHaveBeenCalledOnce();
    expect(mockToggleFavMutate).toHaveBeenCalledWith(false);
  });

  it("clicking favorite (already fav) calls toggleFav.mutate with true", () => {
    mockIsFav = true;
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-remove-from-favorites"));
    expect(mockToggleFavMutate).toHaveBeenCalledOnce();
    expect(mockToggleFavMutate).toHaveBeenCalledWith(true);
  });

  it("clicking favorite calls onClose", () => {
    const onClose = vi.fn();
    mockIsFav = false;
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-add-to-favorites"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  // ── Mark watched mutation payload ─────────────────────────────────────────────

  it("clicking mark-watched calls markWatched.mutate with mediaItemId and profileId", () => {
    renderMenu({ mediaId: "mi-ep-1" });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    expect(mockMarkWatchedMutate).toHaveBeenCalledOnce();
    expect(mockMarkWatchedMutate).toHaveBeenCalledWith({
      mediaItemId: "mi-ep-1",
      params: { profileId: "prof-1" },
    });
  });

  it("clicking mark-unwatched calls markUnwatched.mutate with mediaItemId and profileId", () => {
    renderMenu({ mediaId: "mi-ep-2" });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-unwatched"));
    expect(mockMarkUnwatchedMutate).toHaveBeenCalledOnce();
    expect(mockMarkUnwatchedMutate).toHaveBeenCalledWith({
      mediaItemId: "mi-ep-2",
      params: { profileId: "prof-1" },
    });
  });

  // ── Mark watched: no profile guard ───────────────────────────────────────────

  it("shows error toast and calls onClose when mark-watched clicked without profileId", async () => {
    const { toast } = await import("@evey/store");
    const onClose = vi.fn();
    mockProfileId = null;
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    expect(mockMarkWatchedMutate).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("shows error toast and calls onClose when mark-unwatched clicked without profileId", async () => {
    const { toast } = await import("@evey/store");
    const onClose = vi.fn();
    mockProfileId = null;
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-unwatched"));
    expect(mockMarkUnwatchedMutate).not.toHaveBeenCalled();
    expect(toast.error).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalledOnce();
  });

  // ── Mark watched: onSuccess / onError callbacks ───────────────────────────────

  it("markWatched onSuccess invalidates item, watch history, and feeds", async () => {
    renderMenu({ mediaId: "mi-test" });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    watchedOnSuccess?.();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/media/mi-test"] }),
      ),
    );
    // Must mirror MediaCard.invalidateItem so the watched badge + "Continue
    // Watching" feeds update, not just the detail query.
    expect(mockInvalidateQueries).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: ["/api/v1/playback/history"] }),
    );
    const predicateCall = mockInvalidateQueries.mock.calls.find(
      ([arg]) => typeof arg?.predicate === "function",
    );
    expect(predicateCall).toBeTruthy();
    const predicate = predicateCall![0].predicate as (q: { queryKey: unknown[] }) => boolean;
    expect(predicate({ queryKey: ["/api/v1/feeds/continue-watching"] })).toBe(true);
    expect(predicate({ queryKey: ["/api/v1/popular"] })).toBe(true);
    expect(predicate({ queryKey: ["/api/v1/media/mi-test"] })).toBe(false);
  });

  it("markWatched onSuccess shows success toast", async () => {
    const { toast } = await import("@evey/store");
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    watchedOnSuccess?.();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("markWatched onSuccess calls onClose", async () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    watchedOnSuccess?.();
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("markWatched onError shows error toast", async () => {
    const { toast } = await import("@evey/store");
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    watchedOnError?.();
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });

  it("markWatched onError calls onClose", async () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-watched"));
    watchedOnError?.();
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  it("markUnwatched onSuccess invalidates the media item query", async () => {
    renderMenu({ mediaId: "mi-test" });
    fireEvent.click(screen.getByTestId("media-context-menu-mark-unwatched"));
    unwatchedOnSuccess?.();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/media/mi-test"] }),
      ),
    );
  });

  it("markUnwatched onSuccess shows success toast", async () => {
    const { toast } = await import("@evey/store");
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-mark-unwatched"));
    unwatchedOnSuccess?.();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("markUnwatched onError shows error toast", async () => {
    const { toast } = await import("@evey/store");
    renderMenu({});
    fireEvent.click(screen.getByTestId("media-context-menu-mark-unwatched"));
    unwatchedOnError?.();
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });

  // ── Edit metadata: navigate + close ──────────────────────────────────────────

  it("clicking edit-metadata calls navigate to /media/$mediaId with edit:true", () => {
    renderMenu({ mediaId: "mi-movie-1" });
    fireEvent.click(screen.getByTestId("media-context-menu-edit-metadata"));
    expect(mockNavigate).toHaveBeenCalledOnce();
    expect(mockNavigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: "/media/$mediaId",
        params: { mediaId: "mi-movie-1" },
      }),
    );
  });

  it("clicking edit-metadata calls onClose", () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.click(screen.getByTestId("media-context-menu-edit-metadata"));
    expect(onClose).toHaveBeenCalledOnce();
  });

  // ── Add to playlist + collection callbacks ────────────────────────────────────

  it("clicking add-to-playlist calls onClose then onAddToPlaylist", () => {
    const onClose = vi.fn();
    const onAddToPlaylist = vi.fn();
    renderMenu({ onClose, onAddToPlaylist });
    fireEvent.click(screen.getByTestId("media-context-menu-add-to-playlist"));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAddToPlaylist).toHaveBeenCalledOnce();
  });

  it("clicking add-to-collection calls onClose then onAddToCollection", () => {
    const onClose = vi.fn();
    const onAddToCollection = vi.fn();
    renderMenu({ onClose, onAddToCollection });
    fireEvent.click(screen.getByTestId("media-context-menu-add-to-collection"));
    expect(onClose).toHaveBeenCalledOnce();
    expect(onAddToCollection).toHaveBeenCalledOnce();
  });

  // ── Outside click closes menu ─────────────────────────────────────────────────

  it("mousedown outside the menu calls onClose", () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.mouseDown(document.body);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("mousedown inside the menu does NOT call onClose", () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.mouseDown(screen.getByRole("menu"));
    expect(onClose).not.toHaveBeenCalled();
  });

  // ── Keyboard: Escape closes menu ─────────────────────────────────────────────

  it("pressing Escape calls onClose", () => {
    const onClose = vi.fn();
    renderMenu({ onClose });
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    expect(onClose).toHaveBeenCalledOnce();
  });

  // ── Position is applied via inline style ──────────────────────────────────────

  it("renders the menu at the provided position", () => {
    renderMenu({ position: { x: 350, y: 450 } });
    const menu = screen.getByRole("menu");
    expect(menu).toHaveStyle({ left: "350px", top: "450px" });
  });
});
