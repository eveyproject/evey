import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// The screen is under test, not the libraries around it: the queries answer
// from the fixtures below and every box is focusable.
const feedConfig = vi.fn();
const libraries = vi.fn();
const resume = vi.fn();
const nextUp = vi.fn();
const recommended = vi.fn();
const listMediaItems = vi.fn();
const collections = vi.fn();
const popular = vi.fn();

vi.mock("../tv/focus", () => ({
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
// The key, never the fallback: a test asserting on English prose would pass
// on a row whose label key is wrong.
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: () => ({ data: { plugins: [{ id: "plg-weather", name: "weather", frontend: [{ slot: "home.section" }] }] } }),
  useQueries: ({ queries }: { queries: unknown[] }) => queries.map(() => listMediaItems()),
}));
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  getBaseURL: () => "http://server",
  artworkUrl: (base: string, id: string) => `${base}/art/${id}`,
  useFeedServiceGetFeedConfig: () => feedConfig(),
  useFeedServiceRecommended: () => recommended(),
  useLibraryServiceListLibraries: () => libraries(),
  usePlaybackServiceGetNextUp: () => nextUp(),
  usePlaybackServiceGetResumeItems: () => resume(),
  getLibraryServiceListMediaItemsQueryOptions: (p: unknown) => ({ queryKey: ["media", p] }),
  getMusicServiceGetAlbumsQueryOptions: (p: unknown) => ({ queryKey: ["albums", p] }),
}));
const authState = { profileId: "prf-1", accessToken: "t" };
vi.mock("@evey/store", () => ({
  useAuthStore: Object.assign((sel: any) => sel(authState), { getState: () => authState }),
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useCollections: () => collections(),
  useMostPopular: () => popular(),
  useWatchProgressMaps: () => ({ watchedMap: new Map([["res-1", true]]), percentMap: new Map([["res-1", 42]]) }),
}));
vi.mock("../plugins/PluginSlot", () => ({
  PluginSlot: ({ pluginId }: { pluginId?: string }) => <div data-testid={`plugin-${pluginId ?? "unplaced"}`} />,
}));
vi.mock("../components/FeedCustomizePanel", () => ({ FeedCustomizePanel: () => <div data-testid="customize" /> }));

import { HomeScreen } from "./Home";

const LIBRARIES = { libraries: [{ id: "lib-films", name: "Films", type: "LIBRARY_TYPE_MOVIES" }] };

beforeEach(() => {
  feedConfig.mockReturnValue({ data: { configs: [] } });
  libraries.mockReturnValue({ data: LIBRARIES, isLoading: false });
  resume.mockReturnValue({ data: { items: [{ id: "res-1", title: "Resume me", type: "MEDIA_ITEM_TYPE_MOVIE" }] }, isLoading: false });
  nextUp.mockReturnValue({ data: { items: [] }, isLoading: false });
  recommended.mockReturnValue({ data: { items: [{ mediaItemId: "rec-1", title: "Recommended one" }] } });
  listMediaItems.mockReturnValue({ data: { items: [{ id: "lib-item", title: "From the library" }] } });
  collections.mockReturnValue({ data: { collections: [{ id: "col-1", name: "Saga" }] } });
  popular.mockReturnValue({ data: [{ mediaItemId: "pop-1", title: "Popular one" }] });
});

describe("the home follows the server's row plan", () => {
  it("renders the rows the profile's feed config asks for, in its order", () => {
    feedConfig.mockReturnValue({
      data: {
        configs: [
          { feedType: "most_popular", enabled: true, displayOrder: 0 },
          { feedType: "resume_playback", enabled: true, displayOrder: 1 },
          { feedType: "recommended", enabled: false, displayOrder: 2 },
        ],
      },
    });
    render(<HomeScreen />);
    const titles = screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
    expect(titles.slice(0, 2)).toEqual(["media.home.mostPopular", "common.feeds.labels.resume_playback"]);
    // Disabled in the config: the row must not appear at all.
    expect(titles).not.toContain("media.home.recommendedForYou");
  });

  it("places a plugin row where the config puts it, and a fresh install above", () => {
    feedConfig.mockReturnValue({
      data: { configs: [{ feedType: "plugin:plg-weather", enabled: true, displayOrder: 0 }] },
    });
    render(<HomeScreen />);
    expect(screen.getByTestId("plugin-plg-weather")).toBeTruthy();
    expect(screen.getByTestId("plugin-unplaced")).toBeTruthy();
  });

  it("carries the watched tick and the progress bar onto the cards", () => {
    render(<HomeScreen />);
    expect(document.querySelector(".watched-badge")).toBeTruthy();
    const bar = document.querySelector(".card-progress i") as HTMLElement;
    expect(bar.style.width).toBe("42%");
  });

  // One row failing must not blank the home, and must not pose as "nothing
  // here": it says so and offers to ask again.
  it("a row whose query failed says so and offers a retry", () => {
    const refetch = vi.fn();
    feedConfig.mockReturnValue({ data: { configs: [{ feedType: "most_popular", enabled: true, displayOrder: 0 }, { feedType: "resume_playback", enabled: true, displayOrder: 1 }] } });
    popular.mockReturnValue({ data: [], isError: true, refetch });
    render(<HomeScreen />);
    expect(screen.getByText("common.loadError")).toBeTruthy();
    fireEvent.click(screen.getByText("common.retry"));
    expect(refetch).toHaveBeenCalled();
    // The other row is untouched.
    expect(screen.getByText("Resume me")).toBeTruthy();
  });

  it("says nothing is here only once every row came back empty", () => {
    libraries.mockReturnValue({ data: { libraries: [] }, isLoading: false });
    resume.mockReturnValue({ data: { items: [] }, isLoading: false });
    recommended.mockReturnValue({ data: { items: [] } });
    collections.mockReturnValue({ data: { collections: [] } });
    popular.mockReturnValue({ data: [] });
    render(<HomeScreen />);
    expect(screen.getByText("media.home.emptyLibrary")).toBeTruthy();
  });
});
