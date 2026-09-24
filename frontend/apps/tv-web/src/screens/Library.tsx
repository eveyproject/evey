import { useEffect, useMemo, useRef, useState } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  IN_LIBRARY_SEARCH_LIMIT,
  dedicatedRouteFor,
  defaultTypeFilterFor,
  hasMoreFromCursor,
  libraryServiceListMediaItems,
  useLibraryServiceGetLibrary,
  useLibraryServiceListGenres,
  useLibraryServiceSearchMedia,
  type LibraryServiceListMediaItemsSortField,
  type LibraryServiceListMediaItemsSortOrder,
  type LibraryServiceListMediaItemsType,
  type LibraryServiceSearchMediaSortField,
  type LibraryServiceSearchMediaSortOrder,
  type LibraryServiceSearchMediaType,
  itemLabel,
  type V1MediaItem,
} from "@evey/api";
import { toast } from "@evey/store";
import {
  CARD_WIDTH_GRID,
  buildPlayAllQueue,
  canPlayAllForSelection,
  isHdrStream,
  pickRandomItem,
  resolutionLabel,
  useDebounce,
  useWatchProgressMaps,
  videoStream,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Chip, Pressable, Spinner, TextField, useCardHandlers } from "../components/ui";
import { GridCard } from "../components/cards";
import { Icon } from "../components/icons";
import { StackHeader } from "../components/StackHeader";
import { navigate, replace } from "../app/router";
import { posterSrc } from "../lib/media";

// The same page the Android TV asks for; the next one is fetched when the
// remote reaches the last rows of the grid, so a held-down key never waits.
export const PAGE_SIZE = 30;
// Cards from the end at which the next page is requested: two rows of a
// grid that fits about six posters across at 1080p.
export const PREFETCH_FROM_END = 12;

// How many cards stay in the document at once. Everything fetched stays in
// memory; only this many are rendered around where the viewer is.
//
// Measured in the LG simulator: a walk of twenty presses costs about one
// style recalculation per press on a three card library and nine on a three
// hundred card one, because the page grows with every page fetched - a real
// library of a few thousand titles would put tens of thousands of nodes in
// front of a television that has neither the memory nor the processor for
// them. Six pages is deep enough that the window slides rarely.
export const RENDER_WINDOW = 180;

/**
 * The slice of the list to render, in whole rows, keeping the card the
 * viewer is on comfortably inside it.
 */
export function renderWindow(total: number, focusedIndex: number, columns: number, size = RENDER_WINDOW): { start: number; end: number } {
  if (total <= size) return { start: 0, end: total };
  const cols = Math.max(1, columns);
  const half = Math.floor(size / 2);
  let start = focusedIndex - half;
  start = Math.max(0, Math.min(start, total - size));
  // Whole rows only, so the grid never shifts sideways under the focus.
  start -= start % cols;
  return { start, end: Math.min(total, start + size) };
}
const TV_WEB_DEDICATED_ROUTES = ["/music", "/photos"];

const SORT_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsSortField }> = [
  { labelKey: "media.library.titleSort", value: "MEDIA_SORT_FIELD_TITLE" },
  { labelKey: "media.library.year", value: "MEDIA_SORT_FIELD_YEAR" },
  { labelKey: "media.library.ratingSort", value: "MEDIA_SORT_FIELD_RATING" },
  { labelKey: "media.library.dateAdded", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const TYPE_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsType }> = [
  { labelKey: "media.library.series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { labelKey: "media.library.episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

/**
 * Whether the card the remote just landed on is close enough to the end of
 * what is loaded for the next page to be fetched now.
 */
export function shouldFetchNextPage(focusedIndex: number, loaded: number, hasNextPage: boolean, isFetching: boolean): boolean {
  if (!hasNextPage || isFetching) return false;
  return focusedIndex >= loaded - PREFETCH_FROM_END;
}

// A library, the Android TV's way: genre chips, the type chips of a shows
// library (series by default, not the series/season/episode mix), the sort
// field with its direction, a search field that turns the grid into one
// relevance-ranked page, and keyset pages that follow the remote down the
// grid. The state lives here, not on the url: back from a title returns to
// the same grid at the same place through the screen cache.
export function LibraryScreen({ libraryId }: { libraryId: string }) {
  const { t } = useTranslation();
  const { data: library } = useLibraryServiceGetLibrary(libraryId, { query: { enabled: !!libraryId } });
  const isShows = library?.type === "LIBRARY_TYPE_SHOWS";

  // A music library has its own screens; the generic grid over it is empty
  // (its tracks are not media items), so the route is swapped in place.
  const dedicated = dedicatedRouteFor(library?.type, TV_WEB_DEDICATED_ROUTES);
  useEffect(() => {
    if (dedicated === "/music") replace({ name: "music", libraryId });
    if (dedicated === "/photos") replace({ name: "photos", libraryId });
  }, [dedicated, libraryId]);

  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  const [typeSeeded, setTypeSeeded] = useState(false);
  useEffect(() => {
    if (!library || typeSeeded) return;
    setTypeFilter(defaultTypeFilterFor(library.type));
    setTypeSeeded(true);
  }, [library, typeSeeded]);
  const [genre, setGenre] = useState("");
  const [sortField, setSortField] = useState<LibraryServiceListMediaItemsSortField>("MEDIA_SORT_FIELD_TITLE");
  const [sortOrder, setSortOrder] = useState<LibraryServiceListMediaItemsSortOrder>("SORT_ORDER_ASC");
  const [search, setSearch] = useState("");
  const query = useDebounce(search.trim(), 300);
  const searchActive = query.length > 0;

  const { data: genresData } = useLibraryServiceListGenres(libraryId, { query: { enabled: !!libraryId } });
  const { watchedMap, percentMap } = useWatchProgressMaps();
  const genres = genresData?.genres ?? [];

  const base = useMemo(() => ({
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    ...(genre ? { genre } : {}),
    sortField,
    sortOrder,
  }), [libraryId, typeFilter, genre, sortField, sortOrder]);

  const pages = useInfiniteQuery({
    queryKey: ["tv-web-library", base],
    queryFn: ({ pageParam, signal }) => libraryServiceListMediaItems(
      { ...base, "pagination.pageSize": PAGE_SIZE, ...(pageParam ? { "pagination.cursor": pageParam } : {}) },
      signal,
    ),
    initialPageParam: "",
    getNextPageParam: (last) => (hasMoreFromCursor(last.pagination) ? last.pagination?.nextCursor : undefined),
    enabled: !!libraryId && typeSeeded && !searchActive && !dedicated,
    gcTime: 30_000,
  });

  // SearchMedia's enums share ListMediaItems's string values; the casts spare
  // a parallel set of literals for two structurally identical types.
  const searchQuery = useLibraryServiceSearchMedia(
    {
      libraryId,
      query,
      ...(typeFilter ? { type: typeFilter as unknown as LibraryServiceSearchMediaType } : {}),
      ...(genre ? { genre } : {}),
      sortField: sortField as unknown as LibraryServiceSearchMediaSortField,
      sortOrder: sortOrder as unknown as LibraryServiceSearchMediaSortOrder,
      limit: IN_LIBRARY_SEARCH_LIMIT,
    },
    { query: { enabled: searchActive && !!libraryId } },
  );

  const items = useMemo(() => {
    const list = searchActive ? (searchQuery.data?.items ?? []) : (pages.data?.pages ?? []).flatMap((p) => p.items ?? []);
    return list.filter((it): it is V1MediaItem & { id: string } => !!it.id);
  }, [searchActive, searchQuery.data, pages.data]);

  const isLoading = searchActive ? searchQuery.isLoading : pages.isLoading || !typeSeeded;
  const isError = searchActive ? searchQuery.isError : pages.isError;

  // Stable per card: a fresh arrow function every render would rebuild every
  // card in the grid each time a page lands.
  const [focusedIndex, setFocusedIndex] = useState(0);
  const gridRef = useRef<HTMLDivElement | null>(null);
  // The grid's own geometry, read from the stylesheet once per render rather
  // than from every card's box.
  const [columns, setColumns] = useState(6);
  const [rowHeight, setRowHeight] = useState(0);
  useEffect(() => {
    const grid = gridRef.current;
    if (!grid) return;
    const cols = getComputedStyle(grid).gridTemplateColumns.split(" ").filter(Boolean).length;
    const card = grid.firstElementChild as HTMLElement | null;
    if (cols > 0) setColumns(cols);
    if (card) setRowHeight(card.offsetHeight + parseFloat(getComputedStyle(grid).rowGap || "0"));
  }, [items.length]);
  const window = useMemo(() => renderWindow(items.length, focusedIndex, columns), [items.length, focusedIndex, columns]);
  const openMedia = useCardHandlers((mediaId: string) => navigate({ name: "media", mediaId }));
  const cardFocused = useCardHandlers((index: number) => {
    setFocusedIndex(index);
    if (searchActiveRef.current) return;
    const p = pagesRef.current;
    if (shouldFetchNextPage(index, p.loaded, p.hasNext, p.fetching)) void p.fetchNext();
  });
  const searchActiveRef = useRef(searchActive);
  searchActiveRef.current = searchActive;
  const pagesRef = useRef({ loaded: 0, hasNext: false, fetching: false, fetchNext: () => {} });
  pagesRef.current = {
    loaded: items.length,
    hasNext: !!pages.hasNextPage,
    fetching: pages.isFetchingNextPage,
    fetchNext: () => { void pages.fetchNextPage(); },
  };

  const narrowedByViewer = !!genre || searchActive || typeFilter !== defaultTypeFilterFor(library?.type);
  const toggleType = (v: LibraryServiceListMediaItemsType) => setTypeFilter(typeFilter === v ? "" : v);
  const toggleGenre = (g: string) => setGenre(genre === g ? "" : g);

  // Shuffle: among what is on screen when a search narrows it, else a random
  // page of one over the whole library (the facets still apply server side).
  const [shuffling, setShuffling] = useState(false);
  const shufflingRef = useRef(false);
  const shuffle = async () => {
    if (shufflingRef.current) return;
    shufflingRef.current = true;
    setShuffling(true);
    try {
      if (searchActive) {
        const pick = pickRandomItem(items);
        if (pick?.id) navigate({ name: "player", mediaId: pick.id });
        else toast.info(t("common.shuffleEmpty"));
        return;
      }
      const probeBase = { libraryId, ...(typeFilter ? { type: typeFilter } : {}), ...(genre ? { genre } : {}) };
      const probe = await libraryServiceListMediaItems({ ...probeBase, "pagination.page": 1, "pagination.pageSize": 1 });
      const count = probe.pagination?.totalItems ?? 0;
      if (count === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const page = Math.floor(Math.random() * count) + 1;
      const res = page === 1 ? probe : await libraryServiceListMediaItems({ ...probeBase, "pagination.page": page, "pagination.pageSize": 1 });
      const pick = res.items?.[0];
      if (pick?.id) navigate({ name: "player", mediaId: pick.id });
      else toast.error(t("common.shuffleFailed"));
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      shufflingRef.current = false;
      setShuffling(false);
    }
  };
  const canPlayAll = canPlayAllForSelection(isShows, typeFilter);
  const playAll = () => {
    const first = buildPlayAllQueue(items)[0];
    if (first) navigate({ name: "player", mediaId: first });
    else toast.info(t("common.playAllEmpty"));
  };

  return (
    <div className="deep-page">
      <StackHeader title={library?.name ?? t("media.library.title")} />
      {isError ? (
        <FocusRoot autoFocus className="center-fill">
          <p className="muted big">{t("common.loadError")}</p>
          <Pressable className="tv-card link-btn" autoFocus onPress={() => (searchActive ? searchQuery.refetch() : pages.refetch())}>{t("common.retry")}</Pressable>
        </FocusRoot>
      ) : (
        <FocusRoot autoFocus className="library-body">
          <FocusRoot className="library-search">
            <TextField value={search} onChange={setSearch} placeholder={t("media.library.filterByName")} />
          </FocusRoot>
          <FocusRoot className="toolbar">
            <div className="toolbar-chips">
              <Chip label={t("media.search.allGenres") || "All"} active={!genre} onPress={() => setGenre("")} />
              {genres.map((g) => <Chip key={g} label={g} active={genre === g} onPress={() => toggleGenre(g)} />)}
            </div>
            {canPlayAll ? (
              <Pressable className="tv-card tool-btn" onPress={playAll} label={t("common.playAll")}>
                <Icon name="play" size={24} /><span>{t("common.playAll")}</span>
              </Pressable>
            ) : null}
            <Pressable className="tv-card tool-btn" onPress={() => void shuffle()} label={t("common.shufflePlay")}>
              {shuffling ? <Spinner /> : <Icon name="shuffle" size={24} />}<span>{t("common.shufflePlay")}</span>
            </Pressable>
          </FocusRoot>
          <FocusRoot className="toolbar">
            <div className="toolbar-chips">
              {SORT_KEYS.map((sk) => <Chip key={sk.value} label={t(sk.labelKey)} active={sortField === sk.value} onPress={() => setSortField(sk.value)} />)}
            </div>
            <Pressable
              className="tv-card tool-btn"
              onPress={() => setSortOrder(sortOrder === "SORT_ORDER_ASC" ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}
              label={sortOrder === "SORT_ORDER_ASC" ? t("common.sortAscending") : t("common.sortDescending")}
            >
              <Icon name={sortOrder === "SORT_ORDER_ASC" ? "arrow-up" : "arrow-down"} size={22} />
            </Pressable>
          </FocusRoot>
          {isShows ? (
            <FocusRoot className="toolbar">
              <div className="toolbar-chips">
                <Chip label={t("media.search.allGenres") || "All"} active={!typeFilter} onPress={() => setTypeFilter("")} />
                {TYPE_KEYS.map((tk) => <Chip key={tk.value} label={t(tk.labelKey)} active={typeFilter === tk.value} onPress={() => toggleType(tk.value)} />)}
              </div>
            </FocusRoot>
          ) : null}

          {isLoading && items.length === 0 ? (
            <div className="center-fill pad-top"><Spinner /></div>
          ) : !isLoading && items.length === 0 ? (
            <div className="center-fill pad-top">
              <p className="muted big">{narrowedByViewer ? t("media.library.noItemsFiltered") : t("common.noItems")}</p>
            </div>
          ) : (
            <FocusRoot grid className="library-grid-wrap">
              {/* The rows above and below the window are kept as height, so the
                  page is as long as the library and the scroll never jumps. */}
              {window.start > 0 ? <div style={{ height: `${(window.start / columns) * rowHeight}px` }} /> : null}
              <div className="grid library-grid" ref={gridRef}>
                {items.slice(window.start, window.end).map((item, i) => {
                  const v = videoStream(item.streams);
                  return (
                    <GridCard
                      key={item.id}
                      focusKey={`library1:${item.id}`}
                      title={itemLabel(item, t)}
                      imageUrl={posterSrc(item.id, "poster", CARD_WIDTH_GRID * 2)}
                      rating={item.communityRating}
                      resolution={resolutionLabel(v)}
                      hdr={isHdrStream(v)}
                      watched={watchedMap.get(item.id) === true}
                      percent={percentMap.get(item.id)}
                      autoFocus={window.start + i === 0}
                      onFocus={cardFocused(item.id, window.start + i)}
                      onPress={openMedia(item.id, item.id)}
                    />
                  );
                })}
              </div>
              {items.length > window.end ? <div style={{ height: `${((items.length - window.end) / columns) * rowHeight}px` }} /> : null}
              {pages.isFetchingNextPage ? <Spinner /> : null}
            </FocusRoot>
          )}
        </FocusRoot>
      )}
    </div>
  );
}
