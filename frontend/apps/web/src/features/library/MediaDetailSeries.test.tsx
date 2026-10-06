import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent } from "@testing-library/react";
import { SeriesSection, SeasonEpisodesSection, PlaySeriesButton } from "./MediaDetailSeries";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: { children: React.ReactNode; to: string; params?: Record<string, string>; [k: string]: unknown }) => {
    const href = params ? Object.entries(params).reduce((acc, [k, v]) => acc.replace(`$${k}`, v), to) : to;
    return <a href={href} {...rest}>{children}</a>;
  },
}));

const mockListMediaItems = vi.fn();
const mockGetMediaItem = vi.fn();

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useLibraryServiceListMediaItems: (...args: unknown[]) => mockListMediaItems(...args),
  useLibraryServiceGetMediaItem: (...args: unknown[]) => mockGetMediaItem(...args),
}));

const mockPlayTarget = vi.fn();
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useSeriesPlayTarget: (...args: unknown[]) => mockPlayTarget(...args),
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
  // The module also owns the shared icon-button box, which DownloadButton
  // reads at import time - a stub without it fails the whole suite on load.
  ACTION_ICON_BOX: "w-10.5 h-10.5 shrink-0 flex items-center justify-center",
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
    expect(screen.getByText("1h 32min")).toBeInTheDocument();
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

// Episodes have no server-side sort key (SortColumnMap covers title, added_at,
// year, premiere_date, community_rating - not the episode index), so the server
// answers in sort_title order and this component re-sorts by index itself. That
// only works while it holds the whole season.
//
// Sending no pagination landed on the handler's default of 50. Past 50
// episodes the page therefore held the alphabetically-first 50, and
// PlaySeasonButton - which sorts by index and takes [0] - started playback on
// whichever episode sorted first by TITLE rather than on episode 1.
describe("season episode queries ask for the whole season", () => {
  beforeEach(() => {
    mockListMediaItems.mockReturnValue({ data: { items: [] } });
    mockGetMediaItem.mockReturnValue({ data: { indexNumber: 1, title: "Season 1" } });
  });

  it("requests episodes with an explicit page size, not the server default", () => {
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );

    expect(mockListMediaItems).toHaveBeenCalled();
    const params = mockListMediaItems.mock.calls.at(-1)?.[0] as Record<string, unknown>;
    expect(params).toMatchObject({ parentId: "se1" });
    expect(
      params["pagination.pageSize"],
      "without an explicit page size the handler serves 50 rows in sort_title order",
    ).toBe(500);
  });
});

// Play on a series resumes where the profile is in it; the first episode is
// only the fallback while the server has not answered (or cannot).
describe("PlaySeriesButton", () => {
  beforeEach(() => {
    mockListMediaItems.mockImplementation((params: { parentId: string }) =>
      params.parentId === "show"
        ? { data: { items: [{ id: "season-1", indexNumber: 1 }] } }
        : { data: { items: [{ id: "s1e2", indexNumber: 2 }, { id: "s1e1", indexNumber: 1 }] } });
  });

  it("starts the server's episode and says Resume once the series is started", () => {
    mockPlayTarget.mockReturnValue({ episode: { id: "s2e3", title: "Third" }, started: true, isLoading: false });
    const onPlay = vi.fn();
    render(<PlaySeriesButton seriesId="show" onPlay={onPlay} />, { wrapper: Wrapper });
    const btn = screen.getByTestId("play-series-button");
    expect(btn.textContent).toContain("Resume");
    fireEvent.click(btn);
    expect(onPlay).toHaveBeenCalledWith("s2e3");
  });

  it("falls back to the first episode, labelled Play, without an answer", () => {
    mockPlayTarget.mockReturnValue({ episode: undefined, started: false, isLoading: false });
    const onPlay = vi.fn();
    render(<PlaySeriesButton seriesId="show" onPlay={onPlay} />, { wrapper: Wrapper });
    const btn = screen.getByTestId("play-series-button");
    expect(btn.textContent).toContain("Play");
    fireEvent.click(btn);
    expect(onPlay).toHaveBeenCalledWith("s1e1");
  });
});
