import type {
  V1MediaItem,
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";
import { normalizeLangCode } from "../../lib/langCodes";

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

// The ONE resolution vocabulary, used by the filter chips AND by the badge on
// each poster. They used to be two different functions: the card labelled a
// 1440p file "1440p" while the filter had no such band and quietly folded it
// into "1080p". A viewer could therefore read "1440p" on a poster in front of
// them and find no way to filter for it (emulator, 2026-09-05).
//
// Ordered from highest to lowest, which is the order the chips are offered in.
export const RESOLUTION_OPTIONS = ["4K", "1440p", "1080p", "720p", "SD"] as const;
export type ResolutionBand = (typeof RESOLUTION_OPTIONS)[number];

export function getResolutionLabel(height: number): ResolutionBand {
  if (height >= 2160) return "4K";
  if (height >= 1440) return "1440p";
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
        // Normalize so 639-2/B and 639-2/T forms of the same language
        // ("fre"/"fra") collapse into one filter entry instead of two -
        // mirrors web's collectFilterOptions (LibraryFilters.tsx).
        if (stream.language && stream.language !== "und") {
          const norm = normalizeLangCode(stream.language);
          if (norm) audioLangs.add(norm);
        }
      } else if (stream.type === "MEDIA_STREAM_TYPE_SUBTITLE") {
        if (stream.language && stream.language !== "und") {
          const norm = normalizeLangCode(stream.language);
          if (norm) subLangs.add(norm);
        }
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
    // SearchMedia returns id/title stubs carrying no streams at all: no data
    // must read as "unknown, keep", not as a mismatch that would empty the
    // grid the moment a stream facet is combined with a text search.
    const hasStreamInfo = streams.length > 0;
    if (hasRes && hasStreamInfo) {
      const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
      const itemRes = videoStreams.map((s) => (s.height ? getResolutionLabel(s.height) : "SD"));
      if (!resolutions!.some((r) => itemRes.includes(r as ResolutionBand))) return false;
    }
    if (hasCodec && hasStreamInfo) {
      const videoStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
      const itemCodecs = videoStreams.map((s) => (s.codec ? normalizeCodec(s.codec) : ""));
      if (!codecs!.some((c) => itemCodecs.includes(c))) return false;
    }
    if (hasAudio && hasStreamInfo) {
      const audioStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
      // Normalize both sides so a "fr" filter (picked from a normalized
      // option) still matches a stream tagged "fra" - the raw code stays
      // untouched on the stream itself, only this comparison is normalized.
      const itemAudioLangs = audioStreams.map((s) => normalizeLangCode(s.language ?? ""));
      const wantAudioLangs = audioLangs!.map((l) => normalizeLangCode(l));
      if (!wantAudioLangs.some((l) => itemAudioLangs.includes(l))) return false;
    }
    if (hasSub && hasStreamInfo) {
      const subStreams = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");
      const itemSubLangs = subStreams.map((s) => normalizeLangCode(s.language ?? ""));
      const wantSubLangs = subLangs!.map((l) => normalizeLangCode(l));
      if (!wantSubLangs.some((l) => itemSubLangs.includes(l))) return false;
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

// --- Bulk selection helpers ---

// Extracted so bulk actions (playlist/collection) always send the FULL
// selection - the previous inline `Array.from(selected)[0]` at the playlist
// call site silently added only the first selected item.
export function buildBulkMediaIds(selected: ReadonlySet<string>): string[] {
  return Array.from(selected);
}

// Mirrors web's `displayItems.map((i) => i.id ?? "").filter(Boolean)` used by
// the "select all" bulk-bar action ($libraryId.lazy.tsx:586).
export function selectableMediaIds(items: V1MediaItem[]): string[] {
  return items.map((i) => i.id ?? "").filter((id) => id.length > 0);
}

// Default type filter, fetch-until-filled policy and the dedicated-route table
// used to live here as copies of web's. They now live in @evey/api
// (libraryRules.ts) so the three clients share one definition; rulesGuard.test.ts
// fails the build if a local copy comes back.
