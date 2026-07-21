import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SeriesSection, SeasonEpisodesSection } from "./MediaDetailSeries";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
}));

const mockListMediaItems = vi.fn();
const mockGetMediaItem = vi.fn();

vi.mock("@evey/api", () => ({
  useLibraryServiceListMediaItems: (...args: unknown[]) => mockListMediaItems(...args),
  useLibraryServiceGetMediaItem: (...args: unknown[]) => mockGetMediaItem(...args),
}));

const mockDifficulties = vi.fn(() => new Map());

vi.mock("@/hooks/usePlaybackDifficulties", () => ({
  usePlaybackDifficulties: (...args: unknown[]) => mockDifficulties(...args),
}));

vi.mock("@/components/ui/Badge", () => ({
  // Forward the rest of the props: the real Badge passes data-testid through,
  // and a stub that swallows it would hide whether a badge rendered at all.
  Badge: ({ children, ...rest }: { children: React.ReactNode; [k: string]: unknown }) => (
    <span data-testid="badge" {...rest}>{children}</span>
  ),
}));

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, ...props }: { children: React.ReactNode; [k: string]: unknown }) => (
    <button {...props}>{children}</button>
  ),
}));

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("SeriesSection", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("returns null when no seasons", () => {
    mockListMediaItems.mockReturnValue({ data: { items: [] } });
    const { container } = render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    expect(container.innerHTML).toBe("");
  });

  // The count is plain micro-caps text now, not a filled Badge: a coloured pill
  // gave a passive number the same visual weight as an action.
  it("renders the season count", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [
          { id: "se1", indexNumber: 1, type: "MEDIA_ITEM_TYPE_SEASON" },
          { id: "se2", indexNumber: 2, type: "MEDIA_ITEM_TYPE_SEASON" },
        ],
      },
    });
    mockGetMediaItem.mockReturnValue({ data: { indexNumber: 1, title: "Season 1" } });
    render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    // The i18n stub returns an empty string for interpolated keys, so assert on
    // the element that carries the count rather than on its text.
    const { container } = render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    expect(container.querySelector(".tabular-nums")).toBeTruthy();
  });
});

describe("SeasonEpisodesSection", () => {
  it("returns null when no episodes", () => {
    mockListMediaItems.mockReturnValue({ data: { items: [] } });
    const { container } = render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders episodes list", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [
          { id: "ep1", indexNumber: 1, title: "Pilot", durationMs: "2700000" },
          { id: "ep2", indexNumber: 2, title: "Second", durationMs: "3000000" },
        ],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("Pilot")).toBeInTheDocument();
    expect(screen.getByText("Second")).toBeInTheDocument();
  });

  it("renders episode numbers", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 3, title: "Third Episode" }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("E3")).toBeInTheDocument();
  });

  it("renders episode duration", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 1, title: "Pilot", durationMs: "5520000" }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("1h 32m")).toBeInTheDocument();
  });

  // The episode row deliberately no longer navigates to a per-episode detail
  // page: the synopsis and the actions now live on the row itself, so that page
  // held nothing an episode needs and sat between the viewer and playback.
  // Every link on the row goes straight to the player.
  it("does not send the episode row to a per-episode detail page", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 1, title: "Pilot" }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    const detailLinks = screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("/media/"));
    expect(detailLinks).toHaveLength(0);
    expect(screen.getByText("Pilot")).toBeInTheDocument();
  });

  it("shows the episode synopsis on the row", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 1, title: "Pilot", overview: "Carmy comes home." }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("Carmy comes home.")).toBeInTheDocument();
  });

  it("exposes a play link to the player for each episode", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 1, title: "Pilot" }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    const playerLinks = screen.getAllByRole("link").filter((l) => l.getAttribute("href") === "/player/ep1");
    expect(playerLinks.length).toBe(1);
  });

  it("sorts episodes by indexNumber", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [
          { id: "ep3", indexNumber: 3, title: "Third" },
          { id: "ep1", indexNumber: 1, title: "First" },
          { id: "ep2", indexNumber: 2, title: "Second" },
        ],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    // Ordering is now read off the player links, one per episode, since the
    // rows no longer carry a /media/ link.
    const links = screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("/player/"));
    expect(links.map((l) => l.getAttribute("href"))).toEqual(["/player/ep1", "/player/ep2", "/player/ep3"]);
  });
});

// The whole reason this uses a bulk hook: ListMediaItems carries no streams, so
// a per-row GetStreamInfo would have meant one round trip per episode. The
// section must ask ONCE, for every episode id at a time.
describe("SeasonEpisodesSection difficulty badges", () => {
  beforeEach(() => {
    mockDifficulties.mockClear();
    mockDifficulties.mockReturnValue(new Map());
  });

  it("asks for every episode in a single call", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [
          { id: "ep1", indexNumber: 1, title: "One" },
          { id: "ep2", indexNumber: 2, title: "Two" },
          { id: "ep3", indexNumber: 3, title: "Three" },
        ],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(mockDifficulties).toHaveBeenCalledTimes(1);
    expect(mockDifficulties).toHaveBeenCalledWith(["ep1", "ep2", "ep3"]);
  });

  it("renders a badge only for the episodes the server graded", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [
          { id: "ep1", indexNumber: 1, title: "One" },
          { id: "ep2", indexNumber: 2, title: "Two" },
        ],
      },
    });
    mockDifficulties.mockReturnValue(
      new Map([["ep1", { level: "heavy", difficulty: "hard", reasons: [] }]]),
    );
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getAllByTestId("playback-difficulty")).toHaveLength(1);
  });
});
