import { buildWatchProgressMaps } from "@evey/client";
import { describe, it, expect } from "vitest";
import React from "react";
import { vi } from "vitest";

// Pure-function tests only (no rendering) - see [libraryId].test.tsx for the
// existing render-based genre/shuffle coverage. These mocks exist solely to
// let the module import succeed outside a real React Native runtime; none of
// them are exercised since we never call the default-exported component.
vi.mock("react-native", () => ({
  View: (p: any) => React.createElement("div", p),
  Text: (p: any) => React.createElement("span", p),
  ScrollView: (p: any) => React.createElement("div", p),
  Pressable: (p: any) => React.createElement("button", p),
  TextInput: (p: any) => React.createElement("input", p),
  FlatList: () => null,
  ActivityIndicator: () => null,
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, absoluteFill: {} },
}));
vi.mock("expo-image", () => ({ Image: () => null }));
vi.mock("@expo/vector-icons", () => ({ Ionicons: () => null }));
vi.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ libraryId: "lib-1" }),
  useRouter: () => ({ push: vi.fn() }),
  Stack: { Screen: () => null },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("../../../src/providers/ThemeProvider", () => ({ useTheme: () => ({ colors: {}, radii: {} }) }));
vi.mock("../../../src/hooks/useServerUrl", () => ({ useServerUrl: () => "http://srv.test" }));
vi.mock("../../../src/hooks/useAuthenticatedImage", () => ({ useAuthHeaders: () => ({}) }));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() } }));
vi.mock("@evey/api", () => ({
  useLibraryServiceGetLibrary: vi.fn(),
  useLibraryServiceListMediaItems: vi.fn(),
  useLibraryServiceListGenres: vi.fn(),
  useLibraryServiceSearchMedia: vi.fn(),
  usePlaybackServiceGetWatchHistory: vi.fn(),
  libraryServiceListMediaItems: vi.fn(),
}));

import {
  buildMediaListParams,
  buildLibrarySearchParams,
  videoStream,
  isHdrStream,
  resolutionLabel,
} from "./[libraryId]";

describe("buildMediaListParams", () => {
  it("sends no cursor on the first page (server starts from the top)", () => {
    expect(
      buildMediaListParams({
        libraryId: "lib-1",
        type: "",
        genre: "",
        sortField: "MEDIA_SORT_FIELD_TITLE",
        sortOrder: "SORT_ORDER_ASC",
        cursor: "",
        pageSize: 30,
      }),
    ).toEqual({
      libraryId: "lib-1",
      sortField: "MEDIA_SORT_FIELD_TITLE",
      sortOrder: "SORT_ORDER_ASC",
      "pagination.pageSize": 30,
    });
  });

  // The screen used to page by `pagination.page` and stop on `totalPages`,
  // which the server never populates - the TV froze on the first 30 items of
  // every library. Keyset paging is also constant-cost per page instead of
  // making the DB skip `offset` rows, which matters on the NAS target.
  it("carries the server cursor on later pages, never a page number", () => {
    const params = buildMediaListParams({
      libraryId: "lib-1",
      type: "MEDIA_ITEM_TYPE_SERIES",
      genre: "Action",
      sortField: "MEDIA_SORT_FIELD_YEAR",
      sortOrder: "SORT_ORDER_DESC",
      cursor: "eyJmIjoieWVhciIsInYiOiIxOTk5In0",
      pageSize: 30,
    });
    expect(params).toMatchObject({
      type: "MEDIA_ITEM_TYPE_SERIES",
      genre: "Action",
      sortOrder: "SORT_ORDER_DESC",
      "pagination.cursor": "eyJmIjoieWVhciIsInYiOiIxOTk5In0",
    });
    expect(params).not.toHaveProperty("pagination.page");
  });
});

describe("buildLibrarySearchParams", () => {
  it("omits empty type/genre and carries the query + sort through", () => {
    const params = buildLibrarySearchParams({
      libraryId: "lib-1",
      query: "matrix",
      type: "",
      genre: "",
      sortField: "MEDIA_SORT_FIELD_RATING",
      sortOrder: "SORT_ORDER_DESC",
      limit: 120,
    });
    expect(params).toEqual({
      libraryId: "lib-1",
      query: "matrix",
      sortField: "MEDIA_SORT_FIELD_RATING",
      sortOrder: "SORT_ORDER_DESC",
      limit: 120,
    });
  });

  it("includes type/genre when set", () => {
    const params = buildLibrarySearchParams({
      libraryId: "lib-1",
      query: "matrix",
      type: "MEDIA_ITEM_TYPE_EPISODE",
      genre: "Drama",
      sortField: "MEDIA_SORT_FIELD_TITLE",
      sortOrder: "SORT_ORDER_ASC",
      limit: 120,
    });
    expect(params).toMatchObject({ type: "MEDIA_ITEM_TYPE_EPISODE", genre: "Drama" });
  });
});

describe("videoStream", () => {
  it("finds the video stream among mixed stream types", () => {
    const streams = [
      { type: "MEDIA_STREAM_TYPE_AUDIO" as const },
      { type: "MEDIA_STREAM_TYPE_VIDEO" as const, height: 1080 },
    ];
    expect(videoStream(streams)?.height).toBe(1080);
  });

  it("returns undefined when there is no video stream", () => {
    expect(videoStream([{ type: "MEDIA_STREAM_TYPE_AUDIO" as const }])).toBeUndefined();
    expect(videoStream(undefined)).toBeUndefined();
  });
});

describe("isHdrStream", () => {
  it("detects PQ (smpte2084) transfer as HDR", () => {
    expect(isHdrStream({ colorTransfer: "smpte2084" })).toBe(true);
  });

  it("detects HLG (arib-std-b67) transfer as HDR", () => {
    expect(isHdrStream({ colorTransfer: "ARIB-STD-B67" })).toBe(true);
  });

  it("detects bt2020 10-bit as HDR even without an explicit transfer tag", () => {
    expect(isHdrStream({ colorSpace: "bt2020nc", bitDepth: 10 })).toBe(true);
  });

  it("does not flag bt2020 8-bit as HDR", () => {
    expect(isHdrStream({ colorSpace: "bt2020nc", bitDepth: 8 })).toBe(false);
  });

  it("is false for a plain SDR stream or when absent", () => {
    expect(isHdrStream({ colorSpace: "bt709", colorTransfer: "bt709" })).toBe(false);
    expect(isHdrStream(undefined)).toBe(false);
  });
});

describe("resolutionLabel", () => {
  it("maps common heights to their marketing label", () => {
    expect(resolutionLabel({ height: 2160 })).toBe("4K");
    expect(resolutionLabel({ height: 1440 })).toBe("1440p");
    expect(resolutionLabel({ height: 1080 })).toBe("1080p");
    expect(resolutionLabel({ height: 720 })).toBe("720p");
  });

  it("falls back to a raw <height>p label for anything else", () => {
    expect(resolutionLabel({ height: 480 })).toBe("480p");
  });

  it("returns null when height is missing or zero", () => {
    expect(resolutionLabel(undefined)).toBeNull();
    expect(resolutionLabel({ height: 0 })).toBeNull();
  });
});

describe("buildWatchProgressMaps", () => {
  it("only marks items watched when played is true, and tracks percent separately", () => {
    const { watchedMap, percentMap } = buildWatchProgressMaps([
      { mediaItemId: "a", played: true, percentComplete: 100 },
      { mediaItemId: "b", played: false, percentComplete: 42 },
      { mediaItemId: "c", played: true },
    ]);
    expect(watchedMap.get("a")).toBe(true);
    expect(watchedMap.has("b")).toBe(false);
    expect(watchedMap.get("c")).toBe(true);
    expect(percentMap.get("a")).toBe(100);
    expect(percentMap.get("b")).toBe(42);
    expect(percentMap.has("c")).toBe(false);
  });

  it("skips entries with no mediaItemId", () => {
    const { watchedMap, percentMap } = buildWatchProgressMaps([{ played: true, percentComplete: 50 }]);
    expect(watchedMap.size).toBe(0);
    expect(percentMap.size).toBe(0);
  });
});
