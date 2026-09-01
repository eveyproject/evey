import { useState, useMemo, memo, type FC } from "react";
import { Folder, Settings2 } from "lucide-react";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  useFeedServiceRecommended,
  useFeedServiceGetFeedConfig,
  useFeedServiceRecentlyAdded,
  usePlaybackServiceGetResumeItems,
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  useMusicServiceGetAlbums,
  usePhotoServiceListPhotoAlbums,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTranslation } from "react-i18next";
import { MediaSlider } from "../../features/home/MediaSlider";
import { MediaCardWithModals as MediaCard } from "../../features/library/MediaCardWithModals";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { slugify } from "@/utils/slugify";
import { useCollections } from "../../features/collection/api";
import { CollectionCard } from "../../features/collection/CollectionCard";
import { useMostPopular } from "@/hooks/usePopular";
import { useWatchedMap } from "@/hooks/useWatchedMap";
import { PluginSlot } from "@/features/plugins/PluginSlot";
import { CustomizePanel } from "@/features/home/CustomizePanel";
import { NextUpRow } from "@/features/home/NextUpRow";
import { coverArtSrc } from "../../lib/coverArtSrc";

export const Route = createLazyFileRoute("/_auth/home")({
  component: HomePage,
});

const MusicLibrarySlider = memo(function MusicLibrarySlider({ libraryId, name, maxItems }: LibrarySliderProps) {
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const { data, isLoading } = useMusicServiceGetAlbums({ libraryId });
  const albums = (data?.albums ?? []).slice(0, maxItems && maxItems > 0 ? maxItems : 20);
  if (!isLoading && albums.length === 0) return null;
  return (
    <MediaSlider title={name} seeAllHref="/music" isLoading={isLoading}>
      {albums.map((a) => (
        <Link
          key={a.id}
          to="/music"
          search={{ tab: "tracks", album: a.id ?? "", libraryId }}
          className="block w-[160px] flex-shrink-0"
        >
          <div
            className="aspect-square rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] bg-cover bg-center"
            style={{ backgroundImage: a.coverArtUrl ? `url(${coverArtSrc(a.coverArtUrl, tokenParam, 320)})` : undefined }}
          />
          <p className="text-sm font-medium text-[var(--text)] truncate mt-2">{a.title}</p>
          <p className="text-xs text-[var(--text-muted)] truncate">{a.artist}</p>
        </Link>
      ))}
    </MediaSlider>
  );
});

const PhotoLibrarySlider = memo(function PhotoLibrarySlider({ libraryId, name, maxItems }: LibrarySliderProps) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";
  const { data, isLoading } = usePhotoServiceListPhotoAlbums({ libraryId });
  const albums = (data?.albums ?? []).slice(0, maxItems && maxItems > 0 ? maxItems : 20);
  if (!isLoading && albums.length === 0) return null;
  return (
    <MediaSlider title={name} seeAllHref="/photos" isLoading={isLoading}>
      {albums.map((a) => (
        <Link key={a.name} to="/photos" className="block w-[160px] flex-shrink-0">
          <div
            className="aspect-square rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] bg-cover bg-center"
            style={{ backgroundImage: a.coverPhotoId ? `url(/api/v1/photos/${a.coverPhotoId}/serve${tokenParam}&w=400&fmt=webp)` : undefined }}
          />
          <p className="text-sm font-medium text-[var(--text)] truncate mt-2" title={a.name}>{(a.name ?? "").split("/").pop()}</p>
          <p className="text-xs text-[var(--text-muted)] truncate">{t("media.photos.photoCount", { count: a.photoCount ?? 0 })}</p>
        </Link>
      ))}
    </MediaSlider>
  );
});

interface LibrarySliderProps {
  libraryId: string;
  name: string;
  libraryType?: string;
  maxItems?: number;
}

const LibrarySlider = memo(function LibrarySlider({ libraryId, name, libraryType, maxItems }: LibrarySliderProps) {
  if (libraryType === "LIBRARY_TYPE_MUSIC") return <MusicLibrarySlider libraryId={libraryId} name={name} maxItems={maxItems} />;
  if (libraryType === "LIBRARY_TYPE_PHOTOS") return <PhotoLibrarySlider libraryId={libraryId} name={name} maxItems={maxItems} />;
  return <MediaItemsLibrarySlider libraryId={libraryId} name={name} libraryType={libraryType} maxItems={maxItems} />;
});

const MediaItemsLibrarySlider = memo(function MediaItemsLibrarySlider({ libraryId, name, libraryType, maxItems }: LibrarySliderProps) {
  const typeFilter = libraryType === "LIBRARY_TYPE_SHOWS"
    ? "MEDIA_ITEM_TYPE_SERIES"
    : libraryType === "LIBRARY_TYPE_MOVIES"
      ? "MEDIA_ITEM_TYPE_MOVIE"
      : undefined;
  // Page over-fetched: server-side library lists don't filter by watched
  // state (that would change the lib page semantics), but home sliders
  // should hide items the active profile has marked watched. Pull a wider
  // window so the visible row stays full after watched items are filtered.
  // The fetched window scales with maxItems so admin can ask for more
  // than the 20 baseline without coming up short after watched-filter.
  const visibleTarget = maxItems && maxItems > 0 ? maxItems : 20;
  const fetchPageSize = Math.max(40, visibleTarget * 2);
  const { data, isLoading } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter as never } : {}),
    "pagination.pageSize": fetchPageSize,
    sortField: "MEDIA_SORT_FIELD_DATE_ADDED",
    sortOrder: "SORT_ORDER_DESC",
  });

  const watchedMap = useWatchedMap();
  const items = useMemo(() => {
    const all = data?.items ?? [];
    return all.filter((it) => !it.id || !watchedMap.get(it.id)).slice(0, visibleTarget);
  }, [data, watchedMap, visibleTarget]);
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={name} seeAllHref={`/library/${slugify(name)}`} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard key={item.id} item={item} lite />
      ))}
    </MediaSlider>
  );
});

const ResumePlaybackSlider = memo(function ResumePlaybackSlider({ maxItems }: SectionProps) {
  const { t } = useTranslation();
  // Same RPC as mobile and TV (PlaybackService.GetResumeItems). The web was
  // the last client on FeedService.ResumePlayback, whose FeedItem carries no
  // duration and no percent: this row structurally COULD NOT show a progress
  // bar - the one thing a "Continue watching" card exists for (audit
  // 2026-08-09). The full MediaItems also bring artwork/seriesTitle, so
  // displayTitle renders episodes as the series - no hand-built SxEy label.
  const { data, isLoading } = usePlaybackServiceGetResumeItems({ limit: maxItems });
  const items = data?.items ?? [];
  const percents = useMemo(() => {
    const map: Record<string, number> = {};
    data?.progress?.forEach((p) => {
      if (p.mediaItemId && p.percentComplete != null) map[p.mediaItemId] = p.percentComplete;
    });
    return map;
  }, [data?.progress]);
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("common.feeds.labels.resume_playback")} isLoading={isLoading}>
      {items.map((item) => {
        const pct = item.id ? percents[item.id] : undefined;
        return (
          <MediaCard
            key={item.id}
            item={item}
            showYear={false}
            showProgress={pct !== undefined && pct > 0}
            progressPercent={pct}
          />
        );
      })}
    </MediaSlider>
  );
});

const MostPopularSlider = memo(function MostPopularSlider({ maxItems }: SectionProps) {
  const { t } = useTranslation();
  const { data, isLoading } = useMostPopular(maxItems ?? 20);
  const items = data ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.mostPopular")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          showType
          lite
        />
      ))}
    </MediaSlider>
  );
});

const RecommendedSlider = memo(function RecommendedSlider({ maxItems }: SectionProps) {
  const { t } = useTranslation();
  const { data, isLoading } = useFeedServiceRecommended({ limit: maxItems ?? 20 });
  const items = data?.items ?? [];
  if (!isLoading && items.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.recommendedForYou")} isLoading={isLoading}>
      {items.map((item) => (
        <MediaCard
          key={item.mediaItemId}
          item={{ id: item.mediaItemId, title: item.title, type: item.type }}
          posterBlurhash={item.primaryBlurhash}
          showType
          lite
        />
      ))}
    </MediaSlider>
  );
});

const CollectionsSection = memo(function CollectionsSection({ maxItems }: SectionProps) {
  const { t } = useTranslation();
  const { data, isLoading } = useCollections();
  const collections = useMemo(() => {
    const all = data?.collections ?? [];
    return maxItems ? all.slice(0, maxItems) : all;
  }, [data, maxItems]);
  if (!isLoading && collections.length === 0) return null;

  return (
    <MediaSlider title={t("media.home.collections")} seeAllHref="/collections" isLoading={isLoading} skeletonCount={6}>
      {collections.map((col) => (
        <CollectionCard key={col.id} collection={col} />
      ))}
    </MediaSlider>
  );
});

// Each home section accepts an optional `maxItems` overriding its default -
// driven by the merged feed config (admin default + profile override).
// Falsy = "use the slider's own default" so we don't accidentally fetch 0.
type SectionProps = { maxItems?: number };

const SECTION_REGISTRY: Record<string, FC<SectionProps>> = {
  resume_playback: ResumePlaybackSlider,
  next_episode: NextUpRow,
  collections: CollectionsSection,
  recommended: RecommendedSlider,
  most_popular: MostPopularSlider,
};

// Keep in sync with KNOWN_FEED_TYPES in features/home/feedTypes.ts.
const FALLBACK_ORDER = [
  "resume_playback", "next_episode", "collections",
  "recommended", "most_popular",
];

function HomePage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const profileId = useAuthStore((s) => s.profileId);
  const [customizeOpen, setCustomizeOpen] = useState(false);

  const { data: librariesData, isLoading: librariesLoading, isError: librariesIsError, error: librariesErrorObj, refetch: refetchLibraries } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];
  const { data: recentlyAdded, isLoading: recentLoading, isError: recentIsError, error: recentErrorObj, refetch: refetchRecent } = useFeedServiceRecentlyAdded();
  const { data: feedConfig, isError: feedConfigError } = useFeedServiceGetFeedConfig(
    { profileId: profileId ?? undefined },
    { query: { retry: 1, staleTime: 60_000 } },
  );
  const hasContent = (recentlyAdded?.items ?? []).length > 0 || libraries.length > 0;
  const isLoading = librariesLoading || recentLoading;
  const isContentError = librariesIsError || recentIsError;
  // Whichever of the two failed drives the error copy - librariesErrorObj first,
  // since a broken library list is the more common/actionable failure.
  const contentError = librariesIsError ? librariesErrorObj : recentErrorObj;
  const retryContent = () => { void refetchLibraries(); void refetchRecent(); };

  const allSections = useMemo(() => {
    const configs = feedConfig?.configs;
    if (!configs || configs.length === 0 || feedConfigError) {
      return FALLBACK_ORDER.map((ft, i) => ({ feedType: ft, enabled: true, displayOrder: i, maxItems: undefined as number | undefined }));
    }
    return [...configs].sort((a, b) => (a.displayOrder ?? 0) - (b.displayOrder ?? 0));
  }, [feedConfig, feedConfigError]);

  const sections = useMemo(() => allSections.filter((c) => c.enabled), [allSections]);

  // Stabilize the Set passed to PluginSlot - otherwise a new Set identity
  // every render forces the slot to re-evaluate its registered list each time
  // the parent re-renders for unrelated reasons.
  const excludeRegisteredPluginIDs = useMemo(
    () => new Set(
      allSections
        .map((s) => s.feedType ?? "")
        .filter((ft) => ft.startsWith("plugin:"))
        .map((ft) => ft.slice("plugin:".length)),
    ),
    [allSections],
  );

  return (
    <div className="animate-fade-in" data-testid="home-page">
      <div className="px-4 sm:px-6 pt-6">
        <div className="flex justify-end mb-2">
          <button
            onClick={() => setCustomizeOpen(true)}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded-[var(--radius-sm)] transition-colors-fast hover:opacity-80 text-[var(--text-muted)]"
            aria-label={t("common.feeds.customize")}
            data-testid="home-customize-btn"
          >
            <Settings2 size={14}  aria-hidden="true" />
            {t("common.feeds.customize")}
          </button>
        </div>

        {/* Plugins mounting the page hero - the strip above every feed. */}
        <PluginSlot name="home.hero" />

        {!isLoading && isContentError ? (
          <FetchErrorState error={contentError} onRetry={retryContent} />
        ) : !isLoading && !hasContent ? (
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.home.emptyLibrary")}
            description={t("media.home.emptyLibraryDesc")}
            action={{ label: t("media.home.goToLibraries"), onClick: () => navigate({ to: "/admin/libraries" }) }}
          />
        ) : (
          <>
            {/* Newly-installed home.section plugins not yet in the user's
                feed config render at the TOP - matches CustomizePanel,
                which prepends them with a negative displayOrder. Once the
                user saves a position, the entry persists and is rendered
                inline below at its sorted slot. */}
            <PluginSlot name="home.section" excludeRegistered={excludeRegisteredPluginIDs} />
            {sections.map((cfg) => {
              const ft = cfg.feedType ?? "";
              const max = cfg.maxItems && cfg.maxItems > 0 ? cfg.maxItems : undefined;
              if (ft.startsWith("library:")) {
                const libId = ft.replace("library:", "");
                const lib = libraries.find((l) => l.id === libId);
                return lib ? (
                  <LibrarySlider key={ft} libraryId={libId} name={lib.name ?? t("media.home.library")} libraryType={lib.type} maxItems={max} />
                ) : null;
              }
              if (ft.startsWith("plugin:")) {
                // Render this single plugin's home.section slot at the
                // exact position dictated by the customise-home order.
                // PluginSlot filters internally to just the matching id.
                const pluginId = ft.slice("plugin:".length);
                return <PluginSlot key={ft} name="home.section" pluginId={pluginId} />;
              }
              const Component = SECTION_REGISTRY[ft];
              return Component ? <Component key={ft} maxItems={max} /> : null;
            })}

            {libraries
              .filter((lib) => lib.id && !allSections.some((s) => s.feedType === `library:${lib.id}`))
              .map((lib) => (
                <LibrarySlider key={lib.id} libraryId={lib.id!} name={lib.name ?? t("media.home.library")} libraryType={lib.type} />
              ))}
          </>
        )}
      </div>

      <CustomizePanel open={customizeOpen} onClose={() => setCustomizeOpen(false)} />
    </div>
  );
}
