import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SeriesSection, SeasonEpisodesSection } from "./MediaDetailSeries";
import { getSeriesTrackPrefs } from "@/hooks/usePlaybackPrefs";

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

vi.mock("@/components/ui/Badge", () => ({
  Badge: ({ children }: { children: React.ReactNode }) => <span data-testid="badge">{children}</span>,
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

  it("renders the gapless-playback toggle, off by default", () => {
    mockListMediaItems.mockReturnValue({
      data: { items: [{ id: "se1", indexNumber: 1, type: "MEDIA_ITEM_TYPE_SEASON" }] },
    });
    mockGetMediaItem.mockReturnValue({ data: { indexNumber: 1, title: "Season 1" } });
    render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    const toggle = screen.getByTestId("gapless-toggle");
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });

  it("persists the gapless preference per-series on click", () => {
    mockListMediaItems.mockReturnValue({
      data: { items: [{ id: "se1", indexNumber: 1, type: "MEDIA_ITEM_TYPE_SEASON" }] },
    });
    mockGetMediaItem.mockReturnValue({ data: { indexNumber: 1, title: "Season 1" } });
    render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    fireEvent.click(screen.getByTestId("gapless-toggle"));
    expect(screen.getByTestId("gapless-toggle")).toHaveAttribute("aria-checked", "true");
    expect(getSeriesTrackPrefs("s1")?.gapless).toBe(true);
  });

  it("returns null when no seasons", () => {
    mockListMediaItems.mockReturnValue({ data: { items: [] } });
    const { container } = render(
      <SeriesSection item={{ id: "s1", type: "MEDIA_ITEM_TYPE_SERIES" } as any} />,
      { wrapper: Wrapper },
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders season count badge", () => {
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
    const badges = screen.getAllByTestId("badge");
    expect(badges.length).toBeGreaterThanOrEqual(1);
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
