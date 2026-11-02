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

// --- Whole-row fit ----------------------------------------------------------
// A grid row is a poster with its title block under it. The grid's rows used
// to be laid against the screen with no regard for its height: the row under
// the focused one showed as the top of its posters, cut before their titles.
// This picks the card width and the list height so the list shows whole rows
// only, every poster with its title, and nothing of the row after them.

export interface LibraryGridFitInput {
  /** Height the list may take: its box on screen, the home shortcut's band excluded. */
  availableHeight: number;
  /** The card's width when the screen has room for it. */
  nominalCardWidth: number;
  posterRatio: number;
  /** Focus ring drawn around the card, on each side. */
  ring: number;
  /** Space between the poster and its title. */
  titleGap: number;
  /** Height reserved for the title, whatever its length (two lines). */
  titleBlock: number;
  /** Vertical gap between two rows. */
  rowGap: number;
  /** Room left above the focused row (its focus zoom grows into it). */
  topInset: number;
  /** Scale of a focused card: it grows past its row by half the excess. */
  focusScale: number;
  /** Smallest share of the nominal width a card may shrink to for one more row. */
  minScale: number;
}

export interface LibraryGridFit {
  cardWidth: number;
  /** Rows the list shows at once, all of them whole. */
  rows: number;
  /** A card's height: ring, poster, gap, title block, ring. */
  cardHeight: number;
  /** What a focused card in the last row hangs below its row. */
  zoomOverhang: number;
  /** The list's own height: the inset, the rows and the last row's zoom. */
  listHeight: number;
}

export function libraryGridFit(input: LibraryGridFitInput): LibraryGridFit {
  const { availableHeight, nominalCardWidth, posterRatio, ring, titleGap, titleBlock, rowGap, topInset, focusScale, minScale } = input;
  const chrome = 2 * ring + titleGap + titleBlock;
  const overhangShare = (focusScale - 1) / 2;
  const cardHeightFor = (width: number) => chrome + width * posterRatio;
  const overhangFor = (width: number) => Math.ceil(overhangShare * cardHeightFor(width));
  const spanFor = (rows: number, width: number) =>
    Math.ceil(topInset + rows * cardHeightFor(width) + (rows - 1) * rowGap + overhangFor(width));
  const fits = (rows: number, width: number) => spanFor(rows, width) <= availableHeight;
  // The widest card, no wider than the nominal one, that lets `rows` rows fit:
  // solved for, then walked down past the rounding.
  const widthFor = (rows: number) => {
    const maxCard = (availableHeight - topInset - (rows - 1) * rowGap) / (rows + overhangShare);
    let width = Math.min(nominalCardWidth, Math.floor((maxCard - chrome) / posterRatio));
    while (width > 1 && !fits(rows, width)) width -= 1;
    return Math.max(1, width);
  };

  let rows = 0;
  while (fits(rows + 1, nominalCardWidth)) rows += 1;
  let cardWidth = nominalCardWidth;
  if (rows === 0) {
    // Not even one row at full size: the card shrinks until one does.
    rows = 1;
    cardWidth = widthFor(1);
  } else {
    // The room under the last whole row is blank: one more row takes it if
    // the cards need to give up little of their size for it.
    const shrunk = widthFor(rows + 1);
    if (shrunk >= nominalCardWidth * minScale && fits(rows + 1, shrunk)) {
      rows += 1;
      cardWidth = shrunk;
    }
  }
  return {
    cardWidth,
    rows,
    cardHeight: cardHeightFor(cardWidth),
    zoomOverhang: overhangFor(cardWidth),
    listHeight: spanFor(rows, cardWidth),
  };
}

/** One line naming what the grid shows: genre, type, then the sort and its direction. */
export function librarySelectionSummary(parts: { genre?: string; type?: string; sort: string; ascending: boolean }): string {
  return [parts.genre, parts.type, `${parts.sort} ${parts.ascending ? "↑" : "↓"}`].filter(Boolean).join(" · ");
}
