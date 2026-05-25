import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
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

  it("links the episode row to its media page", () => {
    mockListMediaItems.mockReturnValue({
      data: {
        items: [{ id: "ep1", indexNumber: 1, title: "Pilot" }],
      },
    });
    render(
      <SeasonEpisodesSection item={{ id: "se1", type: "MEDIA_ITEM_TYPE_SEASON" } as any} />,
      { wrapper: Wrapper },
    );
    expect(screen.getByText("Pilot").closest("a")).toHaveAttribute("href", "/media/ep1");
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
    const links = screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("/media/"));
    expect(links[0]).toHaveTextContent("First");
    expect(links[1]).toHaveTextContent("Second");
    expect(links[2]).toHaveTextContent("Third");
  });
});
