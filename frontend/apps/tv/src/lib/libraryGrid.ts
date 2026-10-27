import {
  getLibraryServiceGetLibraryQueryOptions,
  getLibraryServiceListGenresQueryOptions,
  getLibraryServiceListMediaItemsQueryOptions,
  type LibraryServiceListMediaItemsSortField,
  type LibraryServiceListMediaItemsSortOrder,
  type LibraryServiceListMediaItemsType,
} from "@evey/api";
import type { FetchQueryOptions } from "@tanstack/react-query";

// The library grid's paging and first page, shared by the grid and by the
// screens that fetch that first page ahead (the library list, on focus).

// Two rows of the grid in the first request, so the first page lands and
// paints at once; six rows for the pages after it, since two-row pages had
// the focus catching up with the loading.
export const LIBRARY_PAGE_SIZE = 10;
export const LIBRARY_NEXT_PAGE_SIZE = 30;

/** The list row a card sits on: a FlatList with columns indexes its rows. */
export function libraryGridRow(cardIndex: number, numColumns: number): number {
  return Math.floor(cardIndex / numColumns);
}
export const LIBRARY_DEFAULT_SORT_FIELD: LibraryServiceListMediaItemsSortField = "MEDIA_SORT_FIELD_TITLE";
export const LIBRARY_DEFAULT_SORT_ORDER: LibraryServiceListMediaItemsSortOrder = "SORT_ORDER_ASC";

export interface BuildMediaListParamsInput {
  libraryId: string;
  type?: LibraryServiceListMediaItemsType | "";
  genre?: string;
  sortField: LibraryServiceListMediaItemsSortField;
  sortOrder: LibraryServiceListMediaItemsSortOrder;
  cursor: string;
  pageSize: number;
}

// Single source of truth for the ListMediaItems query params, so the sort/
// type/genre wiring can be unit-tested without rendering the screen.
// Keyset-paginated (opaque server cursor), like web and mobile: the previous
// `pagination.page` + `totalPages` pairing could never advance, because the
// server never populates total_pages (always 0), so the TV froze on the first
// page of every library.
export function buildMediaListParams(input: BuildMediaListParamsInput) {
  return {
    libraryId: input.libraryId,
    ...(input.type ? { type: input.type } : {}),
    ...(input.genre ? { genre: input.genre } : {}),
    sortField: input.sortField,
    sortOrder: input.sortOrder,
    "pagination.pageSize": input.pageSize,
    ...(input.cursor ? { "pagination.cursor": input.cursor } : {}),
  };
}

/** What the grid asks for when it opens: the library, its genres, its first page. */
export function libraryFirstPageQueries(libraryId: string): FetchQueryOptions[] {
  return [
    getLibraryServiceGetLibraryQueryOptions(libraryId),
    getLibraryServiceListGenresQueryOptions(libraryId),
    getLibraryServiceListMediaItemsQueryOptions(
      buildMediaListParams({
        libraryId,
        sortField: LIBRARY_DEFAULT_SORT_FIELD,
        sortOrder: LIBRARY_DEFAULT_SORT_ORDER,
        cursor: "",
        pageSize: LIBRARY_PAGE_SIZE,
      }),
    ),
  ] as FetchQueryOptions[];
}
