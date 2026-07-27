import type {
  V1MediaItem,
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";

// The generated LibraryServiceFilterMediaItemsParams/SortField/SortOrder/Type
// aliases exist in packages/api/src/generated/library.schemas.ts but aren't
// re-exported from the @evey/api package barrel (only a curated subset is -
// see packages/api/src/index.ts). Rather than touch that shared barrel file,
// this reuses the already-exported List* types, which are structurally
// identical (same proto enum, same string literal members) so they type-check
// fine against the FilterMediaItems hook's params without an explicit cast.

// Pure, side-effect-free helpers backing the mobile library filter panel.
// Mirrors frontend/apps/web/src/features/library/LibraryFilters.tsx so both
// clients agree on facet labels and on which fields are server- vs
// client-filtered - kept as plain functions (no React) so they're testable
// without mounting the screen or mocking react-query/react-native.

// --- Resolution / codec facet labels (mirrors web's getResolutionLabel/normalizeCodec) ---

export const RESOLUTION_OPTIONS = ["4K", "1080p", "720p", "SD"] as const;
export type ResolutionBand = (typeof RESOLUTION_OPTIONS)[number];

export function getResolutionLabel(height: number): ResolutionBand {
  if (height >= 2160) return "4K";
  if (height >= 1080) return "1080p";
  if (height >= 720) return "720p";
  return "SD";
}

export function normalizeCodec(codec: string): string {
  const c = codec.toLowerCase();
  if (c === "hevc" || c === "h265" || c === "h.265") return "H.265/HEVC";
  if (c === "h264" || c === "avc" || c === "h.264") return "H.264";
  if (c === "av1") return "AV1";
  if (c === "vp9") return "VP9";
  if (c === "vp8") return "VP8";
  if (c === "mpeg4" || c === "xvid" || c === "divx") return "MPEG-4";
  if (c === "mpeg2video") return "MPEG-2";
  return codec.toUpperCase();
}

export interface FilterFacets {
  resolutions: string[];
  codecs: string[];
  audioLangs: string[];
  subLangs: string[];
}

// Derives the available facet values from whatever items are currently
// loaded (same source the genre chips already use on this screen) rather
// than a dedicated sampling query - the generated API has no facet-listing
// endpoint for these fields (unlike ListGenres for genres).
export function collectFilterOptions(items: V1MediaItem[]): FilterFacets {
  const resolutions = new Set<string>();
  const codecs = new Set<string>();
  const audioLangs = new Set<string>();
  const subLangs = new Set<string>();

  for (const item of items) {
    for (const stream of item.streams ?? []) {
      if (stream.type === "MEDIA_STREAM_TYPE_VIDEO") {
        if (stream.height) resolutions.add(getResolutionLabel(stream.height));
        if (stream.codec) codecs.add(normalizeCodec(stream.codec));
      } else if (stream.type === "MEDIA_STREAM_TYPE_AUDIO") {
        if (stream.language && stream.language !== "und") audioLangs.add(stream.language);
      } else if (stream.type === "MEDIA_STREAM_TYPE_SUBTITLE") {
        if (stream.language && stream.language !== "und") subLangs.add(stream.language);
      }
    }
  }

  return {
    resolutions: RESOLUTION_OPTIONS.filter((r) => resolutions.has(r)),
    codecs: Array.from(codecs).sort(),
    audioLangs: Array.from(audioLangs).sort(),
    subLangs: Array.from(subLangs).sort(),
  };
}

export interface LocalFacetFilters {
  resolutions?: string[];
  codecs?: string[];
  audioLangs?: string[];
  subLangs?: string[];
  // Genre/rating have a server equivalent (FilterMediaItems) but SearchMedia
  // doesn't support them, so callers pass these only while a text search is
  // active - see hasServerFilters/countActiveFilters below for the split.
  genres?: string[];
  minRating?: number;
}

// Client-side narrowing for facets with no generated API param at all
// (resolution/codec/audio+subtitle language), plus the text-search top-up
// for genres/minRating (SearchMediaRequest has neither). Never applied to
// genre/type/year, which always have a server-side equivalent on every path.
export function applyLocalFacetFilters(items: V1MediaItem[], filters: LocalFacetFilters): V1MediaItem[] {
  const { resolutions, codecs, audioLangs, subLangs, genres, minRating } = filters;
  const hasRes = resolutions && resolutions.length > 0;
  const hasCodec = codecs && codecs.length > 0;
  const hasAudio = audioLangs && audioLangs.length > 0;
  const hasSub = subLangs && subLangs.length > 0;
  const hasGenres = genres && genres.length > 0;
  const hasMinRating = minRating != null && minRating > 0;
  if (!hasRes && !hasCodec && !hasAudio && !hasSub && !hasGenres && !hasMinRating) return items;

  return items.filter((item) => {
    const streams = item.streams ?? [];
    if (hasRes) {
      const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
      const itemRes = videoStreams.map((s) => (s.height ? getResolutionLabel(s.height) : "SD"));
      if (!resolutions!.some((r) => itemRes.includes(r as ResolutionBand))) return false;
    }
    if (hasCodec) {
      const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
      const itemCodecs = videoStreams.map((s) => (s.codec ? normalizeCodec(s.codec) : ""));
      if (!codecs!.some((c) => itemCodecs.includes(c))) return false;
    }
    if (hasAudio) {
      const audioStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
      const itemAudioLangs = audioStreams.map((s) => s.language ?? "");
      if (!audioLangs!.some((l) => itemAudioLangs.includes(l))) return false;
    }
    if (hasSub) {
      const subStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");
      const itemSubLangs = subStreams.map((s) => s.language ?? "");
      if (!subLangs!.some((l) => itemSubLangs.includes(l))) return false;
    }
    if (hasGenres) {
      const itemGenres = item.genres ?? [];
      if (!genres!.some((g) => itemGenres.includes(g))) return false;
    }
    if (hasMinRating && (item.communityRating == null || item.communityRating < minRating!)) return false;
    return true;
  });
}

export interface LibraryFilterCounts {
  type?: LibraryServiceListMediaItemsType | "";
  genre?: string;
  genres?: string[];
  resolutions?: string[];
  codecs?: string[];
  audioLangs?: string[];
  subLangs?: string[];
  yearFrom?: number;
  yearTo?: number;
  minRating?: number;
}

// Only fields with a real FilterMediaItems param (genres/yearFrom/yearTo/
// minRating) switch the screen from the regular paged list to the server
// filter endpoint - resolution/codec/audio+subtitle language have no such
// param (see LibraryServiceFilterMediaItemsParams) and stay client-side.
export function hasServerFilters(state: Pick<LibraryFilterCounts, "genres" | "yearFrom" | "yearTo" | "minRating">): boolean {
  return Boolean(
    state.genres?.length ||
    state.yearFrom != null ||
    state.yearTo != null ||
    (state.minRating != null && state.minRating > 0),
  );
}

export function countActiveFilters(state: LibraryFilterCounts): number {
  let count = 0;
  if (state.type) count++;
  if (state.genre) count++;
  if (state.genres?.length) count += state.genres.length;
  if (state.resolutions?.length) count += state.resolutions.length;
  if (state.codecs?.length) count += state.codecs.length;
  if (state.audioLangs?.length) count += state.audioLangs.length;
  if (state.subLangs?.length) count += state.subLangs.length;
  if (state.yearFrom != null) count++;
  if (state.yearTo != null) count++;
  if (state.minRating != null && state.minRating > 0) count++;
  return count;
}

export interface BuildFilterParamsInput {
  libraryId: string;
  type?: LibraryServiceListMediaItemsType | "";
  genre?: string;
  genres?: string[];
  yearFrom?: number;
  yearTo?: number;
  minRating?: number;
  sortField: LibraryServiceListMediaItemsSortField;
  sortOrder: LibraryServiceListMediaItemsSortOrder;
  limit: number;
  offset: number;
}

// Structural shape of LibraryServiceFilterMediaItemsParams (GET
// /api/v1/media/filter) - kept as a local, unnamed-import-free type so
// callers type-check against the real generated param type without needing
// it re-exported from the @evey/api barrel (see the import comment above).
export interface FilterMediaItemsQueryParams {
  libraryId?: string;
  type?: LibraryServiceListMediaItemsType;
  genre?: string;
  genres?: string[];
  yearFrom?: number;
  yearTo?: number;
  minRating?: number;
  sortField?: LibraryServiceListMediaItemsSortField;
  sortOrder?: LibraryServiceListMediaItemsSortOrder;
  limit?: number;
  offset?: number;
}

// Pure request-builder for LibraryService.FilterMediaItems (GET /api/v1/media/filter).
// Extracted so the "which fields get sent, and under what conditions" logic
// can be unit tested without mounting the screen.
export function buildFilterMediaItemsParams(input: BuildFilterParamsInput): FilterMediaItemsQueryParams {
  const { libraryId, type, genre, genres, yearFrom, yearTo, minRating, sortField, sortOrder, limit, offset } = input;
  return {
    libraryId,
    ...(type ? { type } : {}),
    ...(genre ? { genre } : {}),
    ...(genres?.length ? { genres } : {}),
    ...(yearFrom != null ? { yearFrom } : {}),
    ...(yearTo != null ? { yearTo } : {}),
    ...(minRating != null && minRating > 0 ? { minRating } : {}),
    sortField,
    sortOrder,
    limit,
    offset,
  };
}

// Offset-pagination stop condition for the filter endpoint (no cursor
// support, unlike ListMediaItems) - mirrors web's useInfiniteQuery
// getNextPageParam for the same endpoint: stop once the last page came back
// short or the running total reaches the server-reported total.
export function computeNextFilterOffset(
  fetchedSoFar: number,
  lastPageItemCount: number,
  total: number,
  limit: number,
): number | undefined {
  if (lastPageItemCount < limit || fetchedSoFar >= total) return undefined;
  return fetchedSoFar;
}
