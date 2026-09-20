import { useMemo, useState, useEffect, useRef, useCallback } from "react";
import { displayTitle } from "@evey/api";
import { View, Text, ScrollView, FlatList, StyleSheet, Animated, DeviceEventEmitter, useWindowDimensions } from "react-native";
import { Image } from "expo-image";
import { router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueries, useQuery } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import {
  usePlaybackServiceGetResumeItems,
  usePlaybackServiceGetNextUp,
  useLibraryServiceListMediaItems,
  useLibraryServiceListLibraries,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecommended,
  getLibraryServiceListMediaItemsQueryOptions,
  getMusicServiceGetAlbumsQueryOptions,
  getPhotoServiceListPhotoAlbumsQueryOptions,
  eveyFetch,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { mediaTypeLabelKey } from "../../../src/lib/mediaTypeLabel";
import { musicAlbumHref } from "../../../src/lib/musicRoutes";
import { progressPercent } from "../../../src/lib/progressPercent";
import { artworkUrl } from "../../../src/lib/artwork";
import { ScreenState } from "../../../src/components/ScreenState";
import { FeedCustomizePanel } from "../../../src/components/FeedCustomizePanel";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
import { CARD_WIDTH_FEATURED, POSTER_RATIO, GRID_GAP, homeWidgetBudget } from "../../../src/lib/tvLayout";
import { HOME_LIBRARY_ROW_PAGE_SIZE } from "../../../src/lib/homeLibraryRows";
import { useCollections } from "../../../src/api/collections";
import { useMostPopular } from "../../../src/hooks/usePopular";
import { buildHomeSections, resolveMaxItems, type HomeSection } from "../../../src/lib/homeSections";

// How long the home waits for its slower rows before showing what it has.
const HOME_REVEAL_CAP_MS = 2500;
// Side gutter of a row: the first poster's focus ring and scale need room at
// the left edge, not a card flush with the title (owner, 2026-09-20).
const HOME_ROW_GUTTER = 32;
const CARD_WIDTH = CARD_WIDTH_FEATURED;
const CARD_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const CARD_TOTAL_WIDTH = CARD_WIDTH + GRID_GAP; // card + separator

// Same reasoning as the media page: the TV renders at 1080p, so a backdrop
// backdrop is requested at screen width instead of the full original.

const tvRowGetItemLayout = (_: unknown, index: number) => ({
  length: CARD_TOTAL_WIDTH,
  offset: CARD_TOTAL_WIDTH * index,
  index,
});

function tvRowKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

function RowSeparator() {
  return <View style={{ width: 16 }} />;
}

// A row whose fetch failed used to return null - identical to a row with no
// items, so a library or a feed that is unreachable read as "empty" rather
// than "the server did not answer" (mobile's LoadFailed, ported here inline
// since a TV row lives inside a vertical scroll, not its own screen).
function RowFailure({ onRetry }: { onRetry?: () => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <View style={styles.rowFailure}>
      <Text style={[styles.rowFailureMessage, { color: theme.colors.textMuted }]}>
        {t("common.loadError")}
      </Text>
      {onRetry ? (
        <FocusableCard
          onPress={onRetry}
          accessibilityLabel={t("common.retry")}
          style={[styles.rowRetryBtn, { backgroundColor: theme.colors.surface }]}
        >
          <Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.retry")}</Text>
        </FocusableCard>
      ) : null}
    </View>
  );
}

// The home screen must hand the TV remote's initial focus to exactly one
// element: the first row that actually has items, in on-screen order, so the
// remote is never left with nowhere to go. It used to be the hero's Play
// button; the hero is gone (owner's call 2026-09-19 - five recently-added
// titles sat above the plugin sections, outside the customisation the rest
// of the home obeys), so the focus opens on real content.
export function selectHomeFocusTarget(rowHasItems: boolean[]): number | null {
  const idx = rowHasItems.findIndex(Boolean);
  return idx === -1 ? null : idx;
}

function MediaCard({
  item,
  serverUrl,
  authHeaders,
  isFirst,
  watchPercent,
  onFocus,
}: {
  item: V1MediaItem;
  serverUrl: string;
  authHeaders?: { Authorization: string };
  isFirst?: boolean;
  watchPercent?: number;
  onFocus?: () => void;
}) {
  const { t } = useTranslation();
  const theme = useTheme();
  const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", CARD_WIDTH * 2) : undefined;
  const blurhash = (item.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;
  // The recommended/popular projections carry no `year` (see recommendedItems
  // below), which left MediaCard's subtitle line blank on every card in those
  // two rows. Falling back to the localized type ("Movie"/"Series") gives the
  // subtitle something to say instead of nothing.
  const typeLabelKey = !item.year ? mediaTypeLabelKey(item.type) : null;

  return (
    <FocusableCard
      onPress={() => router.push(`/media/${item.id}`)}
      hasTVPreferredFocus={isFirst}
      onFocus={onFocus}
      accessibilityLabel={displayTitle(item, t)}
      style={styles.card}
    >
      <View style={[styles.poster, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm }]}>
        {posterUri ? (
          // The corners are the image's own, not a clipping wrapper's: a
          // rounded overflow-hidden View clips through a path, which the
          // television's GPU pays for on every visible card, every frame.
          <Image
            cachePolicy="memory-disk"
            source={{ uri: posterUri, headers: authHeaders }}
            style={{ width: CARD_WIDTH, height: CARD_HEIGHT, borderRadius: theme.radii.sm }}
            contentFit="cover"
            transition={200}
            recyclingKey={item.id}
            placeholder={blurhash ? { blurhash } : undefined}
            placeholderContentFit="cover"
          />
        ) : (
          <View style={[styles.posterPlaceholder, { backgroundColor: theme.colors.surface }]}>
            <Text style={{ color: theme.colors.textMuted }}>?</Text>
          </View>
        )}
        {item.communityRating != null && item.communityRating > 0 && (
          <View style={[styles.ratingBadge, { backgroundColor: theme.colors.warning }]}>
            <Ionicons name="star" size={12} color={theme.colors.onWarning} />
            <Text style={[styles.ratingBadgeText, { color: theme.colors.onWarning }]}>
              {item.communityRating.toFixed(1)}
            </Text>
          </View>
        )}
      </View>
      {watchPercent != null && watchPercent > 0 && (
        <View style={[styles.progressBar, { backgroundColor: theme.colors.border }]}>
          <View style={[styles.progressFill, { width: `${watchPercent}%`, backgroundColor: theme.colors.accent }]} />
        </View>
      )}
      <Text style={[styles.cardTitle, { color: theme.colors.text }]} numberOfLines={1}>
        {displayTitle(item, t)}
      </Text>
      {item.year ? (
        <Text style={[styles.cardYear, { color: theme.colors.textMuted }]}>{item.year}</Text>
      ) : typeLabelKey ? (
        <Text style={[styles.cardYear, { color: theme.colors.textMuted }]}>{t(typeLabelKey)}</Text>
      ) : null}
    </FocusableCard>
  );
}

function MediaRow({
  title,
  items,
  serverUrl,
  authHeaders,
  watchPercents,
  focusFirst,
  onCardFocus,
  isError,
  onRetry,
}: {
  title: string;
  items: V1MediaItem[];
  serverUrl: string;
  authHeaders?: { Authorization: string };
  watchPercents?: Record<string, number>;
  focusFirst?: boolean;
  /** Fired when any card of this row takes the focus. */
  onCardFocus?: () => void;
  /** The fetch behind this row failed - render the failure block instead of
   * silently disappearing (a failed shelf used to look identical to an
   * empty one). */
  isError?: boolean;
  onRetry?: () => void;
}) {
  const theme = useTheme();
  if (isError) {
    return (
      <View style={styles.rowContainer}>
        <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
        <RowFailure onRetry={onRetry} />
      </View>
    );
  }
  if (items.length === 0) return null;

  return (
    <View style={styles.rowContainer}>
      <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
      <FlatList
        horizontal
        data={items}
        keyExtractor={tvRowKeyExtractor}
        renderItem={({ item, index }) => (
          <MediaCard
            item={item}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            isFirst={index === 0 && !!focusFirst}
            onFocus={onCardFocus}
            watchPercent={item.id ? watchPercents?.[item.id] : undefined}
          />
        )}
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={{ paddingHorizontal: HOME_ROW_GUTTER }}
        ItemSeparatorComponent={RowSeparator}
        getItemLayout={tvRowGetItemLayout}
        initialNumToRender={5}
        maxToRenderPerBatch={8}
        windowSize={5}
      />
    </View>
  );
}

// A generic horizontal row of square-art cards (collections, music albums,
// photo albums). Deliberately separate from MediaRow: those destinations are
// not /media/<id>, and a card that lands the viewer on a screen built for
// something else is the exact mistake the library-row type filter exists to
// prevent.
function TileRow({
  title,
  tiles,
  focusFirst,
  onCardFocus,
  aspectRatio = "square",
  isError,
  onRetry,
}: {
  title: string;
  tiles: {
    key: string;
    label: string;
    sublabel?: string;
    uri?: string;
    onPress: () => void;
    /** Announced instead of `label` when the visible text is abbreviated - a
     * photo album shows "Holiday" but is called "2024/Holiday", and at three
     * metres the reader is the only thing carrying that context. */
    a11yLabel?: string;
  }[];
  focusFirst?: boolean;
  onCardFocus?: () => void;
  /** "poster" crops to a 2:3 sheet (collections), "square" keeps the tile
   * square (music/photo art, which is actually square). A collection poster
   * forced into a square used to get cropped down to its middle third. */
  aspectRatio?: "poster" | "square";
  isError?: boolean;
  onRetry?: () => void;
}) {
  const theme = useTheme();
  if (isError) {
    return (
      <View style={styles.rowContainer}>
        <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
        <RowFailure onRetry={onRetry} />
      </View>
    );
  }
  if (tiles.length === 0) return null;
  const tileHeight = aspectRatio === "poster" ? CARD_HEIGHT : CARD_WIDTH;
  return (
    <View style={styles.rowContainer}>
      <Text style={[styles.rowTitle, { color: theme.colors.text }]}>{title}</Text>
      <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: HOME_ROW_GUTTER, gap: GRID_GAP }}>
        {tiles.map((tile, i) => (
          <FocusableCard
            key={tile.key}
            onPress={tile.onPress}
            hasTVPreferredFocus={i === 0 && !!focusFirst}
            onFocus={onCardFocus}
            style={styles.card}
            accessibilityLabel={tile.a11yLabel ?? tile.label}
          >
            <View
              style={{
                width: CARD_WIDTH,
                height: tileHeight,
                borderRadius: theme.radii.sm,
                overflow: "hidden",
                backgroundColor: theme.colors.surface,
                justifyContent: "center",
                alignItems: "center",
              }}
            >
              {tile.uri ? (
                <Image source={{ uri: tile.uri }} style={{ width: CARD_WIDTH, height: tileHeight }} contentFit="cover" transition={200} />
              ) : (
                // No artwork: an empty grey square used to be the tile. The
                // first letter at least tells a scanning eye what it is
                // looking at until art arrives.
                <Text style={[styles.tileInitial, { color: theme.colors.textMuted }]}>
                  {tile.label.trim().charAt(0).toUpperCase() || "?"}
                </Text>
              )}
            </View>
            <Text style={[styles.cardTitle, { color: theme.colors.text }]} numberOfLines={1}>{tile.label}</Text>
            {tile.sublabel ? (
              <Text style={[styles.cardYear, { color: theme.colors.textMuted }]} numberOfLines={1}>{tile.sublabel}</Text>
            ) : null}
          </FocusableCard>
        ))}
      </ScrollView>
    </View>
  );
}

export default function HomeScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const accessToken = useAuthStore((s) => s.accessToken);


  // The row plan comes from the server, exactly as on web and mobile: same
  // feed types, same order, same per-section item counts. The TV used to
  // hardcode three rows plus one per library, so an admin who reordered the
  // home changed nothing on the television.
  const { data: feedConfigData } = useFeedServiceGetFeedConfig({ profileId });
  const libraries = useLibraryServiceListLibraries();
  const sections = useMemo(
    () => buildHomeSections(feedConfigData?.configs ?? [], libraries.data?.libraries ?? []),
    [feedConfigData, libraries.data],
  );
  // ?customize=1 - the Settings entry opens the home with its panel already
  // up, so "reorganise my home" is reachable from where a viewer looks for
  // settings, not only from the home itself.
  const { customize: customizeParam } = useLocalSearchParams<{ customize?: string }>();
  const [customizeOpen, setCustomizeOpen] = useState(customizeParam === "1");
  useEffect(() => {
    if (customizeParam === "1") setCustomizeOpen(true);
  }, [customizeParam]);
  // The header's Customize button (tabs layout) reaches the panel this way:
  // the header is drawn by the shell, above this screen.
  useEffect(() => {
    const sub = DeviceEventEmitter.addListener("eveyHomeCustomize", () => setCustomizeOpen(true));
    return () => sub.remove();
  }, []);

  // Display names for the customize panel. Without them a library row reads as
  // a raw UUID and a plugin row as `plg-…`, which is unusable at three metres.
  const libraryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const lib of libraries.data?.libraries ?? []) {
      if (lib.id && lib.name) map[lib.id] = lib.name;
    }
    return map;
  }, [libraries.data]);

  // Only plugins shipping a `home.section` module can appear in the feed; the
  // rest must not be offered as rows. Same query and same filter as mobile.
  const { data: pluginsData } = useQuery<{ plugins?: { id?: string; name?: string; frontend?: { slot?: string }[] }[] }>({
    queryKey: ["/api/v1/plugins"],
    queryFn: () => eveyFetch({ url: "/api/v1/plugins", method: "GET" }),
    staleTime: 60_000,
  });
  const pluginNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const p of pluginsData?.plugins ?? []) {
      if (!p.id || !p.name) continue;
      if (!(p.frontend ?? []).some((m) => m.slot === "home.section")) continue;
      map[p.id] = p.name;
    }
    return map;
  }, [pluginsData]);

  const maxItemsByType = useMemo(() => {
    const map: Record<string, number> = {};
    for (const s of sections) {
      if (s.kind === "feed" && s.maxItems && s.maxItems > 0) map[s.feedType] = s.maxItems;
    }
    return map;
  }, [sections]);

  const resume = usePlaybackServiceGetResumeItems({ limit: resolveMaxItems(maxItemsByType.resume_playback, 20) });
  const nextUp = usePlaybackServiceGetNextUp({ limit: resolveMaxItems(maxItemsByType.next_episode, 20) });
  const recommended = useFeedServiceRecommended({ limit: resolveMaxItems(maxItemsByType.recommended, 20) });
  const popular = useMostPopular(resolveMaxItems(maxItemsByType.most_popular, 20));
  const collections = useCollections();
  // The "recently added" shelf is the per-library
  // rows' job, which is why no client renders it as its own section.
  const recentlyAdded = useLibraryServiceListMediaItems({
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
    "pagination.pageSize": 20,
  });

  // Every library row is fetched HERE rather than inside its component, so the
  // screen knows which rows have items - that is what lets the remote's
  // initial focus land on the first row that is actually there.
  const librarySections = useMemo(() => sections.filter((s) => s.kind === "library"), [sections]);
  const libraryRowQueries = useQueries({
    queries: librarySections.map((row) => {
      const limit = resolveMaxItems(row.maxItems, HOME_LIBRARY_ROW_PAGE_SIZE);
      if (row.libraryType === "LIBRARY_TYPE_MUSIC") {
        return getMusicServiceGetAlbumsQueryOptions({ libraryId: row.libraryId });
      }
      if (row.libraryType === "LIBRARY_TYPE_PHOTOS") {
        return getPhotoServiceListPhotoAlbumsQueryOptions({ libraryId: row.libraryId });
      }
      return getLibraryServiceListMediaItemsQueryOptions({
        libraryId: row.libraryId,
        ...(row.libraryType === "LIBRARY_TYPE_SHOWS" ? { type: "MEDIA_ITEM_TYPE_SERIES" as never } : {}),
        ...(row.libraryType === "LIBRARY_TYPE_MOVIES" ? { type: "MEDIA_ITEM_TYPE_MOVIE" as never } : {}),
        sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
        sortOrder: "SORT_ORDER_DESC",
        "pagination.pageSize": limit,
      });
    }),
  });

  // Deliberately NOT waiting on the per-library rows: on the weak-NAS target
  // those are several extra queries, and gating the whole screen on the
  // slowest one turns the home into a long blank spinner. The transverse rows
  // paint as soon as they are ready and library rows fill in behind them.
  // Focus is safe either way: it is claimed by the first row that HAS items
  // at render time, so a row arriving late cannot steal it.
  const isLoading = resume.isLoading || nextUp.isLoading || recentlyAdded.isLoading;
  // The page is REVEALED once, not assembled in front of the viewer: rows
  // that landed one after another, each with its own jank on a television
  // GPU, read as a slow home (owner, 2026-09-20). The spinner stays until the
  // library rows have answered too - or until a short cap, so a slow NAS
  // still gets a page - and the whole home then fades in together.
  const [revealCapPassed, setRevealCapPassed] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setRevealCapPassed(true), HOME_REVEAL_CAP_MS);
    return () => clearTimeout(t);
  }, []);
  // Only when ALL THREE failed. One transverse row erroring still leaves a
  // usable home, and blanking it would be a worse answer than a short page.
  const isError = resume.isError && nextUp.isError && recentlyAdded.isError;

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) {
        map[p.mediaItemId] = progressPercent(p.percentComplete);
      }
    });
    return map;
  }, [resume.data?.progress]);

  // The recommended feed and the popular endpoint answer with their own
  // shapes; both are projected onto V1MediaItem so one card component serves
  // every row (same projection mobile does).
  const recommendedItems = useMemo(
    () =>
      (recommended.data?.items ?? [])
        .filter((i) => !!i.mediaItemId)
        .map((i) => ({
          id: i.mediaItemId,
          title: i.title,
          type: i.type,
          artwork: i.primaryBlurhash ? [{ type: "ARTWORK_TYPE_PRIMARY", blurhash: i.primaryBlurhash }] : undefined,
        }) as V1MediaItem),
    [recommended.data],
  );
  const popularItems = useMemo(
    () => (popular.data ?? []).map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem),
    [popular.data],
  );

  const recentlyAddedItems = recentlyAdded.data?.items ?? [];
  // Stable identity: a fresh Set every render would make PluginSlot
  // re-evaluate its list on every unrelated re-render of this screen.
  const placedPluginIds = useMemo(
    () => new Set(sections.filter((s) => s.kind === "plugin").map((s) => s.pluginId)),
    [sections],
  );
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";

  // Item counts per section, in render order, so selectHomeFocusTarget can
  // hand the remote to the first row that has something in it.
  let libraryCursor = 0;
  const sectionCounts = sections.map((section) => {
    switch (section.kind) {
      case "feed":
        switch (section.feedType) {
          case "resume_playback":
          case "continue_watching":
            return resume.data?.items?.length ?? 0;
          case "next_episode":
            return nextUp.data?.items?.length ?? 0;
          case "collections":
            return collections.data?.collections?.length ?? 0;
          case "recommended":
            return recommendedItems.length;
          case "most_popular":
            return popularItems.length;
          default:
            return 0;
        }
      case "library": {
        const q = libraryRowQueries[libraryCursor++]?.data as
          | { items?: unknown[]; albums?: unknown[] }
          | undefined;
        return (q?.items ?? q?.albums ?? []).length;
      }
      // A plugin slot owns its own focus and its own emptiness; it never
      // claims the screen's initial focus.
      default:
        return 0;
    }
  });
  const focusTarget = selectHomeFocusTarget(sectionCounts.map((n) => n > 0));

  // The first row claims the focus at mount so the D-pad has somewhere to go,
  // and Android scrolls a newly focused child into view - which pushed the
  // Customize row and the top of the widget off the screen, clipped mid-glyph
  // (owner, 2026-09-20). Snap back to the top once that settles: focus stays
  // where it is, and the page opens on its first pixel like any TV home.
  // Android scrolls on requestChildFocus, and the row claims focus some frames
  // after mount - a single snap races it and loses. Same remedy the plugin
  // bridge uses for its own late measurements: nudge a few times across the
  // window where the focus settles, then stop.
  const scrollRef = useRef<ScrollView>(null);
  useEffect(() => {
    const ids = [0, 150, 400].map((d) =>
      setTimeout(() => scrollRef.current?.scrollTo({ y: 0, animated: false }), d),
    );
    return () => ids.forEach(clearTimeout);
  }, [focusTarget]);
  // The first row is the top of the page: while the focus sits on it (or on
  // Customize), the page shows its first pixel, whatever Android did to bring
  // the focused card into view or however the plugin slot above it grew. The
  // snap is driven by FOCUS and by content growth, never by the scroll event
  // itself: undoing scrolls as they happened raced the focus leaving the row,
  // and a DOWN press landed on a second row the page had just hidden again
  // (owner, 2026-09-20).
  const onFirstRow = useRef(true);
  const snapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const snapTop = useCallback(() => {
    onFirstRow.current = true;
    scrollRef.current?.scrollTo({ y: 0, animated: false });
    // Android scrolls the focused child into view on the layout pass that
    // follows the focus; one more snap after it, unless the focus moved on.
    if (snapTimer.current) clearTimeout(snapTimer.current);
    snapTimer.current = setTimeout(() => {
      snapTimer.current = null;
      if (onFirstRow.current) scrollRef.current?.scrollTo({ y: 0, animated: false });
    }, 150);
  }, []);
  useEffect(() => () => { if (snapTimer.current) clearTimeout(snapTimer.current); }, []);
  const leaveTop = useCallback(() => {
    onFirstRow.current = false;
  }, []);
  const holdTopOnGrowth = useCallback(() => {
    if (onFirstRow.current) scrollRef.current?.scrollTo({ y: 0, animated: false });
  }, []);

  // "Nothing loaded, nothing errored, nothing has an item" used to render as
  // a near-blank screen with just the Customize row - indistinguishable from
  // a broken page. Any row still loading or erroring is excluded: those cases
  // already have their own thing to say (a spinner or RowFailure), and are
  // not "the library is empty".
  const transverseRowsPending = recommended.isLoading || popular.isLoading || collections.isLoading;
  const transverseRowsErrored =
    resume.isError || nextUp.isError || collections.isError || recommended.isError || popular.isError;
  const libraryRowsUnsettled = libraryRowQueries.some(
    (q) => (q as { isLoading?: boolean; isError?: boolean }).isLoading || (q as { isLoading?: boolean; isError?: boolean }).isError,
  );
  const isAllEmpty =
    !transverseRowsPending &&
    !transverseRowsErrored &&
    !libraryRowsUnsettled &&
    recentlyAddedItems.length === 0 &&
    sectionCounts.every((n) => n === 0);

  function renderSection(section: HomeSection, index: number, libraryIndex: number) {
    const focusFirst = focusTarget === index;
    const onCardFocus = focusFirst ? snapTop : leaveTop;
    if (section.kind === "plugin") {
      return <PluginSlot key={section.key} name="home.section" pluginId={section.pluginId} />;
    }
    if (section.kind === "library") {
      const data = libraryRowQueries[libraryIndex]?.data as
        | { items?: V1MediaItem[]; albums?: Record<string, unknown>[] }
        | undefined;
      if (section.libraryType === "LIBRARY_TYPE_MUSIC") {
        const albums = (data?.albums ?? []).slice(0, resolveMaxItems(section.maxItems, 20));
        return (
          <TileRow
            key={section.key}
            title={section.name}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            tiles={albums.map((a) => ({
              key: String(a.id ?? a.title ?? ""),
              label: String(a.title ?? ""),
              sublabel: a.artist ? String(a.artist) : undefined,
              uri: a.coverArtUrl ? `${String(a.coverArtUrl)}${tokenParam}` : undefined,
              // Through the shared helper: the album screen needs the library
              // id too, and a bare id left it with a disabled query - a title
              // that was the raw id and "no track found" (device, 2026-08-03).
              onPress: () => router.push(musicAlbumHref(String(a.id ?? ""), section.libraryId) as never),
            }))}
          />
        );
      }
      if (section.libraryType === "LIBRARY_TYPE_PHOTOS") {
        const albums = (data?.albums ?? []).slice(0, resolveMaxItems(section.maxItems, 20));
        return (
          <TileRow
            key={section.key}
            title={section.name}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            tiles={albums.map((a) => ({
              key: String(a.name ?? ""),
              label: String(a.name ?? "").split("/").pop() ?? "",
              a11yLabel: String(a.name ?? ""),
              sublabel: t("media.photos.photoCount", { count: Number(a.photoCount ?? 0) }),
              uri: a.coverPhotoId
                ? `${serverUrl}/api/v1/photos/${String(a.coverPhotoId)}/serve${tokenParam}&w=400&fmt=webp`
                : undefined,
              onPress: () => router.push("/(auth)/photos"),
            }))}
          />
        );
      }
      return (
        <MediaRow
          key={section.key}
          title={t("media.home.recentlyAddedInLibrary", { name: section.name })}
          items={data?.items ?? []}
          serverUrl={serverUrl}
          authHeaders={authHeaders}
          focusFirst={focusFirst}
          onCardFocus={onCardFocus}
        />
      );
    }

    switch (section.feedType) {
      case "resume_playback":
      case "continue_watching":
        return (
          <MediaRow
            key={section.key}
            title={t("media.home.continueWatching")}
            items={resume.data?.items ?? []}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            watchPercents={watchPercents}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            isError={resume.isError}
            onRetry={() => void resume.refetch()}
          />
        );
      case "next_episode":
        return (
          <MediaRow
            key={section.key}
            title={t("media.home.upNext")}
            items={nextUp.data?.items ?? []}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            isError={nextUp.isError}
            onRetry={() => void nextUp.refetch()}
          />
        );
      case "collections": {
        const all = collections.data?.collections ?? [];
        // 105 repaired sagas landed today: an unbounded row would mount them all.
        const cols = all.slice(0, resolveMaxItems(section.maxItems, 20));
        return (
          <TileRow
            key={section.key}
            title={t("media.home.collections")}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            aspectRatio="poster"
            isError={collections.isError}
            onRetry={() => void collections.refetch()}
            tiles={cols.map((c) => ({
              key: c.id,
              label: c.name,
              uri: c.posterUrl ? `${c.posterUrl}${tokenParam}` : undefined,
              onPress: () => router.push(`/(auth)/collections/${c.id}`),
            }))}
          />
        );
      }
      case "recommended":
        return (
          <MediaRow
            key={section.key}
            title={t("media.home.recommendedForYou")}
            items={recommendedItems}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            isError={recommended.isError}
            onRetry={() => void recommended.refetch()}
          />
        );
      case "most_popular":
        return (
          <MediaRow
            key={section.key}
            title={t("media.home.mostPopular")}
            items={popularItems}
            serverUrl={serverUrl}
            authHeaders={authHeaders}
            focusFirst={focusFirst}
            onCardFocus={onCardFocus}
            isError={popular.isError}
            onRetry={() => void popular.refetch()}
          />
        );
      default:
        return null;
    }
  }

  // A home that fetched nothing used to render its empty rows, which reads as
  // "your library is empty" rather than "the server did not answer". The side
  // nav is drawn by the layout, so this keeps every tab reachable.
  const libraryRowsLoading = libraryRowQueries.some((q) => (q as { isLoading?: boolean }).isLoading);
  const revealed = !isLoading && (revealCapPassed || (!transverseRowsPending && !libraryRowsLoading));
  const revealOpacity = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    if (!revealed) return;
    Animated.timing(revealOpacity, { toValue: 1, duration: 280, useNativeDriver: true }).start();
  }, [revealed, revealOpacity]);
  if (!revealed && !isError) return <HomeSkeleton />;
  if (isError) {
    return (
      <ScreenState
        loading={false}
        error={isError}
        onRetry={() => {
          void resume.refetch();
          void nextUp.refetch();
          void recentlyAdded.refetch();
        }}
      />
    );
  }

  let renderCursor = 0;
  return (
    <Animated.View style={{ flex: 1, opacity: revealOpacity }}>
    <ScrollView
      ref={scrollRef}
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingTop: 4, paddingBottom: 16 }}
      onContentSizeChange={holdTopOnGrowth}
    >
      {/* Plugin sections the feed config does not place yet render first, the
          same as mobile: a home.section plugin is a hero widget, and pushing
          it under every row made the user reorder it by hand. The ones it DOES
          place are rendered inline below, so they are excluded here or they
          would appear twice. */}
      <PluginSlot name="home.section" excludeRegistered={placedPluginIds} />
      {sections.map((section, i) =>
        renderSection(section, i, section.kind === "library" ? renderCursor++ : -1),
      )}

      {/* Nothing loaded anywhere and nothing errored either: a fresh install
          with no library scanned yet, or every feed disabled. The screen used
          to fall through to just the Customize row, which reads as broken
          rather than as "add a library". This is the only focusable content
          on the page in that case, so it claims the initial focus. */}
      {isAllEmpty ? (
        <View style={styles.emptyStateContainer}>
          <FocusableCard
            hasTVPreferredFocus={focusTarget === null}
            onPress={() => setCustomizeOpen(true)}
            accessibilityLabel={t("media.home.emptyLibrary")}
            style={[styles.emptyStateCard, { backgroundColor: theme.colors.surface }]}
          >
            <Text style={[styles.emptyStateTitle, { color: theme.colors.text }]}>
              {t("media.home.emptyLibrary")}
            </Text>
          </FocusableCard>
        </View>
      ) : null}

      <FeedCustomizePanel
        visible={customizeOpen}
        onClose={() => setCustomizeOpen(false)}
        configs={feedConfigData?.configs ?? []}
        profileId={profileId}
        libraryNames={libraryNames}
        pluginNames={pluginNames}
      />
    </ScrollView>
    </Animated.View>
  );
}

// What the home is about to be, pulsing, while its rows answer: a widget
// block, a row title, a row of posters, at the sizes the real ones will
// have, so the fade-in lands the content where the eye already is. A
// spinner in the middle of a dark screen read as "nothing is happening"
// (owner, 2026-09-20).
function HomeSkeleton() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { height: windowHeight } = useWindowDimensions();
  const pulse = useRef(new Animated.Value(0.45)).current;
  useEffect(() => {
    const loop = Animated.loop(
      Animated.sequence([
        Animated.timing(pulse, { toValue: 0.9, duration: 700, useNativeDriver: true }),
        Animated.timing(pulse, { toValue: 0.45, duration: 700, useNativeDriver: true }),
      ]),
    );
    loop.start();
    return () => loop.stop();
  }, [pulse]);
  const block = { backgroundColor: theme.colors.surface, borderRadius: theme.radii.sm };
  return (
    <Animated.View style={{ flex: 1, paddingTop: 4, opacity: pulse }} accessibilityLabel={t("common.loading", "Loading")}>
      <View style={[block, { height: homeWidgetBudget(windowHeight), marginHorizontal: 24, borderRadius: theme.radii.md }]} />
      <View style={[block, { height: 26, width: 280, marginLeft: HOME_ROW_GUTTER, marginTop: 28, marginBottom: 14 }]} />
      <View style={{ flexDirection: "row", paddingLeft: HOME_ROW_GUTTER, gap: GRID_GAP, overflow: "hidden" }}>
        {Array.from({ length: 8 }, (_, i) => (
          <View key={i}>
            <View style={[block, { width: CARD_WIDTH, height: CARD_HEIGHT }]} />
            <View style={[block, { height: 16, width: CARD_WIDTH * 0.8, marginTop: 10 }]} />
          </View>
        ))}
      </View>
    </Animated.View>
  );
}

const styles = StyleSheet.create({
  rowContainer: {
    marginBottom: 32,
  },
  rowTitle: {
    fontSize: 24,
    fontWeight: "700",
    paddingHorizontal: HOME_ROW_GUTTER,
    marginBottom: 10,
  },
  rowFailure: {
    paddingHorizontal: 24,
    alignItems: "flex-start",
    gap: 10,
  },
  rowFailureMessage: {
    fontSize: 17,
  },
  rowRetryBtn: {
    paddingVertical: 10,
    paddingHorizontal: 24,
    borderRadius: 8,
  },
  emptyStateContainer: {
    paddingHorizontal: 24,
    marginBottom: 32,
    alignItems: "flex-start",
  },
  emptyStateCard: {
    paddingVertical: 16,
    paddingHorizontal: 32,
    borderRadius: 12,
  },
  emptyStateTitle: {
    fontSize: 20,
    fontWeight: "600",
  },
  card: {
    width: CARD_WIDTH,
  },
  tileInitial: {
    fontSize: 40,
    fontWeight: "700",
  },
  poster: {
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
    overflow: "hidden",
    position: "relative",
  },
  ratingBadge: {
    position: "absolute",
    bottom: 6,
    right: 6,
    flexDirection: "row",
    alignItems: "center",
    gap: 3,
    paddingVertical: 2,
    paddingHorizontal: 6,
    borderRadius: 4,
  },
  ratingBadgeText: {
    fontSize: 12,
    fontWeight: "700",
  },
  posterPlaceholder: {
    width: CARD_WIDTH,
    height: CARD_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  progressBar: {
    height: 4,
    borderRadius: 2,
    marginTop: 4,
    overflow: "hidden",
  },
  progressFill: {
    height: "100%",
    borderRadius: 2,
  },
  cardTitle: {
    fontSize: 15,
    fontWeight: "600",
    marginTop: 4,
  },
  cardYear: {
    fontSize: 13,
    marginTop: 2,
  },
});
