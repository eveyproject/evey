import { useEffect, useMemo, useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  displayTitle,
  eveyFetch,
  getLibraryServiceListMediaItemsQueryOptions,
  getMusicServiceGetAlbumsQueryOptions,
  getPhotoServiceListPhotoAlbumsQueryOptions,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecommended,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetNextUp,
  usePlaybackServiceGetResumeItems,
  type V1MediaItem,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import {
  CARD_WIDTH_FEATURED,
  HOME_LIBRARY_ROW_PAGE_SIZE,
  POSTER_RATIO,
  buildHomeSections,
  homeWidgetBudget,
  mediaTypeLabelKey,
  progressPercent,
  resolveMaxItems,
  useCollections,
  useMostPopular,
  type HomeSection,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Pressable } from "../components/ui";
import { PosterCard } from "../components/cards";
import { navigate } from "../app/router";
import { authedSrc, posterSrc, serverImageSrc } from "../lib/media";
import { dpPx } from "../lib/uiScale";
import { FeedCustomizePanel } from "../components/FeedCustomizePanel";
import { PluginSlot } from "../plugins/PluginSlot";
import { HOME_CUSTOMIZE_EVENT } from "../components/StackHeader";
import { ScreenState } from "../components/ScreenState";

// How long the home waits for its slower rows before showing what it has.
const HOME_REVEAL_CAP_MS = 2500;

interface Tile {
  key: string;
  label: string;
  sublabel?: string;
  uri?: string;
  onPress: () => void;
}

function RowFailure({ title, onRetry }: { title: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <FocusRoot className="home-row">
      <h2 className="home-row-title">{title}</h2>
      <div className="row-failure">
        <span className="muted">{t("common.loadError")}</span>
        <Pressable className="tv-card retry-btn" onPress={onRetry}>{t("common.retry")}</Pressable>
      </div>
    </FocusRoot>
  );
}

function MediaRow({ rowKey, title, items, watchPercents, focusFirst }: {
  rowKey: string;
  title: string;
  items: V1MediaItem[];
  watchPercents?: Record<string, number>;
  focusFirst: boolean;
}) {
  const { t } = useTranslation();
  if (items.length === 0) return null;
  return (
    <FocusRoot className="home-row">
      <h2 className="home-row-title">{title}</h2>
      <div className="home-row-list media">
        {items.map((item, i) => {
          const typeKey = !item.year ? mediaTypeLabelKey(item.type) : null;
          return (
            <PosterCard
              key={item.id}
              width={CARD_WIDTH_FEATURED}
              title={displayTitle(item, t)}
              subtitle={item.year ? String(item.year) : typeKey ? t(typeKey) : undefined}
              imageUrl={posterSrc(item.id, "poster", CARD_WIDTH_FEATURED * 2)}
              rating={item.communityRating}
              percent={item.id ? watchPercents?.[item.id] : undefined}
              focusKey={`home:${rowKey}:${item.id}`}
              autoFocus={focusFirst && i === 0}
              onPress={() => item.id && navigate({ name: "media", mediaId: item.id })}
            />
          );
        })}
      </div>
    </FocusRoot>
  );
}

function TileRow({ rowKey, title, tiles, focusFirst, aspect = "square" }: {
  rowKey: string;
  title: string;
  tiles: Tile[];
  focusFirst: boolean;
  aspect?: "poster" | "square";
}) {
  if (tiles.length === 0) return null;
  return (
    <FocusRoot className="home-row">
      <h2 className="home-row-title">{title}</h2>
      <div className="home-row-list">
        {tiles.map((tile, i) => (
          <PosterCard
            key={tile.key}
            width={CARD_WIDTH_FEATURED}
            aspect={aspect}
            title={tile.label}
            subtitle={tile.sublabel}
            imageUrl={tile.uri}
            focusKey={`home:${rowKey}:${tile.key}`}
            autoFocus={focusFirst && i === 0}
            onPress={tile.onPress}
          />
        ))}
      </div>
    </FocusRoot>
  );
}

// What the home is about to be, pulsing, at the sizes the real rows will have.
function HomeSkeleton() {
  const budget = homeWidgetBudget(window.innerHeight / dpPx());
  const w = CARD_WIDTH_FEATURED / 10;
  return (
    <div className="home-skeleton" aria-busy="true">
      <div className="sk-block" style={{ height: `${budget / 10}rem`, margin: "0 2.4rem", borderRadius: "var(--r-md)" }} />
      <div className="sk-block" style={{ height: "2.6rem", width: "28rem", margin: "2.8rem 0 1.4rem 3.2rem" }} />
      <div className="sk-row">
        {Array.from({ length: 8 }, (_, i) => (
          <div key={i}>
            <div className="sk-block" style={{ width: `${w}rem`, height: `${w * POSTER_RATIO}rem` }} />
            <div className="sk-block" style={{ width: `${w * 0.8}rem`, height: "1.6rem", marginTop: "1rem" }} />
          </div>
        ))}
      </div>
    </div>
  );
}

/**
 * The home, as the Android television draws it: the plugin widgets, then the
 * rows of the plan the server holds (same feeds, same order, same counts as
 * every client), revealed at once rather than assembled in front of the viewer.
 */
export function HomeScreen({ customize }: { customize?: boolean }) {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const [customizeOpen, setCustomizeOpen] = useState(!!customize);
  useEffect(() => {
    if (customize) setCustomizeOpen(true);
  }, [customize]);
  useEffect(() => {
    const open = () => setCustomizeOpen(true);
    window.addEventListener(HOME_CUSTOMIZE_EVENT, open);
    return () => window.removeEventListener(HOME_CUSTOMIZE_EVENT, open);
  }, []);

  const { data: feedConfigData } = useFeedServiceGetFeedConfig({ profileId });
  const libraries = useLibraryServiceListLibraries();
  const sections = useMemo(
    () => buildHomeSections(feedConfigData?.configs ?? [], libraries.data?.libraries ?? []),
    [feedConfigData, libraries.data],
  );

  const libraryNames = useMemo(() => {
    const map: Record<string, string> = {};
    for (const lib of libraries.data?.libraries ?? []) if (lib.id && lib.name) map[lib.id] = lib.name;
    return map;
  }, [libraries.data]);
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
  const recentlyAdded = useLibraryServiceListMediaItems({
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
    "pagination.pageSize": 20,
  });

  const librarySections = useMemo(
    () => sections.filter((s): s is Extract<HomeSection, { kind: "library" }> => s.kind === "library"),
    [sections],
  );
  const libraryRows = useQueries({
    queries: librarySections.map((row) => {
      const limit = resolveMaxItems(row.maxItems, HOME_LIBRARY_ROW_PAGE_SIZE);
      if (row.libraryType === "LIBRARY_TYPE_MUSIC") return getMusicServiceGetAlbumsQueryOptions({ libraryId: row.libraryId });
      if (row.libraryType === "LIBRARY_TYPE_PHOTOS") return getPhotoServiceListPhotoAlbumsQueryOptions({ libraryId: row.libraryId });
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

  const [revealCapPassed, setRevealCapPassed] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setRevealCapPassed(true), HOME_REVEAL_CAP_MS);
    return () => clearTimeout(timer);
  }, []);

  const watchPercents = useMemo(() => {
    const map: Record<string, number> = {};
    resume.data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) map[p.mediaItemId] = progressPercent(p.percentComplete);
    });
    return map;
  }, [resume.data?.progress]);

  const recommendedItems = useMemo(
    () => (recommended.data?.items ?? []).filter((i) => !!i.mediaItemId).map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem),
    [recommended.data],
  );
  const popularItems = useMemo(
    () => (popular.data ?? []).map((i) => ({ id: i.mediaItemId, title: i.title, type: i.type }) as V1MediaItem),
    [popular.data],
  );
  const placedPluginIds = useMemo(
    () => new Set(sections.filter((s) => s.kind === "plugin").map((s) => (s as Extract<HomeSection, { kind: "plugin" }>).pluginId)),
    [sections],
  );

  type LibraryData = { items?: V1MediaItem[]; albums?: Record<string, unknown>[] } | undefined;
  const libraryData = (section: Extract<HomeSection, { kind: "library" }>): LibraryData =>
    libraryRows[librarySections.indexOf(section)]?.data as LibraryData;

  const libraryTiles = (section: Extract<HomeSection, { kind: "library" }>): Tile[] => {
    const albums = (libraryData(section)?.albums ?? []).slice(0, resolveMaxItems(section.maxItems, 20));
    if (section.libraryType === "LIBRARY_TYPE_MUSIC") {
      return albums.map((a) => ({
        key: String(a.id ?? a.title ?? ""),
        label: String(a.title ?? ""),
        sublabel: a.artist ? String(a.artist) : undefined,
        uri: serverImageSrc(a.coverArtUrl ? String(a.coverArtUrl) : undefined),
        onPress: () => navigate({ name: "album", albumId: String(a.id ?? ""), libraryId: section.libraryId }),
      }));
    }
    return albums.map((a) => ({
      key: String(a.name ?? ""),
      label: String(a.name ?? "").split("/").pop() ?? "",
      sublabel: t("media.photos.photoCount", { count: Number(a.photoCount ?? 0) }),
      uri: a.coverPhotoId ? authedSrc(`/api/v1/photos/${String(a.coverPhotoId)}/serve?w=400&fmt=webp`) : undefined,
      onPress: () => navigate({ name: "photos", libraryId: section.libraryId }),
    }));
  };

  // Item counts in render order: the first row that has something takes the
  // remote, never a plugin slot.
  const sectionCounts = sections.map((section) => {
    if (section.kind === "library") {
      const d = libraryData(section);
      return (d?.items ?? d?.albums ?? []).length;
    }
    if (section.kind !== "feed") return 0;
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
  });
  const focusTarget = sectionCounts.findIndex((n) => n > 0);

  const isLoading = resume.isLoading || nextUp.isLoading || recentlyAdded.isLoading;
  const isError = resume.isError && nextUp.isError && recentlyAdded.isError;
  const transversePending = recommended.isLoading || popular.isLoading || collections.isLoading;
  const transverseErrored = resume.isError || nextUp.isError || collections.isError || recommended.isError || popular.isError;
  const libraryLoading = libraryRows.some((q) => q.isLoading);
  const libraryUnsettled = libraryRows.some((q) => q.isLoading || q.isError);
  const revealed = !isLoading && (revealCapPassed || (!transversePending && !libraryLoading));
  const isAllEmpty = !transversePending && !transverseErrored && !libraryUnsettled
    && (recentlyAdded.data?.items ?? []).length === 0 && sectionCounts.every((n) => n === 0);

  if (isError) {
    return (
      <ScreenState
        error
        onRetry={() => {
          void resume.refetch();
          void nextUp.refetch();
          void recentlyAdded.refetch();
        }}
      />
    );
  }
  if (!revealed) return <HomeSkeleton />;

  const renderSection = (section: HomeSection, index: number) => {
    const focusFirst = focusTarget === index;
    if (section.kind === "plugin") {
      return <div key={section.key} className="home-plugin"><PluginSlot name="home.section" pluginId={section.pluginId} /></div>;
    }
    if (section.kind === "library") {
      if (section.libraryType === "LIBRARY_TYPE_MUSIC" || section.libraryType === "LIBRARY_TYPE_PHOTOS") {
        return <TileRow key={section.key} rowKey={section.key} title={section.name} tiles={libraryTiles(section)} focusFirst={focusFirst} />;
      }
      return (
        <MediaRow
          key={section.key}
          rowKey={section.key}
          title={t("media.home.recentlyAddedInLibrary", { name: section.name })}
          items={libraryData(section)?.items ?? []}
          focusFirst={focusFirst}
        />
      );
    }
    const feed = (title: string, q: { isError: boolean; refetch: () => unknown }, items: V1MediaItem[], percents?: Record<string, number>) =>
      q.isError
        ? <RowFailure key={section.key} title={title} onRetry={() => void q.refetch()} />
        : <MediaRow key={section.key} rowKey={section.key} title={title} items={items} watchPercents={percents} focusFirst={focusFirst} />;
    switch (section.feedType) {
      case "resume_playback":
      case "continue_watching":
        return feed(t("media.home.continueWatching"), resume, resume.data?.items ?? [], watchPercents);
      case "next_episode":
        return feed(t("media.home.upNext"), nextUp, nextUp.data?.items ?? []);
      case "recommended":
        return feed(t("media.home.recommendedForYou"), recommended, recommendedItems);
      case "most_popular":
        return feed(t("media.home.mostPopular"), popular, popularItems);
      case "collections": {
        if (collections.isError) return <RowFailure key={section.key} title={t("media.home.collections")} onRetry={() => void collections.refetch()} />;
        const cols = (collections.data?.collections ?? []).slice(0, resolveMaxItems(section.maxItems, 20));
        return (
          <TileRow
            key={section.key}
            rowKey={section.key}
            title={t("media.home.collections")}
            aspect="poster"
            focusFirst={focusFirst}
            tiles={cols.map((c) => ({
              key: c.id,
              label: c.name,
              uri: serverImageSrc(c.posterUrl),
              onPress: () => navigate({ name: "collection", collectionId: c.id }),
            }))}
          />
        );
      }
      default:
        return null;
    }
  };

  return (
    <FocusRoot autoFocus className="home-screen">
      <div className="home-plugin"><PluginSlot name="home.section" excludeRegistered={placedPluginIds} /></div>
      {sections.map(renderSection)}
      {isAllEmpty ? (
        <FocusRoot className="home-empty">
          <Pressable className="tv-card home-empty-card" autoFocus onPress={() => setCustomizeOpen(true)}>
            {t("media.home.emptyLibrary")}
          </Pressable>
        </FocusRoot>
      ) : null}
      {customizeOpen ? (
        <FeedCustomizePanel
          configs={feedConfigData?.configs ?? []}
          profileId={profileId}
          libraryNames={libraryNames}
          pluginNames={pluginNames}
          onClose={() => setCustomizeOpen(false)}
        />
      ) : null}
    </FocusRoot>
  );
}
