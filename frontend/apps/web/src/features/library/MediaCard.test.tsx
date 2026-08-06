import React from "react";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaCard } from "./MediaCard";

vi.mock("@/hooks/useWatchedMap", () => ({
  useWatchedMap: () => new Map(),
  useIsWatched: () => false,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    params,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    params?: Record<string, string>;
    [k: string]: unknown;
  }) => {
    const href = params?.mediaId ? to.replace("$mediaId", params.mediaId) : to;
    return (
      <a
        href={href}
        data-testid={rest["data-testid"] as string}
        className={rest["className"] as string}
        onMouseEnter={rest["onMouseEnter"] as React.MouseEventHandler}
        onMouseLeave={rest["onMouseLeave"] as React.MouseEventHandler}
        onFocus={rest["onFocus"] as React.FocusEventHandler}
        onBlur={rest["onBlur"] as React.FocusEventHandler}
        onContextMenu={rest["onContextMenu"] as React.MouseEventHandler}
      >
        {children}
      </a>
    );
  },
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
  useFavoriteServiceIsFavorite: () => ({ data: false }),
  favoriteServiceAddFavorite: vi.fn(),
  favoriteServiceRemoveFavorite: vi.fn(),
  getFavoriteServiceIsFavoriteQueryKey: (id: string) => ["favorite", id],
  getFavoriteServiceListFavoritesQueryKey: () => ["favorites"],
  useRatingServiceGetStarRating: () => ({ data: 0 }),
  getRatingServiceGetStarRatingQueryKey: (id: string) => ["star-rating", id],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: Object.assign(
    (selector: (s: { accessToken: string | null; profileId: string | null }) => unknown) =>
      selector({ accessToken: "test-token", profileId: null }),
    { getState: () => ({ accessToken: "test-token", profileId: null }) },
  ),
}));

let queryClient: QueryClient;
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

const baseItem = {
  id: "42",
  title: "Some Movie",
  year: 2010,
  type: "MEDIA_ITEM_TYPE_MOVIE" as const,
};

const itemWithPreview = {
  ...baseItem,
  overview: "A mind-bending thriller",
  communityRating: 8.8,
  genres: ["Sci-Fi", "Action"],
};

describe("MediaCard", () => {
  beforeEach(() => {
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders title", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const titles = screen.getAllByText("Some Movie");
    expect(titles.length).toBeGreaterThanOrEqual(1);
  });

  it("renders poster image with correct src", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const img = screen.getByAltText("Some Movie");
    expect(img).toBeInTheDocument();
    expect(img).toHaveAttribute("src", "/api/v1/media/42/artwork/poster");
  });

  it("renders custom posterSrc", () => {
    render(<MediaCard item={baseItem} posterSrc="/custom.jpg" />, { wrapper: Wrapper });
    const img = screen.getByAltText("Some Movie");
    expect(img).toHaveAttribute("src", "/custom.jpg");
  });

  it("links to correct media page", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const link = screen.getByTestId("media-card");
    expect(link).toHaveAttribute("href", "/media/42");
  });

  it("shows year badge", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    expect(screen.getByText("2010")).toBeInTheDocument();
  });

  it("hides year when showYear=false", () => {
    render(<MediaCard item={baseItem} showYear={false} />, { wrapper: Wrapper });
    expect(screen.queryByText("2010")).not.toBeInTheDocument();
  });

  it("shows type label when showType=true", () => {
    render(<MediaCard item={baseItem} showType />, { wrapper: Wrapper });
    expect(screen.getByText("Film")).toBeInTheDocument();
  });

  it("shows progress bar when showProgress and progressPercent", () => {
    const { container } = render(
      <MediaCard item={baseItem} showProgress progressPercent={45} />,
      { wrapper: Wrapper },
    );
    const bar = container.querySelector("[style*='width: 45%']");
    expect(bar).toBeInTheDocument();
  });

  it("renders titled placeholder when no poster", () => {
    render(<MediaCard item={{ id: undefined, title: "Some Other Movie" }} />, { wrapper: Wrapper });
    // The posterless placeholder is the role="img" labelled by the title.
    expect(screen.getByRole("img", { name: "Some Other Movie" })).toBeInTheDocument();
  });

  // Badge tests
  it("shows watched badge when isWatched=true", () => {
    render(<MediaCard item={baseItem} isWatched />, { wrapper: Wrapper });
    expect(screen.getByTestId("watched-badge")).toBeInTheDocument();
  });

  it("shows watched badge when progressPercent >= 95", () => {
    render(<MediaCard item={baseItem} showProgress progressPercent={96} />, { wrapper: Wrapper });
    expect(screen.getByTestId("watched-badge")).toBeInTheDocument();
  });

  it("watched badge is not highlighted when progressPercent < 95", () => {
    render(<MediaCard item={baseItem} showProgress progressPercent={80} />, { wrapper: Wrapper });
    const badge = screen.getByTestId("watched-badge");
    expect(badge.className).not.toContain("bg-[var(--color-success)]");
  });

  it("shows resolution badge", () => {
    render(<MediaCard item={baseItem} resolution="1080p" />, { wrapper: Wrapper });
    expect(screen.getByTestId("resolution-badge")).toHaveTextContent("1080p");
  });

  it("formats 2160p as 4K", () => {
    render(<MediaCard item={baseItem} resolution="2160p" />, { wrapper: Wrapper });
    expect(screen.getByTestId("resolution-badge")).toHaveTextContent("4K");
  });

  it("shows HDR badge", () => {
    render(<MediaCard item={baseItem} isHdr />, { wrapper: Wrapper });
    expect(screen.getByTestId("hdr-badge")).toHaveTextContent("HDR");
  });

  it("shows both resolution and HDR badges together", () => {
    render(<MediaCard item={baseItem} resolution="2160p" isHdr />, { wrapper: Wrapper });
    expect(screen.getByTestId("resolution-badge")).toHaveTextContent("4K");
    expect(screen.getByTestId("hdr-badge")).toHaveTextContent("HDR");
  });

  it("does not show resolution/HDR badges when not provided", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    expect(screen.queryByTestId("resolution-badge")).not.toBeInTheDocument();
    expect(screen.queryByTestId("hdr-badge")).not.toBeInTheDocument();
  });

  // Keyboard accessibility
  it("shows preview on focus (keyboard navigation)", () => {
    render(<MediaCard item={itemWithPreview} />, { wrapper: Wrapper });
    const card = screen.getByTestId("media-card");

    expect(screen.queryByText("A mind-bending thriller")).not.toBeInTheDocument();

    act(() => { fireEvent.focus(card); });
    act(() => { vi.advanceTimersByTime(500); });

    expect(screen.getByText("A mind-bending thriller")).toBeInTheDocument();
  });

  it("hides preview on blur", () => {
    render(<MediaCard item={itemWithPreview} />, { wrapper: Wrapper });
    const card = screen.getByTestId("media-card");

    act(() => { fireEvent.focus(card); });
    act(() => { vi.advanceTimersByTime(500); });
    expect(screen.getByText("A mind-bending thriller")).toBeInTheDocument();

    act(() => { fireEvent.blur(card); });
    expect(screen.queryByText("A mind-bending thriller")).not.toBeInTheDocument();
  });

  it("shows preview on mouse hover", () => {
    render(<MediaCard item={itemWithPreview} />, { wrapper: Wrapper });
    const card = screen.getByTestId("media-card");

    act(() => { fireEvent.mouseEnter(card); });
    act(() => { vi.advanceTimersByTime(500); });

    expect(screen.getByText("A mind-bending thriller")).toBeInTheDocument();
  });

  it("does not show preview for items without preview data", () => {
    render(<MediaCard item={baseItem} />, { wrapper: Wrapper });
    const card = screen.getByTestId("media-card");

    act(() => { fireEvent.focus(card); });
    act(() => { vi.advanceTimersByTime(500); });

    // overview text shouldn't appear since baseItem has no overview/genres
    expect(screen.queryByText("A mind-bending thriller")).not.toBeInTheDocument();
  });
});
