import { describe, it, expect } from "vitest";
import type { V1MediaItem } from "@evey/api";
import {
  getResolutionLabel,
  normalizeCodec,
  collectFilterOptions,
  applyLocalFacetFilters,
  hasServerFilters,
  countActiveFilters,
  buildFilterMediaItemsParams,
  computeNextFilterOffset,
  buildBulkMediaIds,
  selectableMediaIds,
} from "./libraryFilterUtils";

function item(overrides: Partial<V1MediaItem> = {}): V1MediaItem {
  return { id: "mi-1", title: "Item", ...overrides };
}

describe("getResolutionLabel", () => {
  it.each([
    [2160, "4K"],
    [2200, "4K"],
    [1080, "1080p"],
    [1200, "1080p"],
    [720, "720p"],
    [900, "720p"],
    [480, "SD"],
    [0, "SD"],
  ])("maps height %d to %s", (height, expected) => {
    expect(getResolutionLabel(height)).toBe(expected);
  });
});

describe("normalizeCodec", () => {
  it.each([
    ["hevc", "H.265/HEVC"],
    ["H265", "H.265/HEVC"],
    ["h264", "H.264"],
    ["AVC", "H.264"],
    ["av1", "AV1"],
    ["vp9", "VP9"],
    ["vp8", "VP8"],
    ["xvid", "MPEG-4"],
    ["mpeg2video", "MPEG-2"],
    ["prores", "PRORES"],
  ])("normalizes %s to %s", (input, expected) => {
    expect(normalizeCodec(input)).toBe(expected);
  });
});

describe("collectFilterOptions", () => {
  it("collects distinct facets across items, skipping und language", () => {
    const items: V1MediaItem[] = [
      item({
        streams: [
          { type: "MEDIA_STREAM_TYPE_VIDEO", height: 2160, codec: "hevc" },
          { type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng" },
          { type: "MEDIA_STREAM_TYPE_AUDIO", language: "und" },
          { type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "fre" },
        ],
      }),
      item({
        id: "mi-2",
        streams: [
          { type: "MEDIA_STREAM_TYPE_VIDEO", height: 1080, codec: "h264" },
          { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fre" },
        ],
      }),
    ];

    expect(collectFilterOptions(items)).toEqual({
      resolutions: ["4K", "1080p"],
      codecs: ["H.264", "H.265/HEVC"],
      audioLangs: ["en", "fr"],
      subLangs: ["fr"],
    });
  });

  it("returns empty facets for items with no streams", () => {
    expect(collectFilterOptions([item()])).toEqual({
      resolutions: [],
      codecs: [],
      audioLangs: [],
      subLangs: [],
    });
  });

  it("normalizes 639-2/B and 639-2/T codes for the same language into a single filter entry", () => {
    const items: V1MediaItem[] = [
      item({
        streams: [
          { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fre" },
          { type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "ger" },
        ],
      }),
      item({
        id: "mi-2",
        streams: [
          { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fra" },
          { type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "deu" },
        ],
      }),
    ];

    const options = collectFilterOptions(items);
    expect(options.audioLangs).toEqual(["fr"]);
    expect(options.subLangs).toEqual(["de"]);
  });
});

describe("applyLocalFacetFilters", () => {
  const items: V1MediaItem[] = [
    item({
      id: "movie-4k-hevc-eng",
      genres: ["Action"],
      communityRating: 8,
      streams: [
        { type: "MEDIA_STREAM_TYPE_VIDEO", height: 2160, codec: "hevc" },
        { type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng" },
        { type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "fre" },
      ],
    }),
    item({
      id: "movie-1080p-h264-fre",
      genres: ["Drama"],
      communityRating: 5,
      streams: [
        { type: "MEDIA_STREAM_TYPE_VIDEO", height: 1080, codec: "h264" },
        { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fre" },
      ],
    }),
  ];

  it("returns all items unchanged when no filter is set", () => {
    expect(applyLocalFacetFilters(items, {})).toBe(items);
  });

  it("narrows by resolution", () => {
    const result = applyLocalFacetFilters(items, { resolutions: ["4K"] });
    expect(result.map((i) => i.id)).toEqual(["movie-4k-hevc-eng"]);
  });

  it("narrows by codec", () => {
    const result = applyLocalFacetFilters(items, { codecs: ["H.264"] });
    expect(result.map((i) => i.id)).toEqual(["movie-1080p-h264-fre"]);
  });

  it("narrows by audio language", () => {
    const result = applyLocalFacetFilters(items, { audioLangs: ["eng"] });
    expect(result.map((i) => i.id)).toEqual(["movie-4k-hevc-eng"]);
  });

  it("narrows by subtitle language", () => {
    const result = applyLocalFacetFilters(items, { subLangs: ["fre"] });
    expect(result.map((i) => i.id)).toEqual(["movie-4k-hevc-eng"]);
  });

  it("narrows by the search-only genres top-up", () => {
    const result = applyLocalFacetFilters(items, { genres: ["Drama"] });
    expect(result.map((i) => i.id)).toEqual(["movie-1080p-h264-fre"]);
  });

  it("narrows by the search-only minRating top-up", () => {
    const result = applyLocalFacetFilters(items, { minRating: 6 });
    expect(result.map((i) => i.id)).toEqual(["movie-4k-hevc-eng"]);
  });

  it("combines multiple facets with AND semantics", () => {
    const result = applyLocalFacetFilters(items, { resolutions: ["4K"], audioLangs: ["fre"] });
    expect(result).toEqual([]);
  });

  it("keeps items without stream info when stream facets are active (search stubs)", () => {
    // SearchMedia returns id/title stubs with no streams: no data must read
    // as "unknown, keep", not as a mismatch that empties the whole grid.
    const stub = item({ id: "search-stub" });
    const result = applyLocalFacetFilters([stub], {
      resolutions: ["4K"],
      codecs: ["H.264"],
      audioLangs: ["fr"],
      subLangs: ["fr"],
    });
    expect(result.map((i) => i.id)).toEqual(["search-stub"]);
  });

  it("still narrows by the non-stream top-ups on a streamless stub", () => {
    const stub = item({ id: "search-stub", genres: ["Action"], communityRating: 4 });
    expect(applyLocalFacetFilters([stub], { genres: ["Drama"] })).toEqual([]);
    expect(applyLocalFacetFilters([stub], { minRating: 6 })).toEqual([]);
  });

  it("matches a normalized audio language filter against a raw 639-2/B stream tag", () => {
    const result = applyLocalFacetFilters(items, { audioLangs: ["fr"] });
    expect(result.map((i) => i.id)).toEqual(["movie-1080p-h264-fre"]);
  });

  it("matches a raw 639-2/T filter code against the same language's 639-2/B stream tag", () => {
    const result = applyLocalFacetFilters(items, { audioLangs: ["fra"] });
    expect(result.map((i) => i.id)).toEqual(["movie-1080p-h264-fre"]);
  });
});

describe("hasServerFilters", () => {
  it("is false with no genres/year/rating", () => {
    expect(hasServerFilters({})).toBe(false);
  });

  it.each([
    [{ genres: ["Action"] }],
    [{ yearFrom: 2000 }],
    [{ yearTo: 2020 }],
    [{ minRating: 3 }],
  ])("is true when %o is set", (state) => {
    expect(hasServerFilters(state)).toBe(true);
  });

  it("ignores minRating of 0", () => {
    expect(hasServerFilters({ minRating: 0 })).toBe(false);
  });
});

describe("countActiveFilters", () => {
  it("counts zero for an empty state", () => {
    expect(countActiveFilters({})).toBe(0);
  });

  it("counts each active facet, including one per multi-select value", () => {
    expect(
      countActiveFilters({
        type: "MEDIA_ITEM_TYPE_MOVIE",
        genre: "Action",
        genres: ["Drama", "Comedy"],
        resolutions: ["4K"],
        codecs: ["H.264", "AV1"],
        yearFrom: 2000,
        yearTo: 2020,
        minRating: 4,
      }),
    ).toBe(1 + 1 + 2 + 1 + 2 + 1 + 1 + 1);
  });

  it("ignores a minRating of 0", () => {
    expect(countActiveFilters({ minRating: 0 })).toBe(0);
  });
});

describe("buildFilterMediaItemsParams", () => {
  const base = {
    libraryId: "lib-1",
    sortField: "MEDIA_SORT_FIELD_TITLE" as const,
    sortOrder: "SORT_ORDER_ASC" as const,
    limit: 30,
    offset: 0,
  };

  it("omits unset optional fields", () => {
    expect(buildFilterMediaItemsParams(base)).toEqual(base);
  });

  it("includes genres/year/rating when set", () => {
    expect(
      buildFilterMediaItemsParams({
        ...base,
        type: "MEDIA_ITEM_TYPE_MOVIE",
        genre: "Action",
        genres: ["Drama", "Comedy"],
        yearFrom: 2000,
        yearTo: 2020,
        minRating: 4,
        offset: 30,
      }),
    ).toEqual({
      ...base,
      type: "MEDIA_ITEM_TYPE_MOVIE",
      genre: "Action",
      genres: ["Drama", "Comedy"],
      yearFrom: 2000,
      yearTo: 2020,
      minRating: 4,
      offset: 30,
    });
  });

  it("drops a minRating of 0", () => {
    expect(buildFilterMediaItemsParams({ ...base, minRating: 0 })).toEqual(base);
  });

  it("drops an empty genres array", () => {
    expect(buildFilterMediaItemsParams({ ...base, genres: [] })).toEqual(base);
  });
});

describe("computeNextFilterOffset", () => {
  it("returns undefined when the last page came back short", () => {
    expect(computeNextFilterOffset(10, 5, 100, 10)).toBeUndefined();
  });

  it("returns undefined once the running total reaches the server total", () => {
    expect(computeNextFilterOffset(100, 10, 100, 10)).toBeUndefined();
  });

  it("returns the running total as the next offset otherwise", () => {
    expect(computeNextFilterOffset(30, 30, 100, 30)).toBe(30);
  });
});

// Regression test for the bulk-add-to-playlist bug: mobile used to call
// Array.from(selected)[0], adding only the FIRST selected item to a playlist/
// collection instead of the whole selection (web sends the full mediaIds
// array - see $libraryId.lazy.tsx:538-540).
describe("buildBulkMediaIds", () => {
  it("returns every selected id, not just the first", () => {
    const selected = new Set(["mi-1", "mi-2", "mi-3"]);
    expect(buildBulkMediaIds(selected)).toEqual(["mi-1", "mi-2", "mi-3"]);
  });

  it("returns an empty array for an empty selection", () => {
    expect(buildBulkMediaIds(new Set())).toEqual([]);
  });
});

describe("selectableMediaIds", () => {
  it("collects ids of the given items, skipping ones with no id", () => {
    const items: V1MediaItem[] = [
      item({ id: "mi-1" }),
      item({ id: undefined }),
      item({ id: "mi-2" }),
    ];
    expect(selectableMediaIds(items)).toEqual(["mi-1", "mi-2"]);
  });

  it("returns an empty array for an empty list", () => {
    expect(selectableMediaIds([])).toEqual([]);
  });
});

// Mirrors web's DEFAULT_TYPE_FILTER ($libraryId.lazy.tsx:78-81): a shows
// library defaults to series-only, a movies library to movies-only, so a
// mixed series/season/episode grid isn't the first thing the user sees.
