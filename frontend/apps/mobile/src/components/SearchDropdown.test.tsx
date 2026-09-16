import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "aria-selected": accessibilityState?.selected }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-image", () => ({ Image: () => React.createElement("img", { alt: "" }) }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => (opts && typeof opts === "object" ? `${key}(${Object.values(opts as object).join(",")})` : key),
  }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surfaceRaised: "#222", surface: "#111", border: "#333", shadow: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000" } }),
}));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("../lib/artwork", () => ({ artworkUrl: () => "http://evey.local/art" }));
vi.mock("../lib/coverArtUri", () => ({ coverArtUri: (_s: string, u?: string) => u ?? null }));
vi.mock("./LoadFailed", () => ({ LoadFailed: ({ onRetry, message }: any) => React.createElement("button", { onClick: onRetry }, message) }));

const search = vi.hoisted(() => ({
  media: { hook: vi.fn(), state: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() } },
  music: { hook: vi.fn(), state: { data: undefined as unknown, isLoading: false, isError: false, refetch: vi.fn() } },
}));
vi.mock("@evey/api", () => ({
  useLibraryServiceSearchMedia: (...a: unknown[]) => {
    search.media.hook(...a);
    return search.media.state;
  },
  useMusicServiceSearchMusic: (...a: unknown[]) => {
    search.music.hook(...a);
    return search.music.state;
  },
  displayTitle: (item: { title?: string }) => item.title ?? "",
}));

import { SearchDropdown } from "./SearchDropdown";

function mount(props: Partial<React.ComponentProps<typeof SearchDropdown>> = {}) {
  const handlers = {
    onKindChange: vi.fn(), onPickRecent: vi.fn(), onRemoveRecent: vi.fn(), onClearRecents: vi.fn(), onSelectMedia: vi.fn(), onSelectTrack: vi.fn(), onSeeAll: vi.fn(),
  };
  render(<SearchDropdown query="  matrix " visible kind="media" recents={[]} {...handlers} {...props} />);
  return handlers;
}

beforeEach(() => {
  search.media.hook.mockReset();
  search.music.hook.mockReset();
  search.media.state = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
  search.music.state = { data: undefined, isLoading: false, isError: false, refetch: vi.fn() };
});

// The dropdown queries only the active scope with the trimmed text, shows
// recents instead when the box is empty, offers "see all" only when the
// server holds more than the six shown, and reports a failed search as a
// failure rather than as "no results".
describe("SearchDropdown", () => {
  it("queries the active scope only, trimmed, and lists media hits with a see-all row", () => {
    search.media.state.data = { items: [{ id: "m1", title: "One", type: "MEDIA_ITEM_TYPE_MOVIE", year: 1999 }], totalHits: 9 };
    const h = mount();
    expect(search.media.hook).toHaveBeenLastCalledWith({ query: "matrix", limit: 6 }, { query: { enabled: true } });
    expect(search.music.hook).toHaveBeenLastCalledWith({ query: "matrix", limit: 6 }, { query: { enabled: false } });
    expect(screen.getByLabelText("One")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("One"));
    expect(h.onSelectMedia).toHaveBeenCalledWith(expect.objectContaining({ id: "m1" }));
    fireEvent.click(screen.getByText("media.search.viewAllResults(9)"));
    expect(h.onSeeAll).toHaveBeenCalled();
    fireEvent.click(screen.getByText("media.search.sectionMusic"));
    expect(h.onKindChange).toHaveBeenCalledWith("music");
  });

  it("no see-all row when every hit is shown; a failed search offers a retry, not 'no results'", () => {
    search.media.state.data = { items: [{ id: "m1", title: "One" }], totalHits: 1 };
    mount();
    expect(screen.queryByText(/viewAllResults/)).toBeNull();

    search.media.state = { data: undefined, isLoading: false, isError: true, refetch: vi.fn() };
    mount();
    expect(screen.queryByText("media.search.noResults")).toBeNull();
    fireEvent.click(screen.getByText("media.search.searchFailed"));
    expect(search.media.state.refetch).toHaveBeenCalled();
  });

  it("music scope lists tracks and plays one", () => {
    search.music.state.data = { tracks: [{ id: "t1", title: "Song", artist: "Band", album: "LP" }] };
    const h = mount({ kind: "music" });
    expect(search.music.hook).toHaveBeenLastCalledWith({ query: "matrix", limit: 6 }, { query: { enabled: true } });
    expect(screen.getByText("Band · LP")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Song"));
    expect(h.onSelectTrack).toHaveBeenCalledWith(expect.objectContaining({ id: "t1" }));
  });

  it("an empty box shows the recents; a one-letter query shows nothing", () => {
    const h = mount({ query: "", recents: ["alien", "blade"] });
    expect(search.media.hook).toHaveBeenLastCalledWith({ query: "", limit: 6 }, { query: { enabled: false } });
    fireEvent.click(screen.getByLabelText("blade"));
    expect(h.onPickRecent).toHaveBeenCalledWith("blade");
    fireEvent.click(screen.getByLabelText("common.removeItem(alien)"));
    expect(h.onRemoveRecent).toHaveBeenCalledWith("alien");
    fireEvent.click(screen.getByLabelText("media.search.clearAll"));
    expect(h.onClearRecents).toHaveBeenCalled();

    const { container } = render(
      <SearchDropdown query="a" visible kind="media" recents={["alien"]} onKindChange={vi.fn()} onPickRecent={vi.fn()} onRemoveRecent={vi.fn()} onClearRecents={vi.fn()} onSelectMedia={vi.fn()} onSelectTrack={vi.fn()} onSeeAll={vi.fn()} />,
    );
    expect(container.innerHTML).toBe("");
  });
});
