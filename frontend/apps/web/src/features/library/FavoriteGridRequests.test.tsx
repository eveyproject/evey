import React from "react";
import { render, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MediaCard } from "./MediaCard";

// Deliberately NOT mocking the favourite hooks: this file measures how many
// HTTP calls a grid actually issues, so the real hook -> generated client ->
// eveyFetch -> fetch chain has to run end to end. Only the router, the auth
// store and the unrelated playback/rating endpoints are stubbed.
vi.mock("@/hooks/useWatchedMap", () => ({
  useWatchedMap: () => new Map(),
  useIsWatched: () => false,
}));

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to, params, ...rest }: {
    children: React.ReactNode;
    to: string;
    params?: Record<string, string>;
    [k: string]: unknown;
  }) => {
    const href = params?.mediaId ? to.replace("$mediaId", params.mediaId) : to;
    return <a href={href} data-testid={rest["data-testid"] as string}>{children}</a>;
  },
  useNavigate: () => vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  usePlaybackServiceMarkAsWatched: () => ({ mutate: vi.fn() }),
  usePlaybackServiceMarkAsUnwatched: () => ({ mutate: vi.fn() }),
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

const GRID_SIZE = 24;

let favoriteCalls: string[] = [];
let queryClient: QueryClient;

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    headers: new Headers({ "content-type": "application/json" }),
    text: () => Promise.resolve(JSON.stringify(body)),
    clone() { return this as unknown as Response; },
    json: () => Promise.resolve(body),
  } as unknown as Response;
}

function Wrapper({ children }: { children: React.ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

describe("favourite state on a media grid", () => {
  beforeEach(() => {
    favoriteCalls = [];
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : String(input);
      if (url.includes("/api/v1/favorites")) {
        favoriteCalls.push(url);
        return Promise.resolve(jsonResponse({ favoriteMediaItemIds: [] }));
      }
      return Promise.resolve(jsonResponse({}));
    }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    queryClient.clear();
  });

  // The regression this file exists for: every card mounted its own
  // FavoriteButton, each firing GET /api/v1/favorites/{id}. A 226-card screen
  // meant 226 requests, which on its own saturated the 300 req/min limiter and
  // got the page's real calls refused as collateral damage.
  it("issues a single favourite request for a whole grid, not one per card", async () => {
    const items = Array.from({ length: GRID_SIZE }, (_, i) => ({
      id: `m-${i}`,
      title: `Some Movie ${i}`,
      year: 2010,
      type: "MEDIA_ITEM_TYPE_MOVIE" as const,
    }));

    render(
      <div>
        {items.map((item) => <MediaCard key={item.id} item={item} />)}
      </div>,
      { wrapper: Wrapper },
    );

    await waitFor(() => expect(favoriteCalls.length).toBeGreaterThan(0));
    // Let any straggler query settle before counting.
    await new Promise((r) => setTimeout(r, 20));

    expect(favoriteCalls.length).toBe(1);
  });

  it("asks the batch endpoint for every card id at once", async () => {
    const items = Array.from({ length: 3 }, (_, i) => ({
      id: `m-${i}`,
      title: `Some Movie ${i}`,
      type: "MEDIA_ITEM_TYPE_MOVIE" as const,
    }));

    const bodies: string[] = [];
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = typeof input === "string" ? input : String(input);
      if (url.includes("/api/v1/favorites")) {
        favoriteCalls.push(url);
        bodies.push(String(init?.body ?? ""));
        return Promise.resolve(jsonResponse({ favoriteMediaItemIds: ["m-1"] }));
      }
      return Promise.resolve(jsonResponse({}));
    }));

    render(
      <div>{items.map((item) => <MediaCard key={item.id} item={item} />)}</div>,
      { wrapper: Wrapper },
    );

    await waitFor(() => expect(favoriteCalls.length).toBe(1));
    expect(bodies[0]).toContain("m-0");
    expect(bodies[0]).toContain("m-1");
    expect(bodies[0]).toContain("m-2");
  });
});
