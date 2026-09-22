import { useEffect, useMemo, useState } from "react";
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
  type V1MediaItem,
} from "@evey/api";
import { useDebounce } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Card, Spinner, TextField } from "../components/ui";
import { navigate, replace } from "../app/router";
import { posterSrc } from "../lib/media";

// The same page the Android TV asks for; the next one is fetched when the
// remote reaches the last rows of the grid, so a held-down key never waits.
export const PAGE_SIZE = 30;
// Cards from the end at which the next page is requested: two rows of a
// grid that fits about six posters across at 1080p.
export const PREFETCH_FROM_END = 12;
const TV_WEB_DEDICATED_ROUTES = ["/music"];

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

function itemTitle(item: V1MediaItem): string {
  if (item.type === "MEDIA_ITEM_TYPE_EPISODE" && item.seriesTitle) return `${item.seriesTitle} - ${item.title ?? ""}`;
  return item.title ?? "";
}

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
  const total = pages.data?.pages[0]?.pagination?.totalItems;

  const onCardFocus = (index: number) => {
    if (searchActive) return;
    if (shouldFetchNextPage(index, items.length, !!pages.hasNextPage, pages.isFetchingNextPage)) void pages.fetchNextPage();
  };

  const toggleType = (v: LibraryServiceListMediaItemsType) => setTypeFilter(typeFilter === v ? "" : v);
  const toggleGenre = (g: string) => setGenre(genre === g ? "" : g);

  return (
    <div className="screen">
      <FocusRoot autoFocus className="header">
        <h1 className="title">{library?.name ?? t("common.nav.libraries", "Library")}</h1>
        <div className="actions" style={{ margin: 0 }}>
          <TextField value={search} onChange={setSearch} placeholder={t("media.search.searchLibrary", "Search your media library")} />
        </div>
      </FocusRoot>

      {isShows ? (
        <FocusRoot className="chips">
          {TYPE_KEYS.map((tk) => (
            <Button key={tk.value} variant={typeFilter === tk.value ? "primary" : "default"} onPress={() => toggleType(tk.value)}>{t(tk.labelKey)}</Button>
          ))}
        </FocusRoot>
      ) : null}

      {genres.length > 0 ? (
        <FocusRoot className="chips">
          <Button variant={!genre ? "primary" : "default"} onPress={() => setGenre("")}>{t("media.search.allGenres", "All genres")}</Button>
          {genres.map((g) => (
            <Button key={g} variant={genre === g ? "primary" : "default"} onPress={() => toggleGenre(g)}>{g}</Button>
          ))}
        </FocusRoot>
      ) : null}

      <FocusRoot className="chips">
        {SORT_KEYS.map((s) => (
          <Button key={s.value} variant={sortField === s.value ? "primary" : "default"} onPress={() => setSortField(s.value)}>{t(s.labelKey)}</Button>
        ))}
        <Button onPress={() => setSortOrder(sortOrder === "SORT_ORDER_ASC" ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}>
          {sortOrder === "SORT_ORDER_ASC" ? `↑ ${t("media.library.ascending", "Ascending")}` : `↓ ${t("media.library.descending", "Descending")}`}
        </Button>
        {total != null && !searchActive ? <span className="muted chips-count">{t("media.library.resultCount", { count: total, defaultValue: `${total} results` })}</span> : null}
      </FocusRoot>

      {isLoading ? <Spinner /> : null}
      {isError ? (
        <div className="actions">
          <p className="error">{t("common.loadError", "Failed to load")}</p>
          <Button onPress={() => (searchActive ? searchQuery.refetch() : pages.refetch())}>{t("common.retry", "Retry")}</Button>
        </div>
      ) : null}
      {!isLoading && !isError && items.length === 0 ? (
        <p className="muted">{genre || typeFilter || searchActive ? t("media.library.noItemsFiltered", "Try adjusting your filters or search term.") : t("media.library.noItemsEmptyUser", "No content here yet.")}</p>
      ) : null}
      <FocusRoot>
        <div className="grid">
          {items.map((item, i) => (
            <Card
              key={item.id}
              title={itemTitle(item)}
              imageUrl={posterSrc(item.id, "poster", 400)}
              onFocus={() => onCardFocus(i)}
              onPress={() => navigate({ name: "media", mediaId: item.id })}
            />
          ))}
        </div>
        {pages.isFetchingNextPage ? <Spinner /> : null}
      </FocusRoot>
    </div>
  );
}
