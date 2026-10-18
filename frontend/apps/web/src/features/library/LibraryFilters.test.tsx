import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { LibraryFiltersPanel, applyLocalFilters } from "./LibraryFilters";

vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick }: { children: React.ReactNode; onClick?: () => void }) => (
    <button onClick={onClick}>{children}</button>
  ),
}));

const emptyOptions = { resolutions: [], codecs: [], audioLangs: [], subLangs: [] };

const baseSearch = {};

describe("LibraryFiltersPanel", () => {
  it("year_from input calls onUpdate with yearFrom when changed", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const fromInput = screen.getByPlaceholderText("From");
    fireEvent.change(fromInput, { target: { value: "2010" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ yearFrom: 2010 })
    );
  });

  it("year_to input calls onUpdate with yearTo when changed", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const toInput = screen.getByPlaceholderText("To");
    fireEvent.change(toInput, { target: { value: "2022" } });

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({ yearTo: 2022 })
    );
  });

  it("min_rating: clicking a star calls onUpdate with minRating", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    const starButtons = screen.getAllByTitle(/Min \d+ star/i);
    fireEvent.click(starButtons[2]); // 3rd star = min 3

    expect(onUpdate).toHaveBeenCalledWith({ minRating: 3 });
  });

  it("clicking 'Clear all' calls onUpdate to clear all filters", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={{ yearFrom: 2010, minRating: 3, genres: ["Action"] }}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    fireEvent.click(screen.getByText("Clear all"));

    expect(onUpdate).toHaveBeenCalledWith(
      expect.objectContaining({
        yearFrom: undefined,
        yearTo: undefined,
        minRating: undefined,
        genres: undefined,
      })
    );
  });

  it("genre chip click calls onUpdate with selected genres", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={["Action", "Drame"]}
        availableOptions={emptyOptions}
        onUpdate={onUpdate}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Action" }));

    expect(onUpdate).toHaveBeenCalledWith({ genres: ["Action"] });
  });

  it("offers only the genres the library actually has, never a hardcoded list", () => {
    // A hardcoded English list used to be merged in here. Metadata providers
    // return genres in the server's language, so a French library showed the
    // same genres twice ("Adventure" beside "Aventure") and the English chips
    // matched nothing, because the filter compares the stored string exactly.
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={["Aventure", "Comédie"]}
        availableOptions={emptyOptions}
        onUpdate={vi.fn()}
      />
    );

    expect(screen.getByRole("button", { name: "Aventure" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Comédie" })).toBeInTheDocument();
    for (const english of ["Adventure", "Comedy", "Action", "Sci-Fi", "Western"]) {
      expect(screen.queryByRole("button", { name: english })).not.toBeInTheDocument();
    }
  });

  it("renders no genre section at all when the library reports no genres", () => {
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={emptyOptions}
        onUpdate={vi.fn()}
      />
    );
    expect(screen.queryByRole("button", { name: "Action" })).not.toBeInTheDocument();
  });

  it("resolution checkbox (span) click calls onUpdate with selected resolutions", () => {
    const onUpdate = vi.fn();
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={{ ...emptyOptions, resolutions: ["1080p", "4K"] }}
        onUpdate={onUpdate}
      />
    );

    // CheckboxGroup renders span[role="checkbox"] elements; get all and use first
    const checkboxes = screen.getAllByRole("checkbox");
    // First checkbox corresponds to "1080p"
    fireEvent.click(checkboxes[0]);

    expect(onUpdate).toHaveBeenCalledWith({ resolutions: ["1080p"] });
  });

  it("shows the localized language name, not the raw code, for audio/subtitle filter options", () => {
    render(
      <LibraryFiltersPanel
        search={baseSearch}
        availableGenres={[]}
        availableOptions={{ ...emptyOptions, audioLangs: ["fr"], subLangs: ["de"] }}
        onUpdate={vi.fn()}
      />
    );

    expect(screen.getByText("French")).toBeInTheDocument();
    expect(screen.getByText("German")).toBeInTheDocument();
    expect(screen.queryByText("fr")).not.toBeInTheDocument();
    expect(screen.queryByText("de")).not.toBeInTheDocument();
  });
});

function mediaItem(overrides: Partial<V1MediaItem> = {}): V1MediaItem {
  return { id: "mi-1", title: "Item", ...overrides };
}

describe("applyLocalFilters", () => {
  it("matches a normalized audio language filter against a raw 639-2/B stream tag", () => {
    const items: V1MediaItem[] = [
      mediaItem({
        id: "vff",
        streams: [{ type: "MEDIA_STREAM_TYPE_AUDIO", language: "fre" }],
      }),
      mediaItem({
        id: "eng",
        streams: [{ type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng" }],
      }),
    ];

    const result = applyLocalFilters(items, { audioLangs: ["fr"] });
    expect(result.map((i) => i.id)).toEqual(["vff"]);
  });

  it("matches a raw 639-2/T stream tag against the same filter as its 639-2/B sibling", () => {
    const items: V1MediaItem[] = [
      mediaItem({
        id: "fra-tagged",
        streams: [{ type: "MEDIA_STREAM_TYPE_AUDIO", language: "fra" }],
      }),
    ];

    const result = applyLocalFilters(items, { audioLangs: ["fre"] });
    expect(result.map((i) => i.id)).toEqual(["fra-tagged"]);
  });

  it("keeps items without stream info when stream facets are active (search stubs)", () => {
    // SearchMedia returns id/title stubs with no streams: no data must read
    // as "unknown, keep", not as a mismatch that empties the whole grid.
    const stub = mediaItem({ id: "search-stub" });
    const result = applyLocalFilters([stub], {
      resolutions: ["4K"],
      codecs: ["H.264"],
      audioLangs: ["fr"],
      subLangs: ["fr"],
    });
    expect(result.map((i) => i.id)).toEqual(["search-stub"]);
  });
});
